package credentials

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/cotta-dev/retri/internal/config"
)

type promptFunc func(string) (string, error)
type commandFunc func(string, []string, []byte) ([]byte, error)
type result struct {
	done  chan struct{}
	value string
	err   error
}

var errPromptDisabled = errors.New("terminal prompt is disabled; initialize the credential cache first")

// Resolver owns one invocation's snapshot. Successes and failures are shared
// per name. Session-cache TTL governs reuse by subsequent invocations.
type Resolver struct {
	specs     map[string]config.CredentialSpec
	mu        sync.Mutex
	values    map[string]*result
	promptMu  sync.Mutex
	prompt    promptFunc
	run       commandFunc
	cache     credentialCache
	namespace string
	refresh   bool
}

func NewResolver(specs map[string]config.CredentialSpec) *Resolver {
	copySpecs := make(map[string]config.CredentialSpec, len(specs))
	for name, spec := range specs {
		copySpecs[name] = spec
	}
	return &Resolver{specs: copySpecs, values: make(map[string]*result), prompt: readHiddenPrompt, run: runCommand, cache: newCredentialCache()}
}

// Configure must precede Resolve. Namespace is the canonical config path.
func (r *Resolver) Configure(namespace string, refresh bool, env []string) {
	r.namespace, r.refresh = namespace, refresh
	r.run = commandRunner(env)
}

func (r *Resolver) Resolve(name string) (string, error) {
	return r.ResolveWithPrompt(name, true)
}

// ResolveWithPrompt can forbid terminal input while still accepting a cached
// prompt credential. Policy failures do not poison resolutions allowing input.
func (r *Resolver) ResolveWithPrompt(name string, allowPrompt bool) (string, error) {
	for {
		r.mu.Lock()
		if previous := r.values[name]; previous != nil {
			r.mu.Unlock()
			<-previous.done
			if allowPrompt && errors.Is(previous.err, errPromptDisabled) {
				continue
			}
			return previous.value, previous.err
		}
		entry := &result{done: make(chan struct{})}
		r.values[name] = entry
		r.mu.Unlock()
		entry.value, entry.err = r.resolve(name, allowPrompt)
		if errors.Is(entry.err, errPromptDisabled) {
			r.mu.Lock()
			delete(r.values, name)
			r.mu.Unlock()
		}
		close(entry.done)
		return entry.value, entry.err
	}
}

func (r *Resolver) resolve(name string, allowPrompt bool) (string, error) {
	spec, ok := r.specs[name]
	if !ok {
		return "", fmt.Errorf("credential %q is not defined", name)
	}
	cached := config.IsKeyringCacheBackend(spec.Cache.Backend)
	if cached {
		if r.namespace == "" {
			return "", fmt.Errorf("credential %q: cache requires a config namespace", name)
		}
		if r.refresh {
			if err := r.cache.Delete(r.namespace, name, spec); err != nil {
				return "", fmt.Errorf("credential %q: clear cache: %w", name, err)
			}
		} else {
			value, found, err := r.cache.Get(r.namespace, name, spec)
			if err != nil {
				return "", fmt.Errorf("credential %q: cache: %w", name, err)
			}
			if found {
				return value, nil
			}
		}
	}
	if !allowPrompt && strings.EqualFold(strings.TrimSpace(spec.Provider), "prompt") {
		return "", fmt.Errorf("credential %q: %w", name, errPromptDisabled)
	}
	value, err := r.resolveProvider(name, spec)
	if err != nil {
		return "", err
	}
	if err := validateValue(value); err != nil {
		return "", fmt.Errorf("credential %q: %w", name, err)
	}
	if cached {
		if err := r.cache.Put(r.namespace, name, spec, value); err != nil {
			return "", fmt.Errorf("credential %q: cache: %w", name, err)
		}
	}
	return value, nil
}

// Clear never contacts providers. Removed definitions expire at their old TTL.
func (r *Resolver) Clear() error {
	if r.namespace == "" {
		return fmt.Errorf("cache requires a config namespace")
	}
	for name, spec := range r.specs {
		if config.IsKeyringCacheBackend(spec.Cache.Backend) {
			if err := r.cache.Delete(r.namespace, name, spec); err != nil {
				return fmt.Errorf("credential %q: clear cache: %w", name, err)
			}
		}
	}
	return nil
}

// Close drops references after callers finish; Go strings cannot be reliably erased.
func (r *Resolver) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	clear(r.values)
	clear(r.specs)
}

func validateValue(value string) error {
	if value == "" {
		return fmt.Errorf("provider returned an empty credential")
	}
	if strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("credential contains a terminal line delimiter or NUL")
	}
	return nil
}
