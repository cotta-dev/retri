package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCredentialPromptPolicyLayers(t *testing.T) {
	var cfg Config
	if err := yaml.Unmarshal([]byte("defaults:\n  prompt_password: false\n  prompt_secret: false\n"), &cfg); err != nil {
		t.Fatal(err)
	}
	rh := ResolvedHost{}
	check := func(wantPassword, wantSecret bool) {
		t.Helper()
		p, s := ResolveCredentialPrompts(rh, cfg.Defaults)
		if p != wantPassword || s != wantSecret {
			t.Fatalf("prompt policy = %v, %v", p, s)
		}
	}
	check(false, false)
	on, off := true, false
	rh.GroupConfigs = []GroupConfig{{CommonFields: CommonFields{PromptPassword: &on}}, {CommonFields: CommonFields{PromptPassword: &off, PromptSecret: &on}}}
	check(false, true)
	rh.DeviceConfig.CommonFields = CommonFields{PromptPassword: &on, PromptSecret: &off}
	check(true, false)
	rh.HostConfig.CommonFields = CommonFields{PromptPassword: &off, PromptSecret: &on}
	check(false, true)
	p, s := ResolveCredentialPrompts(ResolvedHost{}, GlobalOptions{})
	if !p || !s {
		t.Fatal("omitted settings must allow prompts")
	}
}

func TestCredentialLayerMatrix(t *testing.T) {
	t.Setenv("RETRI_SSH_PASSWORD", "env")
	t.Setenv("RETRI_SSH_SECRET", "env")
	fields := func(literal, named string) CommonFields {
		return CommonFields{Password: literal, Secret: literal, PasswordCredential: named, SecretCredential: named}
	}
	defaults := GlobalOptions{CommonFields: fields("default", "")}
	rh := ResolvedHost{GroupConfigs: []GroupConfig{{CommonFields: fields("group1", "")}, {CommonFields: fields("", "group2")}}}
	check := func(literal, named, cli string) {
		t.Helper()
		p, s := ResolveCredentialSources(rh, defaults, cli, cli)
		for _, v := range []CredentialSource{p, s} {
			if v.Literal != literal || v.Credential != named || !v.Set {
				t.Fatalf("incorrect source: %+v", v)
			}
		}
	}
	check("", "group2", "")
	rh.DeviceConfig.CommonFields = fields("device", "")
	check("device", "", "")
	rh.HostConfig.CommonFields = fields("", "host")
	check("", "host", "")
	check("cli", "", "cli")
	rh = ResolvedHost{}
	check("default", "", "")
	defaults = GlobalOptions{}
	check("env", "", "")
}

func TestRejectInvalidCredentialDefinitions(t *testing.T) {
	for _, spec := range []CredentialSpec{
		{Provider: "env", Ref: "ENV", Field: "password"},
		{Provider: "prompt", Ref: "ignored"},
		{Provider: "literal", Value: "x", Prompt: "ignored"},
		{Provider: "bitwarden", Ref: "--session"},
		{Provider: "bitwarden", Ref: "item", Cache: CredentialCacheConfig{Backend: "session-keyring"}},
		{Provider: "prompt", Cache: CredentialCacheConfig{Backend: "session-keyring", TTL: "1ms"}},
		{Provider: "prompt", Cache: CredentialCacheConfig{Backend: "session-keyring", TTL: "25h"}},
		{Provider: "prompt", Cache: CredentialCacheConfig{Backend: "user-keyring", TTL: "25h"}},
		{Provider: "bitwarden", Ref: "item", Cache: CredentialCacheConfig{Backend: "user-keyring"}},
	} {
		cfg := Config{Credentials: map[string]CredentialSpec{"test": spec}}
		if err := cfg.Validate(); err == nil {
			t.Fatalf("invalid definition accepted: provider=%s", spec.Provider)
		}
	}
}

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
	spec := cfg.Credentials["sudo"]
	spec.Cache.Backend = "user-keyring"
	cfg.Credentials["sudo"] = spec
	if err := cfg.Validate(); err != nil {
		t.Fatal("user keyring rejected", err)
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
