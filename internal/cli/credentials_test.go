package cli

import (
	"github.com/cotta-dev/retri/internal/config"
	"os"
	"testing"
)

func TestNonTTYUnsetCredentialsRemainOptional(t *testing.T) {
	t.Setenv("RETRI_SSH_PASSWORD", "")
	t.Setenv("RETRI_SSH_SECRET", "")
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	old := os.Stdin
	os.Stdin = f
	defer func() { os.Stdin = old }()
	hosts := []config.ResolvedHost{{HostConfig: config.HostConfig{Host: "test"}}}
	got, err := resolveTargetCredentials(hosts, config.Config{}, Options{})
	if err != nil || len(got) != 1 || got[0].Password != "" || got[0].Secret != "" {
		t.Fatalf("non-TTY regression: %v", err)
	}
	cfg := config.Config{Credentials: map[string]config.CredentialSpec{"required": {Provider: "prompt"}}, Defaults: config.GlobalOptions{CommonFields: config.CommonFields{PasswordCredential: "required"}}}
	if _, err := resolveTargetCredentials(hosts, cfg, Options{}); err == nil {
		t.Fatal("explicit prompt must fail without TTY")
	}
	if _, err := resolveTargetCredentials(hosts, cfg, Options{Password: "override"}); err != nil {
		t.Fatal("CLI must bypass prompt", err)
	}
}
