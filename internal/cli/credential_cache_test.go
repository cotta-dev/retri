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
			"sudo":   {Provider: "prompt", Cache: config.CredentialCacheConfig{Backend: "user-keyring"}},
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
		{"literal-password-skipped", Options{Password: "literal"}, nil, []string{"sudo"}},
		{"literal-secret-skipped", Options{Secret: "literal"}, nil, []string{"login"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := credentialCacheInitNames(cfg, tc.opts, tc.args)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("names = %v, err = %v", got, err)
			}
		})
	}
	if _, err := credentialCacheInitNames(cfg, Options{Group: "missing"}, nil); err == nil {
		t.Fatal("invalid group accepted")
	}
	cfg.Credentials["sudo"] = config.CredentialSpec{Provider: "prompt"}
	if got, err := credentialCacheInitNames(cfg, Options{}, nil); err != nil || !reflect.DeepEqual(got, []string{"login"}) {
		t.Fatal("uncached credential must be skipped while cached login remains selected", err)
	}
	if got, err := credentialCacheInitNames(config.Config{}, Options{}, nil); err != nil || len(got) != 0 {
		t.Fatal("empty selection must succeed", err)
	}
	cfg.Defaults.PasswordCredential = "undefined"
	if _, err := credentialCacheInitNames(cfg, Options{}, nil); err == nil {
		t.Fatal("undefined credential must remain an error")
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
		{"no-credentials", []string{"--credential-cache-init"}, "nothing to initialize", true},
		{"short-no-credentials", []string{"-i"}, "nothing to initialize", true},
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

func TestCredentialCacheInitCLIUncachedSources(t *testing.T) {
	for _, tc := range []struct {
		name    string
		config  string
		args    []string
		want    string
		success bool
	}{
		{"uncached-prompt", "credentials:\n  login: {provider: prompt}\nhosts:\n  - host: test01\n    password_credential: login\n", nil, "nothing to initialize", true},
		{"literal", "defaults: {password: test-password, secret: test-secret}\n", nil, "nothing to initialize", true},
		{"cli-override", "credentials:\n  login: {provider: prompt, cache: {backend: session-keyring}}\ndefaults: {password_credential: login}\n", []string{"-p", "test-password"}, "nothing to initialize", true},
		{"uncached-env-unset", "credentials:\n  login: {provider: env, ref: RETRI_TEST_MISSING}\ndefaults: {password_credential: login}\n", nil, "nothing to initialize", true},
		{"cached-provider-failure", "credentials:\n  login: {provider: env, ref: RETRI_TEST_MISSING, cache: {backend: session-keyring}}\ndefaults: {password_credential: login}\n", nil, "Failed to initialize credential cache", false},
		{"undefined", "defaults: {password_credential: undefined}\n", nil, "Config validation failed", false},
		{"multiple-selectors", "defaults: {}\n", []string{"-g", "missing"}, "only one target", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			home := filepath.Join(root, "home")
			if err := os.Mkdir(home, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "config.yaml")
			if err := os.WriteFile(path, []byte(tc.config), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			args := append([]string{"-test.run=^TestCLIHelperProcess$", "--", "-i", "-c", path, "-H", "test01"}, tc.args...)
			cmd := exec.CommandContext(ctx, os.Args[0], args...)
			cmd.Env = []string{"RETRI_CLI_HELPER=1", "HOME=" + home, "PATH=" + home}
			out, err := cmd.CombinedOutput()
			if (err == nil) != tc.success || !strings.Contains(string(out), tc.want) {
				t.Fatalf("err=%v, output=%s", err, out)
			}
			if tc.success && strings.Contains(string(out), "Credential cache initialized") {
				t.Fatal("skipped initialization reported as completed")
			}
			if strings.Contains(string(out), "test-password") || strings.Contains(string(out), "test-secret") {
				t.Fatal("credential value exposed")
			}
			entries, err := os.ReadDir(home)
			if err != nil || len(entries) != 0 {
				t.Fatal("unexpected filesystem side effects", err)
			}
		})
	}
}
