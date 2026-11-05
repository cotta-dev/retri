//go:build linux

package cli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/cotta-dev/retri/internal/config"
	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

func TestDisabledFallbackPromptsOnTerminal(t *testing.T) {
	if os.Getenv("RETRI_NO_PROMPT_HELPER") == "1" {
		off := false
		cfg := config.Config{Defaults: config.GlobalOptions{CommonFields: config.CommonFields{PromptPassword: &off, PromptSecret: &off}}}
		got, err := resolveTargetCredentials([]config.ResolvedHost{{HostConfig: config.HostConfig{Host: "test"}}}, cfg, Options{})
		if err != nil || len(got) != 1 || got[0].Password != "" || got[0].Secret != "" {
			t.Fatal("unset credentials must be skipped", err)
		}
		return
	}
	master, terminal, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = master.Close(); _ = terminal.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDisabledFallbackPromptsOnTerminal$")
	cmd.Env = []string{"RETRI_NO_PROMPT_HELPER=1", "HOME=" + t.TempDir()}
	cmd.Stdin = terminal
	out, err := cmd.CombinedOutput()
	if err != nil || strings.Contains(string(out), "leave blank to skip") {
		t.Fatalf("unexpected prompt: %v\n%s", err, out)
	}
}

func TestCredentialCacheInitWithoutSSHOrLogs(t *testing.T) {
	if os.Getenv("RETRI_CACHE_INIT_HELPER") == "1" {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if _, err := unix.KeyctlInt(unix.KEYCTL_JOIN_SESSION_KEYRING, 0, 0, 0, 0); err != nil {
			if errors.Is(err, unix.EPERM) || errors.Is(err, unix.EACCES) || errors.Is(err, unix.ENOSYS) {
				os.Exit(77)
			}
			t.Fatal(err)
		}
		path := os.Getenv("RETRI_TEST_CONFIG")
		os.Args = []string{"retri", "-c", path, "-i"}
		Run("test", nil, "")
		// Confirm a later normal resolution reads the cache despite disabled
		// prompts and a removed source environment variable.
		if err := os.Unsetenv("RETRI_TEST_LOGIN"); err != nil {
			t.Fatal(err)
		}
		if err := os.Unsetenv("RETRI_TEST_SUDO"); err != nil {
			t.Fatal(err)
		}
		cfg := config.Load(path, "")
		got, err := resolveTargetCredentials([]config.ResolvedHost{{}}, cfg, Options{ConfigFile: path})
		if err != nil || len(got) != 1 || got[0].Password != "test-login" || got[0].Secret != "test-sudo" {
			t.Fatal("initialized cache not reusable", err)
		}
		return
	}
	home := t.TempDir()
	path := filepath.Join(home, "config.yaml")
	content := "credentials:\n  login:\n    provider: env\n    ref: RETRI_TEST_LOGIN\n    cache: {backend: session-keyring, ttl: 1s}\n  sudo:\n    provider: env\n    ref: RETRI_TEST_SUDO\n    cache: {backend: session-keyring, ttl: 1s}\ndefaults:\n  password_credential: login\n  secret_credential: sudo\n  prompt_password: false\n  prompt_secret: false\n  update_check: true\n  command: uptime\nhosts:\n  - host: no-ssh.invalid\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCredentialCacheInitWithoutSSHOrLogs$", "-test.v")
	cmd.Env = []string{"RETRI_CACHE_INIT_HELPER=1", "RETRI_TEST_CONFIG=" + path, "RETRI_TEST_LOGIN=test-login", "RETRI_TEST_SUDO=test-sudo", "HOME=" + home, "PATH=" + home, "SHELL=" + filepath.Join(home, "no-shell")}
	out, err := cmd.CombinedOutput()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 77 {
			t.Skip("kernel session keyring unavailable")
		}
		t.Fatalf("init failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "Credential cache initialized") || strings.Contains(string(out), "test-login") || strings.Contains(string(out), "test-sudo") {
		t.Fatalf("unexpected output: %s", out)
	}
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 1 || entries[0].Name() != "config.yaml" {
		t.Fatal("unexpected logs, config or update-check files", err)
	}
}
