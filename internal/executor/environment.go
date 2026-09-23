package executor

import "strings"

var sensitiveChildEnvironment = map[string]struct{}{
	"RETRI_SSH_PASSWORD": {},
	"RETRI_SSH_SECRET":   {},
	"BW_SESSION":         {},
}

// sanitizedEnvironment removes known credential-bearing variables before
// launching SSH or a recorded shell. Provider-specific processes (such as bw)
// are launched separately and retain the environment they need.
func sanitizedEnvironment(env []string) []string {
	out := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if _, sensitive := sensitiveChildEnvironment[name]; sensitive {
			continue
		}
		out = append(out, entry)
	}
	return out
}
