package credentials

import (
	"errors"
	"github.com/cotta-dev/retri/internal/config"
	"testing"
	"time"
)

type memoryKeys struct {
	data map[string][]byte
	err  error
}

func (m *memoryKeys) Read(key string) ([]byte, bool, error) {
	b, ok := m.data[key]
	return append([]byte(nil), b...), ok, m.err
}
func (m *memoryKeys) Write(key string, b []byte, _ time.Duration) error {
	if m.err != nil {
		return m.err
	}
	m.data[key] = append([]byte(nil), b...)
	return nil
}
func (m *memoryKeys) Delete(key string) error { delete(m.data, key); return m.err }

func TestCacheIdentityExpiryAndDefinitionChanges(t *testing.T) {
	now := time.Unix(1000, 0)
	keys := &memoryKeys{data: make(map[string][]byte)}
	c := &keyringCache{keys: keys, now: func() time.Time { return now }}
	spec := config.CredentialSpec{Provider: "literal", Value: "first", Cache: config.CredentialCacheConfig{Backend: "session-keyring", TTL: "1h"}}
	if err := c.Put("prod", "login", spec, "first"); err != nil {
		t.Fatal(err)
	}
	if v, ok, err := c.Get("prod", "login", spec); err != nil || !ok || v != "first" {
		t.Fatal("cache miss")
	}
	if _, ok, _ := c.Get("dev", "login", spec); ok {
		t.Fatal("cross-config leak")
	}
	other := spec
	other.Value = "second"
	if _, ok, _ := c.Get("prod", "login", other); ok {
		t.Fatal("stale literal")
	}
	other = spec
	other.Provider = "env"
	other.Ref = "OTHER"
	if _, ok, _ := c.Get("prod", "login", other); ok {
		t.Fatal("stale provider")
	}
	now = now.Add(10 * time.Minute)
	other = spec
	other.Cache.TTL = "5m"
	if _, ok, _ := c.Get("prod", "login", other); ok {
		t.Fatal("shortened TTL ignored")
	}
	now = now.Add(time.Hour)
	if _, ok, _ := c.Get("prod", "login", spec); ok {
		t.Fatal("expired value accepted")
	}
}

func TestOfflineCacheRefreshAndClear(t *testing.T) {
	keys := &memoryKeys{data: make(map[string][]byte)}
	c := &keyringCache{keys: keys, now: time.Now}
	spec := config.CredentialSpec{Provider: "bitwarden", Ref: "item", Server: "https://vault.example", Account: "account", Cache: config.CredentialCacheConfig{Backend: "session-keyring"}}
	if err := c.Put("config", "login", spec, "cached"); err != nil {
		t.Fatal(err)
	}
	newResolver := func() *Resolver {
		r := NewResolver(map[string]config.CredentialSpec{"login": spec})
		r.cache = c
		r.namespace = "config"
		r.run = func(string, []string, []byte) ([]byte, error) { return nil, errors.New("offline") }
		return r
	}
	if v, err := newResolver().Resolve("login"); err != nil || v != "cached" {
		t.Fatal("offline cache failed")
	}
	r := newResolver()
	r.refresh = true
	if _, err := r.Resolve("login"); err == nil {
		t.Fatal("refresh silently reused old value")
	}
	if _, ok, _ := c.Get("config", "login", spec); ok {
		t.Fatal("refresh did not clear")
	}
	if err := c.Put("config", "login", spec, "cached"); err != nil {
		t.Fatal(err)
	}
	if err := newResolver().Clear(); err != nil {
		t.Fatal(err)
	}
	if len(keys.data) != 0 {
		t.Fatal("clear failed")
	}
}

func TestCacheErrorsAreNotMisses(t *testing.T) {
	c := &keyringCache{keys: &memoryKeys{err: errors.New("permission denied")}, now: time.Now}
	if _, _, err := c.Get("config", "login", config.CredentialSpec{}); err == nil {
		t.Fatal("cache error swallowed")
	}
}

func TestCacheMetadataDoesNotHashLiteralSecret(t *testing.T) {
	a := config.CredentialSpec{Provider: "literal", Value: "guessable"}
	b := a
	b.Value = "different"
	if cacheDescription("config", "login", a) != cacheDescription("config", "login", b) {
		t.Fatal("public metadata depends on literal secret")
	}
}
