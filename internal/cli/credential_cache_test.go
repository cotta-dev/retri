package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cotta-dev/retri/internal/config"
)

func TestCredentialCacheInitNames(t *testing.T) {
	t.Setenv("RETRI_SSH_PASSWORD", "")
	t.Setenv("RETRI_SSH_SECRET", "")
	cache := config.CredentialCacheConfig{Backend: "session-keyring"}
	cfg := config.Config{
		Credentials: map[string]config.CredentialSpec{
			"login":  {Provider: "prompt", Cache: cache},
			"sudo":   {Provider: "prompt", Cache: cache},
			"shared": {Provider: "prompt", Cache: cache},
			"unused": {Provider: "prompt"},
		},
		Defaults: config.GlobalOptions{CommonFields: config.CommonFields{PasswordCredential: "login", SecretCredential: "sudo"}},
		Groups:   []config.GroupConfig{{Name: "web", Hosts: []string{"node-[01-02]"}, CommonFields: config.CommonFields{PasswordCredential: "shared", SecretCredential: "shared"}}},
	}
	for _, tc := range []struct {
		name string
		opts Options
		args []string
		want []string
	}{
		{"defaults", Options{}, nil, []string{"login", "sudo"}},
		{"group-without-commands", Options{Group: "web"}, nil, []string{"shared"}},
		{"host", Options{Host: "node-01"}, nil, []string{"shared"}},
		{"positional-host", Options{}, []string{"node-02"}, []string{"shared"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := credentialCacheInitNames(cfg, tc.opts, tc.args)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("names = %v, err = %v", got, err)
			}
		})
	}
	for _, opts := range []Options{{Group: "missing"}, {Password: "literal"}, {Secret: "literal"}} {
		if _, err := credentialCacheInitNames(cfg, opts, nil); err == nil {
			t.Fatal("invalid selection accepted")
		}
	}
	cfg.Credentials["sudo"] = config.CredentialSpec{Provider: "prompt"}
	if _, err := credentialCacheInitNames(cfg, Options{}, nil); err == nil {
		t.Fatal("uncached credential accepted")
	}
	if _, err := credentialCacheInitNames(config.Config{}, Options{}, nil); err == nil {
		t.Fatal("empty selection accepted")
	}
}

func TestCLIHelperProcess(t *testing.T) {
	if os.Getenv("RETRI_CLI_HELPER") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{"retri"}, os.Args[i+1:]...)
			Run("test", []byte("defaults: {}\n"), "config help")
			os.Exit(0)
		}
	}
	os.Exit(2)
}

func TestCredentialCacheInitCLIValidationAndHelp(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		want    string
		success bool
	}{
		{"short-help", []string{"-h"}, "--credential-cache-init", true},
		{"long-help", []string{"--help"}, "--credential-cache-init", true},
		{"command-conflict", []string{"--credential-cache-init", "--command", "uptime"}, "cannot be combined", false},
		{"file-conflict", []string{"--credential-cache-init", "-f", "absent.txt"}, "cannot be combined", false},
		{"clear-conflict", []string{"--credential-cache-init", "--credential-cache-clear"}, "cannot be combined", false},
		{"update-conflict", []string{"--credential-cache-init", "--update"}, "cannot be combined", false},
		{"target-conflict", []string{"--credential-cache-init", "-H", "node", "extra"}, "only one target", false},
		{"no-credentials", []string{"--credential-cache-init"}, "no named SSH/sudo credentials", false},
		{"short-no-credentials", []string{"-i"}, "no named SSH/sudo credentials", false},
		{"short-command-conflict", []string{"-i", "--command", "uptime"}, "cannot be combined", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			args := append([]string{"-test.run=^TestCLIHelperProcess$", "--"}, tc.args...)
			cmd := exec.CommandContext(ctx, os.Args[0], args...)
			cmd.Env = []string{"RETRI_CLI_HELPER=1", "HOME=" + home, "PATH=" + home, "SHELL=" + filepath.Join(home, "no-shell")}
			out, err := cmd.CombinedOutput()
			if (err == nil) != tc.success || !strings.Contains(string(out), tc.want) {
				t.Fatalf("err=%v, output=%s", err, out)
			}
			entries, err := os.ReadDir(home)
			if err != nil || len(entries) != 0 {
				t.Fatal("unexpected filesystem side effects", err)
			}
		})
	}
}
