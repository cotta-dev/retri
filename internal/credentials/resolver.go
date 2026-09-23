package credentials

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"

	"github.com/cotta-dev/retri/internal/config"
)

const defaultSessionCacheTTL = 8 * time.Hour

type promptFunc func(label string) (string, error)
type commandFunc func(name string, args []string, stdin []byte) ([]byte, error)

// Resolver resolves named credentials and keeps an in-process cache so one
// credential is only requested once per retri invocation. Optional persistent
// session caching is controlled independently by each credential definition.
type Resolver struct {
	specs  map[string]config.CredentialSpec
	mu     sync.Mutex
	values map[string]string
	prompt promptFunc
	run    commandFunc
}

// NewResolver creates a resolver for the configured named credentials.
func NewResolver(specs map[string]config.CredentialSpec) *Resolver {
	return &Resolver{
		specs:  specs,
		values: make(map[string]string),
		prompt: readHiddenPrompt,
		run:    runCommand,
	}
}

// Resolve returns one named credential. The resolver serializes first-time
// resolution so concurrent hosts cannot trigger duplicate prompts or provider calls.
func (r *Resolver) Resolve(name string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if value, ok := r.values[name]; ok {
		return value, nil
	}

	spec, ok := r.specs[name]
	if !ok {
		return "", fmt.Errorf("credential %q is not defined", name)
	}

	if strings.EqualFold(strings.TrimSpace(spec.Cache.Backend), "session-keyring") {
		value, found, err := r.readSessionCache(name, spec)
		if err != nil {
			return "", err
		}
		if found {
			r.values[name] = value
			return value, nil
		}
	}

	value, err := r.resolveProvider(name, spec)
	if err != nil {
		return "", err
	}

	if value != "" && strings.EqualFold(strings.TrimSpace(spec.Cache.Backend), "session-keyring") {
		if err := r.writeSessionCache(name, spec, value); err != nil {
			return "", err
		}
	}

	r.values[name] = value
	return value, nil
}

func (r *Resolver) resolveProvider(name string, spec config.CredentialSpec) (string, error) {
	switch strings.ToLower(strings.TrimSpace(spec.Provider)) {
	case "prompt":
		label := strings.TrimSpace(spec.Prompt)
		if label == "" {
			label = fmt.Sprintf("Credential %q", name)
		}
		value, err := r.prompt(label)
		if err != nil {
			return "", fmt.Errorf("credential %q: prompt failed: %w", name, err)
		}
		return value, nil

	case "env":
		value, ok := os.LookupEnv(spec.Ref)
		if !ok {
			return "", fmt.Errorf("credential %q: environment variable %q is not set", name, spec.Ref)
		}
		return value, nil

	case "literal":
		return os.ExpandEnv(spec.Value), nil

	case "keyring":
		value, found, err := r.readKeyring(spec.Ref)
		if err != nil {
			return "", fmt.Errorf("credential %q: %w", name, err)
		}
		if !found {
			return "", fmt.Errorf("credential %q: keyring entry %q was not found", name, spec.Ref)
		}
		return value, nil

	case "bitwarden":
		value, err := r.readBitwarden(spec)
		if err != nil {
			return "", fmt.Errorf("credential %q: %w", name, err)
		}
		return value, nil

	default:
		return "", fmt.Errorf("credential %q: unsupported provider %q", name, spec.Provider)
	}
}

func readHiddenPrompt(label string) (string, error) {
	fmt.Fprintf(os.Stderr, "%s: ", label)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func runCommand(name string, args []string, stdin []byte) ([]byte, error) {
	cmd := exec.Command(name, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	return cmd.Output()
}

func (r *Resolver) readBitwarden(spec config.CredentialSpec) (string, error) {
	out, err := r.run("bw", []string{"get", "item", spec.Ref}, nil)
	if err != nil {
		if _, lookupErr := exec.LookPath("bw"); lookupErr != nil {
			return "", fmt.Errorf("bitwarden provider requires the 'bw' CLI")
		}
		return "", fmt.Errorf("bw get item failed: %w", err)
	}

	var item struct {
		Login *struct {
			Username string `json:"username"`
			Password string `json:"password"`
		} `json:"login"`
		Fields []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(out, &item); err != nil {
		return "", fmt.Errorf("invalid bw item response: %w", err)
	}

	field := strings.TrimSpace(spec.Field)
	if field == "" {
		field = "password"
	}
	switch strings.ToLower(field) {
	case "password":
		if item.Login == nil {
			return "", fmt.Errorf("Bitwarden item has no login password")
		}
		return item.Login.Password, nil
	case "username":
		if item.Login == nil {
			return "", fmt.Errorf("Bitwarden item has no login username")
		}
		return item.Login.Username, nil
	default:
		for _, f := range item.Fields {
			if f.Name == field {
				return f.Value, nil
			}
		}
		return "", fmt.Errorf("Bitwarden item field %q was not found", field)
	}
}

func (r *Resolver) readSessionCache(name string, spec config.CredentialSpec) (string, bool, error) {
	return r.readKeyring(cacheDescription(name, spec))
}

func (r *Resolver) writeSessionCache(name string, spec config.CredentialSpec, value string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("credential %q: session-keyring cache is supported only on Linux", name)
	}
	if _, err := exec.LookPath("keyctl"); err != nil {
		return fmt.Errorf("credential %q: session-keyring cache requires the 'keyctl' command", name)
	}

	description := cacheDescription(name, spec)
	var keyID string
	if out, err := r.run("keyctl", []string{"search", "@s", "user", description}, nil); err == nil {
		keyID = strings.TrimSpace(string(out))
	}
	if keyID != "" {
		if _, err := r.run("keyctl", []string{"pupdate", keyID}, []byte(value)); err != nil {
			return fmt.Errorf("credential %q: failed to update session-keyring cache: %w", name, err)
		}
	} else {
		out, err := r.run("keyctl", []string{"padd", "user", description, "@s"}, []byte(value))
		if err != nil {
			return fmt.Errorf("credential %q: failed to store session-keyring cache: %w", name, err)
		}
		keyID = strings.TrimSpace(string(out))
		if keyID == "" {
			return fmt.Errorf("credential %q: keyctl returned an empty key id", name)
		}
	}

	ttl := defaultSessionCacheTTL
	if spec.Cache.TTL != "" {
		parsed, err := time.ParseDuration(spec.Cache.TTL)
		if err != nil {
			return fmt.Errorf("credential %q: invalid cache ttl %q: %w", name, spec.Cache.TTL, err)
		}
		ttl = parsed
	}
	seconds := int64(ttl / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	if _, err := r.run("keyctl", []string{"timeout", keyID, strconv.FormatInt(seconds, 10)}, nil); err != nil {
		return fmt.Errorf("credential %q: failed to set session-keyring timeout: %w", name, err)
	}
	return nil
}

func (r *Resolver) readKeyring(description string) (string, bool, error) {
	if runtime.GOOS != "linux" {
		return "", false, fmt.Errorf("keyring provider is supported only on Linux")
	}
	if _, err := exec.LookPath("keyctl"); err != nil {
		return "", false, fmt.Errorf("Linux keyring support requires the 'keyctl' command")
	}

	out, err := r.run("keyctl", []string{"search", "@s", "user", description}, nil)
	if err != nil {
		return "", false, nil
	}
	keyID := strings.TrimSpace(string(out))
	if keyID == "" {
		return "", false, nil
	}
	payload, err := r.run("keyctl", []string{"pipe", keyID}, nil)
	if err != nil {
		return "", false, fmt.Errorf("failed to read keyring entry %q: %w", description, err)
	}
	return string(payload), true, nil
}

func cacheDescription(name string, spec config.CredentialSpec) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{name, spec.Provider, spec.Ref, spec.Field}, "\x00")))
	return "retri:credential:" + hex.EncodeToString(sum[:12])
}
