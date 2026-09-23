package config

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/cotta-dev/retri/internal/logencoding"
)

// Validate checks the config for errors that would cause runtime failures.
func (c *Config) Validate() error {
	if err := validateCredentials(c.Credentials); err != nil {
		return err
	}
	if err := validateCommonCredentials("defaults", c.Defaults.CommonFields, c.Credentials); err != nil {
		return err
	}
	if err := validateLogEncoding("defaults", c.Defaults.LogEncoding); err != nil {
		return err
	}

	deviceNames := make([]string, 0, len(c.DeviceTypes))
	for name := range c.DeviceTypes {
		deviceNames = append(deviceNames, name)
	}
	sort.Strings(deviceNames)
	for _, name := range deviceNames {
		dt := c.DeviceTypes[name]
		if dt.PromptRegex != "" {
			if _, err := regexp.Compile(dt.PromptRegex); err != nil {
				return fmt.Errorf("device_type '%s': invalid prompt_regex '%s': %w", name, dt.PromptRegex, err)
			}
		}
		label := fmt.Sprintf("device_type %q", name)
		if err := validateCommonCredentials(label, dt.CommonFields, c.Credentials); err != nil {
			return err
		}
		if err := validateLogEncoding(label, dt.LogEncoding); err != nil {
			return err
		}
	}
	for i, group := range c.Groups {
		label := fmt.Sprintf("groups[%d]", i)
		if group.Name != "" {
			label = fmt.Sprintf("group %q", group.Name)
		}
		if err := validateCommonCredentials(label, group.CommonFields, c.Credentials); err != nil {
			return err
		}
		if err := validateLogEncoding(label, group.LogEncoding); err != nil {
			return err
		}
	}
	for i, host := range c.Hosts {
		label := fmt.Sprintf("hosts[%d]", i)
		if host.Host != "" {
			label = fmt.Sprintf("host %q", host.Host)
		}
		if err := validateCommonCredentials(label, host.CommonFields, c.Credentials); err != nil {
			return err
		}
		if err := validateLogEncoding(label, host.LogEncoding); err != nil {
			return err
		}
	}
	return nil
}

func validateCredentials(credentials map[string]CredentialSpec) error {
	names := make([]string, 0, len(credentials))
	for name := range credentials {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		spec := credentials[name]
		provider := strings.ToLower(strings.TrimSpace(spec.Provider))
		if provider == "" {
			return fmt.Errorf("credential %q: provider is required", name)
		}
		switch provider {
		case "prompt":
		case "env", "keyring", "bitwarden":
			if strings.TrimSpace(spec.Ref) == "" {
				return fmt.Errorf("credential %q: provider %q requires ref", name, provider)
			}
		case "literal":
			if spec.Value == "" {
				return fmt.Errorf("credential %q: provider %q requires value", name, provider)
			}
		default:
			return fmt.Errorf("credential %q: unsupported provider %q", name, spec.Provider)
		}

		backend := strings.ToLower(strings.TrimSpace(spec.Cache.Backend))
		switch backend {
		case "", "none":
			if spec.Cache.TTL != "" {
				return fmt.Errorf("credential %q: cache ttl requires a cache backend", name)
			}
		case "session-keyring":
			if spec.Cache.TTL != "" {
				d, err := time.ParseDuration(spec.Cache.TTL)
				if err != nil || d <= 0 {
					return fmt.Errorf("credential %q: invalid cache ttl %q", name, spec.Cache.TTL)
				}
			}
		default:
			return fmt.Errorf("credential %q: unsupported cache backend %q", name, spec.Cache.Backend)
		}
	}
	return nil
}

func validateCommonCredentials(section string, fields CommonFields, credentials map[string]CredentialSpec) error {
	if fields.Password != "" && fields.PasswordCredential != "" {
		return fmt.Errorf("%s: password and password_credential are mutually exclusive", section)
	}
	if fields.Secret != "" && fields.SecretCredential != "" {
		return fmt.Errorf("%s: secret and secret_credential are mutually exclusive", section)
	}
	if fields.PasswordCredential != "" {
		if _, ok := credentials[fields.PasswordCredential]; !ok {
			return fmt.Errorf("%s: password_credential %q is not defined", section, fields.PasswordCredential)
		}
	}
	if fields.SecretCredential != "" {
		if _, ok := credentials[fields.SecretCredential]; !ok {
			return fmt.Errorf("%s: secret_credential %q is not defined", section, fields.SecretCredential)
		}
	}
	return nil
}

func validateLogEncoding(section, value string) error {
	if _, err := logencoding.Lookup(os.ExpandEnv(value)); err != nil {
		return fmt.Errorf("%s: %w", section, err)
	}
	return nil
}
