package config

import "os"

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
