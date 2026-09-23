package cli

import (
	"fmt"
	"os"
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
	resolved := make([]resolvedHostCredentials, len(targets))
	var missingPasswordHosts, missingSecretHosts []string

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
			missingPasswordHosts = append(missingPasswordHosts, rh.HostConfig.Host)
		}
		if !secretSet {
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

	for i, rh := range targets {
		value := resolved[i]
		passwordSource, secretSource := config.ResolveCredentialSources(rh, cfg.Defaults, opts.Password, opts.Secret)
		if !passwordSource.Set {
			value.Password = fallbackPassword
		}
		if !secretSource.Set {
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
	if len(hosts) == 0 {
		return "", nil
	}
	fmt.Fprintf(os.Stderr, "[INFO] %s not set for: %s\n", label, strings.Join(hosts, ", "))
	fmt.Fprintf(os.Stderr, "%s (leave blank to skip): ", label)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", strings.ToLower(label), err)
	}
	return string(b), nil
}
