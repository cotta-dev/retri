//go:build linux

package credentials

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestCachePublishesOnlyAfterPermissionAndTimeout(t *testing.T) {
	for _, failed := range []string{"", "create", "perm", "timeout", "update"} {
		var steps []string
		step := func(name string) error {
			steps = append(steps, name)
			if name == failed {
				return errors.New("failure")
			}
			return nil
		}
		err := writeKeyPayload([]byte("secret"), time.Minute,
			func() (int, error) { return 1, step("create") },
			func(int) error { return step("perm") },
			func(int, int) error { return step("timeout") },
			func(int, []byte) error { return step("update") },
			func(int) error { return step("revoke") },
		)
		if (err != nil) != (failed != "") {
			t.Fatal("wrong error")
		}
		want := []string{"create", "perm", "timeout", "update"}
		if failed != "" {
			for i, s := range want {
				if s == failed {
					want = want[:i+1]
					break
				}
			}
			if failed != "create" {
				want = append(want, "revoke")
			}
		}
		if !reflect.DeepEqual(steps, want) {
			t.Fatalf("%s: %v", failed, steps)
		}
	}
}

func TestNativeSessionCacheRoundTrip(t *testing.T) {
	if os.Getenv("RETRI_NATIVE_KEYRING_TEST") != "1" {
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(exe, "-test.run=^TestNativeSessionCacheRoundTrip$", "-test.v")
		cmd.Env = append(os.Environ(), "RETRI_NATIVE_KEYRING_TEST=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("native keyring subprocess: %v\n%s", err, out)
		}
		t.Log(string(out))
		return
	}
	// Isolate the test from the caller's login/session ring. Keyring membership
	// is thread-local, so keep this helper on its own OS thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if _, err := unix.KeyctlInt(unix.KEYCTL_JOIN_SESSION_KEYRING, 0, 0, 0, 0); err != nil {
		if errors.Is(err, unix.EPERM) || errors.Is(err, unix.EACCES) || errors.Is(err, unix.ENOSYS) {
			t.Skipf("kernel keyring unavailable: %v", err)
		}
		t.Fatal(err)
	}
	description := fmt.Sprintf("retri-test:%d", time.Now().UnixNano())
	keys := linuxKeys{}
	if err := keys.Write(description, []byte("test-only-value"), time.Second); err != nil {
		if errors.Is(err, unix.EPERM) || errors.Is(err, unix.EACCES) || errors.Is(err, unix.ENOSYS) {
			t.Skipf("kernel keyring unavailable: %v", err)
		}
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := keys.Delete(description); err != nil {
			t.Error(err)
		}
	})
	value, found, err := keys.Read(description)
	if err != nil || !found || string(value) != "test-only-value" {
		t.Fatalf("roundtrip: found=%v err=%v", found, err)
	}
	ring, err := unix.KeyctlSearch(unix.KEY_SPEC_SESSION_KEYRING, "keyring", description, 0)
	if err != nil {
		t.Fatal(err)
	}
	desc, err := unix.KeyctlString(unix.KEYCTL_DESCRIBE, ring)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(desc, ";3f0000;") && !strings.Contains(desc, ";003f0000;") {
		t.Fatalf("unsafe permissions: %s", desc)
	}
	// Replacing a snapshot must allocate a new ring, not extend the old ring.
	if err := keys.Write(description, []byte("replacement"), time.Second); err != nil {
		t.Fatal(err)
	}
	replacement, err := unix.KeyctlSearch(unix.KEY_SPEC_SESSION_KEYRING, "keyring", description, 0)
	if err != nil {
		t.Fatal(err)
	}
	if replacement == ring {
		t.Fatal("cache publication reused old ring")
	}
	time.Sleep(1200 * time.Millisecond)
	if _, found, err := keys.Read(description); err != nil || found {
		t.Fatalf("expired key accepted: %v", err)
	}
}
