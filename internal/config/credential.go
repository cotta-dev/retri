package config

import (
	"os"
	"strings"
)

const (
	CacheBackendSessionKeyring = "session-keyring"
	CacheBackendUserKeyring    = "user-keyring"
)

// IsKeyringCacheBackend reports whether a backend retains credentials between
// invocations. Session and user caches have distinct sharing scopes.
func IsKeyringCacheBackend(backend string) bool {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case CacheBackendSessionKeyring, CacheBackendUserKeyring:
		return true
	default:
		return false
	}
}

// ResolveCredentialPrompts selects prompt policy independently of credential
// sources. Nil inherits; the default for both fields is true.
func ResolveCredentialPrompts(rh ResolvedHost, defaults GlobalOptions) (password, secret bool) {
	password, secret = true, true
	apply := func(fields CommonFields) {
		if fields.PromptPassword != nil {
			password = *fields.PromptPassword
		}
		if fields.PromptSecret != nil {
			secret = *fields.PromptSecret
		}
	}
	apply(defaults.CommonFields)
	for _, group := range rh.GroupConfigs {
		apply(group.CommonFields)
	}
	apply(rh.DeviceConfig.CommonFields)
	apply(rh.HostConfig.CommonFields)
	return
}

// CredentialSource identifies either a literal credential value or a named
// credential definition. Exactly one field should be set.
type CredentialSource struct {
	Literal    string
	Credential string
	Set        bool
}

// ResolveCredentialSources resolves password and secret sources using the same
// configuration priority as other CommonFields. A value and a named credential
// are mutually exclusive at each layer (enforced by Config.Validate).
//
// Priority (low -> high): defaults < groups < device_types < hosts < CLI.
// The environment fallback is used only when no config source was selected.
func ResolveCredentialSources(rh ResolvedHost, defaults GlobalOptions, cliPassword, cliSecret string) (password, secret CredentialSource) {
	password = resolveCredentialSource(
		rh,
		defaults,
		func(fp FieldProvider) string { return fp.Common().Password },
		func(fp FieldProvider) string { return fp.Common().PasswordCredential },
		"RETRI_SSH_PASSWORD",
		cliPassword,
	)
	secret = resolveCredentialSource(
		rh,
		defaults,
		func(fp FieldProvider) string { return fp.Common().Secret },
		func(fp FieldProvider) string { return fp.Common().SecretCredential },
		"RETRI_SSH_SECRET",
		cliSecret,
	)
	return
}

func resolveCredentialSource(
	rh ResolvedHost,
	defaults GlobalOptions,
	getLiteral func(FieldProvider) string,
	getCredential func(FieldProvider) string,
	envKey,
	cliValue string,
) CredentialSource {
	var source CredentialSource
	apply := func(fp FieldProvider) {
		literal := getLiteral(fp)
		credential := getCredential(fp)
		if literal != "" {
			source = CredentialSource{Literal: literal, Set: true}
		}
		if credential != "" {
			source = CredentialSource{Credential: credential, Set: true}
		}
	}

	apply(&defaults)
	for i := range rh.GroupConfigs {
		apply(&rh.GroupConfigs[i])
	}
	apply(&rh.DeviceConfig)
	apply(&rh.HostConfig)

	if source.Literal != "" {
		source.Literal = os.ExpandEnv(source.Literal)
		// Preserve the legacy behavior of password: "${VAR}": if expansion
		// produces an empty value, treat it as unset so the conventional
		// RETRI_* fallback (and ultimately the interactive fallback) can apply.
		if source.Literal == "" {
			source = CredentialSource{}
		}
	}
	if !source.Set && envKey != "" {
		if value := os.Getenv(envKey); value != "" {
			source = CredentialSource{Literal: value, Set: true}
		}
	}
	if cliValue != "" {
		source = CredentialSource{Literal: cliValue, Set: true}
	}
	return source
}
