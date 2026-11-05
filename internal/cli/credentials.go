package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/term"

	"github.com/cotta-dev/retri/internal/config"
	credentialresolver "github.com/cotta-dev/retri/internal/credentials"
)

type resolvedHostCredentials struct {
	Password string
	Secret   string
}

func validateCredentialCacheInitOptions(opts Options, remaining []string) error {
	if !opts.CredentialCacheInit {
		return nil
	}
	if opts.CredentialCacheClear || opts.Update || opts.Command != "" || opts.CommandFile != "" {
		return fmt.Errorf("--credential-cache-init cannot be combined with --credential-cache-clear, --update, --command or --command-file")
	}
	selectors := 0
	if opts.Host != "" {
		selectors++
	}
	if opts.Group != "" {
		selectors++
	}
	selectors += len(remaining)
	if selectors > 1 {
		return fmt.Errorf("--credential-cache-init accepts only one target: -H host, -g group or hostname")
	}
	return nil
}

// Plan all selected sources before accessing any provider or modifying caches.
// Without a selector, only defaults are selected; unused definitions are ignored.
func credentialCacheInitNames(cfg config.Config, opts Options, remaining []string) ([]string, error) {
	host := opts.Host
	if len(remaining) == 1 {
		host = remaining[0]
	}
	targets := []config.ResolvedHost{{}}
	if host != "" || opts.Group != "" {
		config.ExpandHostsInConfig(&cfg)
		targets = config.ResolveTargets(cfg, host, opts.Group)
		if len(targets) == 0 {
			return nil, fmt.Errorf("no targets selected for credential cache initialization")
		}
	}
	names := make(map[string]bool)
	for _, target := range targets {
		password, secret := config.ResolveCredentialSources(target, cfg.Defaults, opts.Password, opts.Secret)
		for _, source := range []config.CredentialSource{password, secret} {
			if !source.Set {
				continue
			}
			if source.Credential == "" {
				return nil, fmt.Errorf("cache initialization requires named credentials; use password_credential/secret_credential instead of literal, environment or CLI overrides")
			}
			spec, ok := cfg.Credentials[source.Credential]
			if !ok || !strings.EqualFold(strings.TrimSpace(spec.Cache.Backend), "session-keyring") {
				return nil, fmt.Errorf("credential %q requires cache.backend: session-keyring", source.Credential)
			}
			names[source.Credential] = true
		}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("no named SSH/sudo credentials selected; configure password_credential or secret_credential")
	}
	selected := make([]string, 0, len(names))
	for name := range names {
		selected = append(selected, name)
	}
	sort.Strings(selected)
	return selected, nil
}

func initializeCredentialCache(cfg config.Config, opts Options, remaining []string) error {
	names, err := credentialCacheInitNames(cfg, opts, remaining)
	if err != nil {
		return err
	}
	r := credentialresolver.NewResolver(cfg.Credentials)
	defer r.Close()
	r.Configure(credentialNamespace(opts.ConfigFile), opts.CredentialCacheRefresh, credentialresolver.ChildEnvironment(cfg, os.Environ(), true))
	// This explicit initialization operation permits prompts even when normal
	// execution disables them. Resolve reuses valid cache unless refresh is set.
	for _, name := range names {
		if _, err := r.Resolve(name); err != nil {
			return err
		}
	}
	return nil
}

func resolveTargetCredentials(targets []config.ResolvedHost, cfg config.Config, opts Options) ([]resolvedHostCredentials, error) {
	resolver := credentialresolver.NewResolver(cfg.Credentials)
	defer resolver.Close()
	resolver.Configure(credentialNamespace(opts.ConfigFile), opts.CredentialCacheRefresh, credentialresolver.ChildEnvironment(cfg, os.Environ(), true))
	resolved := make([]resolvedHostCredentials, len(targets))
	var missingPasswordHosts, missingSecretHosts []string
	missingPassword := make([]bool, len(targets))
	missingSecret := make([]bool, len(targets))

	for i, rh := range targets {
		passwordSource, secretSource := config.ResolveCredentialSources(rh, cfg.Defaults, opts.Password, opts.Secret)
		promptPassword, promptSecret := config.ResolveCredentialPrompts(rh, cfg.Defaults)

		password, passwordSet, err := resolveCredentialSource(passwordSource, resolver, promptPassword)
		if err != nil {
			return nil, fmt.Errorf("host %q SSH password: %w", rh.HostConfig.Host, err)
		}
		secret, secretSet, err := resolveCredentialSource(secretSource, resolver, promptSecret)
		if err != nil {
			return nil, fmt.Errorf("host %q sudo/enable secret: %w", rh.HostConfig.Host, err)
		}

		if !passwordSet && promptPassword {
			missingPassword[i] = true
			missingPasswordHosts = append(missingPasswordHosts, rh.HostConfig.Host)
		}
		if !secretSet && promptSecret {
			missingSecret[i] = true
			missingSecretHosts = append(missingSecretHosts, rh.HostConfig.Host)
		}
		resolved[i] = resolvedHostCredentials{Password: password, Secret: secret}
	}

	fallbackPassword, err := promptFallbackCredential("SSH password", missingPasswordHosts)
	if err != nil {
		return nil, err
	}
	fallbackSecret, err := promptFallbackCredential("Sudo Secret", missingSecretHosts)
	if err != nil {
		return nil, err
	}

	for i := range targets {
		value := resolved[i]
		if missingPassword[i] {
			value.Password = fallbackPassword
		}
		if missingSecret[i] {
			value.Secret = fallbackSecret
		}
		resolved[i] = value
	}
	return resolved, nil
}

func resolveCredentialSource(source config.CredentialSource, resolver *credentialresolver.Resolver, allowPrompt bool) (string, bool, error) {
	if !source.Set {
		return "", false, nil
	}
	if source.Credential != "" {
		value, err := resolver.ResolveWithPrompt(source.Credential, allowPrompt)
		return value, true, err
	}
	return source.Literal, true, nil
}

func promptFallbackCredential(label string, hosts []string) (string, error) {
	if len(hosts) == 0 || !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", nil
	}
	fmt.Fprintf(os.Stderr, "[INFO] %s not set for: %s\n", label, strings.Join(hosts, ", "))
	fmt.Fprintf(os.Stderr, "%s (leave blank to skip): ", label)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	defer clear(b)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", strings.ToLower(label), err)
	}
	return string(b), nil
}

func credentialNamespace(path string) string {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		path = filepath.Join(home, ".config", config.AppName, "config.yaml")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	if canonical, err := filepath.EvalSymlinks(absolute); err == nil {
		return canonical
	}
	return absolute
}
