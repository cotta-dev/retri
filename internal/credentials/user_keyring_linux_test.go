//go:build linux

package credentials

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/cotta-dev/retri/internal/config"
	"golang.org/x/sys/unix"
)

// Each helper has its own revoked session ring. Only unique, short-lived test
// entries are written to the UID keyring; existing user entries are untouched.
func TestUserKeyringAcrossRevokedSessions(t *testing.T) {
	mode := os.Getenv("RETRI_USER_KEYRING_TEST")
	if mode == "" {
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		namespace := t.TempDir()
		run := func(mode, value string) {
			t.Helper()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, exe, "-test.run=^TestUserKeyringAcrossRevokedSessions$", "-test.v")
			cmd.Env = []string{"RETRI_USER_KEYRING_TEST=" + mode, "RETRI_TEST_NAMESPACE=" + namespace, "RETRI_TEST_VALUE=" + value}
			out, err := cmd.CombinedOutput()
			if err != nil {
				var exit *exec.ExitError
				if errors.As(err, &exit) && exit.ExitCode() == 77 {
					t.Skip("kernel user keyring unavailable")
				}
				t.Fatalf("%s: %v\n%s", mode, err, out)
			}
		}
		t.Cleanup(func() { run("clear", "") })
		run("write", "test-first")
		run("read", "test-first")
		run("session-isolation", "")
		run("refresh", "test-second")
		run("read", "test-second")
		run("clear", "")
		run("miss", "")
		run("expiry", "test-short")
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	ring, err := unix.KeyctlInt(unix.KEYCTL_JOIN_SESSION_KEYRING, 0, 0, 0, 0)
	if err != nil {
		if errors.Is(err, unix.EPERM) || errors.Is(err, unix.EACCES) || errors.Is(err, unix.ENOSYS) {
			os.Exit(77)
		}
		t.Fatal(err)
	}
	if mode == "session-isolation" {
		if _, err := unix.KeyctlInt(unix.KEYCTL_LINK, unix.KEY_SPEC_USER_KEYRING, ring, 0, 0); err != nil {
			t.Fatal(err)
		}
	} else if err := revokeKey(ring); err != nil {
		t.Fatal(err)
	}
	if _, err := unix.KeyctlGetKeyringID(unix.KEY_SPEC_USER_KEYRING, true); err != nil {
		if errors.Is(err, unix.EPERM) || errors.Is(err, unix.EACCES) || errors.Is(err, unix.ENOSYS) {
			os.Exit(77)
		}
		t.Fatal(err)
	}
	spec := config.CredentialSpec{Provider: "prompt", Cache: config.CredentialCacheConfig{Backend: "user-keyring", TTL: "30s"}}
	if mode == "session-isolation" {
		spec.Cache.Backend = "session-keyring"
	}
	if mode == "expiry" {
		spec.Cache.TTL = "1s"
	}
	r := NewResolver(map[string]config.CredentialSpec{"test-only": spec})
	r.Configure(os.Getenv("RETRI_TEST_NAMESPACE"), mode == "refresh", nil)
	defer r.Close()
	calls := 0
	r.prompt = func(string) (string, error) { calls++; return os.Getenv("RETRI_TEST_VALUE"), nil }
	switch mode {
	case "clear":
		if err := r.Clear(); err != nil {
			t.Fatal(err)
		}
	case "miss", "session-isolation":
		if _, err := r.ResolveWithPrompt("test-only", false); !errors.Is(err, errPromptDisabled) {
			t.Fatal("cache miss or backend separation failed", err)
		}
	case "write", "refresh", "expiry":
		if _, err := r.Resolve("test-only"); err != nil || calls != 1 {
			t.Fatal("failed to cache test credential", err)
		}
		description := cacheDescription(r.namespace, "test-only", spec)
		cacheRing, err := unix.KeyctlSearch(unix.KEY_SPEC_USER_KEYRING, "keyring", description, 0)
		if err != nil {
			t.Fatal(err)
		}
		valueID, err := unix.KeyctlSearch(cacheRing, "user", "value", 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range []int{cacheRing, valueID} {
			desc, err := unix.KeyctlString(unix.KEYCTL_DESCRIBE, id)
			if err != nil || (!strings.Contains(desc, ";3f0000;") && !strings.Contains(desc, ";003f0000;")) {
				t.Fatal("unsafe key permissions", err)
			}
		}
		if mode == "expiry" {
			time.Sleep(1200 * time.Millisecond)
			fresh := NewResolver(map[string]config.CredentialSpec{"test-only": spec})
			defer fresh.Close()
			fresh.Configure(r.namespace, false, nil)
			if _, err := fresh.ResolveWithPrompt("test-only", false); !errors.Is(err, errPromptDisabled) {
				t.Fatal("expired cache remained usable", err)
			}
		}
	case "read":
		value, err := r.ResolveWithPrompt("test-only", false)
		if err != nil || value != os.Getenv("RETRI_TEST_VALUE") || calls != 0 {
			t.Fatal("cross-session cache reuse failed", err)
		}
	default:
		t.Fatal("unknown helper mode")
	}
}
