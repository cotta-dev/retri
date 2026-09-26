package credentials

import (
	"github.com/cotta-dev/retri/internal/config"
	"os"
	"strings"
)

// ChildEnvironment removes all configured credential inputs, including legacy
// expansions. It keeps ordinary SSH/proxy/locale settings. Only bw gets BW_*.
func ChildEnvironment(cfg config.Config, env []string, bitwarden bool) []string {
	blocked := map[string]bool{"RETRI_SSH_PASSWORD": true, "RETRI_SSH_SECRET": true}
	expansions := func(value string) {
		_ = os.Expand(value, func(name string) string { blocked[name] = true; return "" })
	}
	common := func(fields config.CommonFields) { expansions(fields.Password); expansions(fields.Secret) }
	common(cfg.Defaults.CommonFields)
	for _, d := range cfg.DeviceTypes {
		common(d.CommonFields)
	}
	for _, g := range cfg.Groups {
		common(g.CommonFields)
	}
	for _, h := range cfg.Hosts {
		common(h.CommonFields)
	}
	for _, spec := range cfg.Credentials {
		switch strings.ToLower(strings.TrimSpace(spec.Provider)) {
		case "env":
			blocked[spec.Ref] = true
		case "literal":
			expansions(spec.Value)
		}
	}
	out := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if blocked[name] || (!bitwarden && (strings.HasPrefix(name, "BW_") || name == "BWS_ACCESS_TOKEN")) {
			continue
		}
		out = append(out, entry)
	}
	return out
}
