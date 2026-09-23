package config

import "testing"

func TestResolveCredentialSourcesPriority(t *testing.T) {
	rh := ResolvedHost{
		HostConfig: HostConfig{CommonFields: CommonFields{PasswordCredential: "host-password"}},
		GroupConfigs: []GroupConfig{
			{CommonFields: CommonFields{PasswordCredential: "group-password"}},
		},
		DeviceConfig: DeviceConfig{CommonFields: CommonFields{PasswordCredential: "device-password"}},
	}
	defaults := GlobalOptions{CommonFields: CommonFields{PasswordCredential: "default-password"}}

	password, _ := ResolveCredentialSources(rh, defaults, "", "")
	if password.Credential != "host-password" || password.Literal != "" {
		t.Fatalf("password source = %#v", password)
	}
}

func TestResolveCredentialSourcesHigherLiteralOverridesLowerCredential(t *testing.T) {
	rh := ResolvedHost{
		HostConfig: HostConfig{CommonFields: CommonFields{Password: "host-literal"}},
	}
	defaults := GlobalOptions{CommonFields: CommonFields{PasswordCredential: "default-password"}}

	password, _ := ResolveCredentialSources(rh, defaults, "", "")
	if password.Literal != "host-literal" || password.Credential != "" {
		t.Fatalf("password source = %#v", password)
	}
}

func TestResolveCredentialSourcesCLIOverridesCredential(t *testing.T) {
	defaults := GlobalOptions{CommonFields: CommonFields{PasswordCredential: "named"}}
	password, _ := ResolveCredentialSources(ResolvedHost{}, defaults, "cli-value", "")
	if password.Literal != "cli-value" || password.Credential != "" {
		t.Fatalf("password source = %#v", password)
	}
}

func TestValidateCredentialReference(t *testing.T) {
	cfg := Config{
		Credentials: map[string]CredentialSpec{
			"sudo": {Provider: "prompt"},
		},
		Defaults: GlobalOptions{CommonFields: CommonFields{SecretCredential: "sudo"}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateUnknownCredentialReference(t *testing.T) {
	cfg := Config{Defaults: GlobalOptions{CommonFields: CommonFields{SecretCredential: "missing"}}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected unknown credential validation error")
	}
}

func TestValidateMutuallyExclusiveCredentialSettings(t *testing.T) {
	cfg := Config{Defaults: GlobalOptions{CommonFields: CommonFields{
		Password:           "legacy",
		PasswordCredential: "named",
	}}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected mutually exclusive password settings error")
	}
}

func TestValidateCredentialCacheTTL(t *testing.T) {
	cfg := Config{Credentials: map[string]CredentialSpec{
		"sudo": {
			Provider: "prompt",
			Cache: CredentialCacheConfig{
				Backend: "session-keyring",
				TTL:     "8h",
			},
		},
	}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestResolveCredentialSourcesEmptyLegacyExpansionFallsBack(t *testing.T) {
	t.Setenv("LEGACY_PASSWORD", "")
	t.Setenv("RETRI_SSH_PASSWORD", "fallback-password")
	defaults := GlobalOptions{CommonFields: CommonFields{Password: "${LEGACY_PASSWORD}"}}

	password, _ := ResolveCredentialSources(ResolvedHost{}, defaults, "", "")
	if !password.Set || password.Literal != "fallback-password" || password.Credential != "" {
		t.Fatalf("password source = %#v", password)
	}
}
