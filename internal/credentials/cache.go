package credentials

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cotta-dev/retri/internal/config"
)

const defaultSessionCacheTTL = 8 * time.Hour
const maxCachePayload = 32767
const pendingCachePayload = "retri-cache-pending-v2"

type sessionCache interface {
	Get(string, string, config.CredentialSpec) (string, bool, error)
	Put(string, string, config.CredentialSpec, string) error
	Delete(string, string, config.CredentialSpec) error
}

type keyStore interface {
	Read(string) ([]byte, bool, error)
	Write(string, []byte, time.Duration) error
	Delete(string) error
}

type keyringCache struct {
	keys keyStore
	now  func() time.Time
}

func newSessionCache() sessionCache { return &keyringCache{keys: newKeyStore(), now: time.Now} }

type cacheEntry struct {
	Version     int
	Fingerprint string
	Created     time.Time
	Expires     time.Time
	Value       string
}

func cacheTTL(spec config.CredentialSpec) (time.Duration, error) {
	if spec.Cache.TTL == "" {
		return defaultSessionCacheTTL, nil
	}
	d, err := time.ParseDuration(spec.Cache.TTL)
	if err != nil || d < time.Second || d > 24*time.Hour {
		return 0, fmt.Errorf("cache ttl must be between 1s and 24h")
	}
	return d, nil
}

func normalizedSpec(spec config.CredentialSpec) config.CredentialSpec {
	spec.Provider = strings.ToLower(strings.TrimSpace(spec.Provider))
	spec.Field = strings.TrimSpace(spec.Field)
	if spec.Provider == "bitwarden" && spec.Field == "" {
		spec.Field = "password"
	}
	spec.Server = strings.TrimRight(spec.Server, "/")
	spec.Cache = config.CredentialCacheConfig{}
	return spec
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func cacheDescription(namespace, name string, spec config.CredentialSpec) string {
	spec = normalizedSpec(spec)
	// Literal values never appear in public metadata, even as a guessable hash.
	spec.Value = ""
	b, _ := json.Marshal(struct {
		Namespace, Name string
		Source          config.CredentialSpec
	}{namespace, name, spec})
	return "retri:credential:v2:" + digest(b)
}

func sourceFingerprint(spec config.CredentialSpec) string {
	b, _ := json.Marshal(normalizedSpec(spec))
	defer clear(b)
	return digest(b)
}

func (c *keyringCache) Get(namespace, name string, spec config.CredentialSpec) (string, bool, error) {
	ttl, err := cacheTTL(spec)
	if err != nil {
		return "", false, err
	}
	payload, found, err := c.keys.Read(cacheDescription(namespace, name, spec))
	defer clear(payload)
	if err != nil || !found {
		return "", false, err
	}
	// A placeholder means a writer is in progress or stopped before publication.
	if len(payload) == 0 || string(payload) == pendingCachePayload {
		return "", false, nil
	}
	var entry cacheEntry
	if json.Unmarshal(payload, &entry) != nil || entry.Version != 2 {
		return "", false, fmt.Errorf("invalid credential cache entry; clear the cache")
	}
	now := c.now()
	if entry.Fingerprint != sourceFingerprint(spec) || entry.Created.After(now) || !entry.Expires.After(entry.Created) || !now.Before(entry.Expires) || !now.Before(entry.Created.Add(ttl)) {
		return "", false, nil
	}
	if err := validateValue(entry.Value); err != nil {
		return "", false, fmt.Errorf("invalid credential cache value; clear the cache")
	}
	return entry.Value, true, nil
}

func (c *keyringCache) Put(namespace, name string, spec config.CredentialSpec, value string) error {
	ttl, err := cacheTTL(spec)
	if err != nil {
		return err
	}
	if err := validateValue(value); err != nil {
		return err
	}
	now := c.now()
	payload, err := json.Marshal(cacheEntry{2, sourceFingerprint(spec), now, now.Add(ttl), value})
	defer clear(payload)
	if err != nil {
		return fmt.Errorf("encode credential cache")
	}
	if len(payload) > maxCachePayload {
		return fmt.Errorf("credential exceeds cache size limit")
	}
	return c.keys.Write(cacheDescription(namespace, name, spec), payload, ttl)
}

func (c *keyringCache) Delete(namespace, name string, spec config.CredentialSpec) error {
	return c.keys.Delete(cacheDescription(namespace, name, spec))
}
