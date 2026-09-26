package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"github.com/cotta-dev/retri/internal/config"
	credentialresolver "github.com/cotta-dev/retri/internal/credentials"
)

type resolvedHostCredentials struct {
	Password string
	Secret   string
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

		password, passwordSet, err := resolveCredentialSource(passwordSource, resolver)
		if err != nil {
			return nil, fmt.Errorf("host %q SSH password: %w", rh.HostConfig.Host, err)
		}
		secret, secretSet, err := resolveCredentialSource(secretSource, resolver)
		if err != nil {
			return nil, fmt.Errorf("host %q sudo/enable secret: %w", rh.HostConfig.Host, err)
		}

		if !passwordSet {
			missingPassword[i] = true
			missingPasswordHosts = append(missingPasswordHosts, rh.HostConfig.Host)
		}
		if !secretSet {
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

func resolveCredentialSource(source config.CredentialSource, resolver *credentialresolver.Resolver) (string, bool, error) {
	if !source.Set {
		return "", false, nil
	}
	if source.Credential != "" {
		value, err := resolver.Resolve(source.Credential)
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
