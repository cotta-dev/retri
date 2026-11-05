//go:build linux

package credentials

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/cotta-dev/retri/internal/config"
	"golang.org/x/sys/unix"
)

// No subprocesses inherit secrets. Owner-only permissions intentionally do not
// grant access merely because another user possesses the same keyring.
type linuxKeys struct{ ring int }

func newKeyStore(backend string) keyStore {
	if strings.EqualFold(strings.TrimSpace(backend), config.CacheBackendUserKeyring) {
		return linuxKeys{ring: unix.KEY_SPEC_USER_KEYRING}
	}
	return linuxKeys{}
}

func (k linuxKeys) keyringID() int {
	if k.ring != 0 {
		return k.ring
	}
	return unix.KEY_SPEC_SESSION_KEYRING
}

func missingKey(err error) bool {
	return errors.Is(err, unix.ENOKEY) || errors.Is(err, unix.EKEYEXPIRED) || errors.Is(err, unix.EKEYREVOKED)
}

// A missing/expired cache entry is a miss, but an unusable parent ring is not.
// Never replace the inherited ring here: a process-local replacement would not
// be shared with later Retri invocations from the parent shell.
func (k linuxKeys) checkKeyring() error {
	_, err := unix.KeyctlGetKeyringID(k.keyringID(), false)
	if err == nil || errors.Is(err, unix.ENOKEY) {
		return nil
	}
	if errors.Is(err, unix.EKEYREVOKED) || errors.Is(err, unix.EKEYEXPIRED) {
		if k.keyringID() == unix.KEY_SPEC_USER_KEYRING {
			return fmt.Errorf("user keyring is revoked or expired; check the Linux user keyring configuration: %w", err)
		}
		return fmt.Errorf("session keyring is revoked or expired; start a fresh login session or run \"keyctl session - bash\" and retry in that shell: %w", err)
	}
	return fmt.Errorf("check Linux keyring: %w", err)
}

func (k linuxKeys) Read(description string) ([]byte, bool, error) {
	ring, err := unix.KeyctlSearch(k.keyringID(), "keyring", description, 0)
	if missingKey(err) {
		return nil, false, k.checkKeyring()
	}
	if err != nil {
		return nil, false, fmt.Errorf("search credential keyring: %w", err)
	}
	id, err := unix.KeyctlSearch(ring, "user", "value", 0)
	if missingKey(err) {
		return nil, false, k.checkKeyring()
	}
	if err != nil {
		return nil, false, fmt.Errorf("search cache value: %w", err)
	}
	return readKeyPayload(id)
}

func readKeyPayload(id int) ([]byte, bool, error) {
	n, err := unix.KeyctlBuffer(unix.KEYCTL_READ, id, nil, 0)
	if missingKey(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read keyring: %w", err)
	}
	if n > maxCachePayload {
		return nil, false, fmt.Errorf("keyring payload exceeds limit")
	}
	b := make([]byte, n)
	n, err = unix.KeyctlBuffer(unix.KEYCTL_READ, id, b, 0)
	if err != nil || n > len(b) {
		clear(b)
		if missingKey(err) {
			return nil, false, nil
		}
		if err == nil {
			return nil, false, fmt.Errorf("keyring changed during read; retry")
		}
		return nil, false, fmt.Errorf("read keyring: %w", err)
	}
	return b[:n], true, nil
}

// writeKeyPayload is separate so failure ordering can be tested without a
// kernel keyring. The containing ring expires even if a crash prevents setting
// the value key's timeout after publication (KEYCTL_UPDATE resets that timeout).
func writeKeyPayload(payload []byte, ttl time.Duration, create func() (int, error), perm func(int) error, timeout func(int, int) error, update func(int, []byte) error, revoke func(int) error) error {
	id, err := create()
	if err != nil {
		return fmt.Errorf("create cache key: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = revoke(id)
		}
	}()
	if err := perm(id); err != nil {
		return fmt.Errorf("set cache permissions: %w", err)
	}
	seconds := int((ttl + time.Second - 1) / time.Second)
	if err := timeout(id, seconds); err != nil {
		return fmt.Errorf("set cache timeout: %w", err)
	}
	if err := update(id, payload); err != nil {
		return fmt.Errorf("publish cache key: %w", err)
	}
	committed = true
	return nil
}

func (k linuxKeys) Write(description string, payload []byte, ttl time.Duration) error {
	destination := k.keyringID()
	staging := destination
	if destination == unix.KEY_SPEC_USER_KEYRING {
		// Without a valid session ring the UID ring may not be possessed, and
		// newly created keys do not yet grant their owner setattr permission.
		// Prepare in this thread's ring before publishing to the shared store.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		var err error
		staging, err = unix.KeyctlGetKeyringID(unix.KEY_SPEC_THREAD_KEYRING, true)
		if err != nil {
			return fmt.Errorf("create temporary thread keyring: %w", err)
		}
	}
	var stagedRing int
	if staging != destination {
		defer func() {
			if stagedRing > 0 {
				_, _ = unix.KeyctlInt(unix.KEYCTL_UNLINK, stagedRing, staging, 0, 0)
			}
		}()
	}
	// keyrings have no update method: AddKey creates a new ring and replaces
	// the old link. Concurrent writers therefore never modify each other's key.
	err := writeKeyPayload(payload, ttl,
		func() (int, error) {
			id, err := unix.AddKey("keyring", description, nil, staging)
			if missingKey(err) {
				if keyringErr := k.checkKeyring(); keyringErr != nil {
					return 0, keyringErr
				}
			}
			stagedRing = id
			return id, err
		},
		func(id int) error { return unix.KeyctlSetperm(id, 0x003f0000) },
		func(id, seconds int) error {
			_, err := unix.KeyctlInt(unix.KEYCTL_SET_TIMEOUT, id, seconds, 0, 0)
			return err
		},
		func(ring int, b []byte) error {
			id, err := unix.AddKey("user", "value", []byte(pendingCachePayload), ring)
			if err != nil {
				return err
			}
			committed := false
			defer func() {
				if !committed {
					_ = revokeKey(id)
				}
			}()
			if err = unix.KeyctlSetperm(id, 0x003f0000); err != nil {
				return err
			}
			if _, err = unix.KeyctlBuffer(unix.KEYCTL_UPDATE, id, b, 0); err != nil {
				return err
			}
			if _, err = unix.KeyctlInt(unix.KEYCTL_SET_TIMEOUT, id, int((ttl+time.Second-1)/time.Second), 0, 0); err != nil {
				return err
			}
			committed = true
			return nil
		},
		revokeKey,
	)
	if err != nil {
		return err
	}
	if staging != destination {
		if _, err := unix.KeyctlInt(unix.KEYCTL_LINK, stagedRing, destination, 0, 0); err != nil {
			_ = revokeKey(stagedRing)
			return fmt.Errorf("publish user cache keyring: %w", err)
		}
	}
	return nil
}

func revokeKey(id int) error { _, err := unix.KeyctlInt(unix.KEYCTL_REVOKE, id, 0, 0, 0); return err }

func (k linuxKeys) Delete(description string) error {
	id, err := unix.KeyctlSearch(k.keyringID(), "keyring", description, 0)
	if missingKey(err) {
		return k.checkKeyring()
	}
	if err != nil {
		return fmt.Errorf("search credential keyring: %w", err)
	}
	value, valueErr := unix.KeyctlSearch(id, "user", "value", 0)
	if valueErr == nil {
		if err := revokeKey(value); err != nil && !missingKey(err) {
			return err
		}
	} else if !missingKey(valueErr) {
		return valueErr
	}
	err = revokeKey(id)
	if missingKey(err) {
		return nil
	}
	return err
}

func readSourceKey(description string) (string, bool, error) {
	id, err := unix.KeyctlSearch(unix.KEY_SPEC_SESSION_KEYRING, "user", description, 0)
	if missingKey(err) {
		return "", false, (linuxKeys{}).checkKeyring()
	}
	if err != nil {
		return "", false, fmt.Errorf("search source key: %w", err)
	}
	b, found, err := readKeyPayload(id)
	defer clear(b)
	return string(b), found, err
}
