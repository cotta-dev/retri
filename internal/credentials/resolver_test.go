package credentials

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cotta-dev/retri/internal/config"
)

func TestResolvePromptOncePerNamedCredential(t *testing.T) {
	calls := 0
	r := NewResolver(map[string]config.CredentialSpec{
		"shared": {Provider: "prompt", Prompt: "Shared password"},
	})
	r.prompt = func(label string) (string, error) {
		calls++
		if label != "Shared password" {
			t.Fatalf("label = %q", label)
		}
		return "secret-value", nil
	}

	for i := 0; i < 3; i++ {
		got, err := r.Resolve("shared")
		if err != nil {
			t.Fatal(err)
		}
		if got != "secret-value" {
			t.Fatalf("Resolve() = %q", got)
		}
	}
	if calls != 1 {
		t.Fatalf("prompt calls = %d, want 1", calls)
	}
}

func TestPromptPolicyAcceptsCacheAndRejectsMissWithoutInput(t *testing.T) {
	spec := config.CredentialSpec{Provider: "prompt", Cache: config.CredentialCacheConfig{Backend: "session-keyring"}}
	keys := &memoryKeys{data: make(map[string][]byte)}
	r := NewResolver(map[string]config.CredentialSpec{"shared": spec})
	r.cache = &keyringCache{keys: keys, now: time.Now}
	r.namespace = "config"
	calls := 0
	r.prompt = func(string) (string, error) { calls++; return "test-value", nil }
	if _, err := r.ResolveWithPrompt("shared", false); err == nil || calls != 0 {
		t.Fatal("disabled prompt must fail without reading input")
	}
	if _, err := r.Resolve("shared"); err != nil || calls != 1 {
		t.Fatal("policy failure must not poison an allowed prompt", err)
	}
	r.refresh = true
	if got, err := r.ResolveWithPrompt("shared", false); err != nil || got != "test-value" || calls != 1 {
		t.Fatal("prompt policies must share the acquired snapshot, including refresh", err)
	}
	r.Close()
	r = NewResolver(map[string]config.CredentialSpec{"shared": spec})
	defer r.Close()
	r.cache = &keyringCache{keys: keys, now: time.Now}
	r.namespace = "config"
	r.prompt = func(string) (string, error) { t.Fatal("unexpected prompt"); return "", nil }
	if got, err := r.ResolveWithPrompt("shared", false); err != nil || got != "test-value" {
		t.Fatal("disabled prompt must accept valid cache", err)
	}
}

func TestResolveEnv(t *testing.T) {
	t.Setenv("RETRI_TEST_CREDENTIAL", "from-env")
	r := NewResolver(map[string]config.CredentialSpec{
		"env-secret": {Provider: "env", Ref: "RETRI_TEST_CREDENTIAL"},
	})
	got, err := r.Resolve("env-secret")
	if err != nil {
		t.Fatal(err)
	}
	if got != "from-env" {
		t.Fatalf("Resolve() = %q", got)
	}
}

func TestResolveMissingEnvFails(t *testing.T) {
	r := NewResolver(map[string]config.CredentialSpec{
		"env-secret": {Provider: "env", Ref: "RETRI_TEST_DOES_NOT_EXIST"},
	})
	if _, err := r.Resolve("env-secret"); err == nil {
		t.Fatal("expected missing environment variable error")
	}
}

func TestResolveLiteralExpandsEnvironment(t *testing.T) {
	t.Setenv("RETRI_TEST_PART", "expanded")
	r := NewResolver(map[string]config.CredentialSpec{
		"literal": {Provider: "literal", Value: "prefix-${RETRI_TEST_PART}"},
	})
	got, err := r.Resolve("literal")
	if err != nil {
		t.Fatal(err)
	}
	if got != "prefix-expanded" {
		t.Fatalf("Resolve() = %q", got)
	}
}

func TestResolveBitwardenPasswordAndCustomField(t *testing.T) {
	var calls [][]string
	r := NewResolver(map[string]config.CredentialSpec{
		"password": {Provider: "bitwarden", Ref: "item-id"},
		"enable":   {Provider: "bitwarden", Ref: "item-id", Field: "enable"},
	})
	r.run = func(name string, args []string, stdin []byte) ([]byte, error) {
		if name != "bw" {
			return nil, errors.New("unexpected command")
		}
		calls = append(calls, append([]string(nil), args...))
		return []byte(`{"login":{"username":"admin","password":"login-secret"},"fields":[{"name":"enable","value":"enable-secret"}]}`), nil
	}

	password, err := r.Resolve("password")
	if err != nil {
		t.Fatal(err)
	}
	enable, err := r.Resolve("enable")
	if err != nil {
		t.Fatal(err)
	}
	if password != "login-secret" || enable != "enable-secret" {
		t.Fatalf("password=%q enable=%q", password, enable)
	}
	want := [][]string{{"get", "item", "item-id", "--nointeraction"}, {"get", "item", "item-id", "--nointeraction"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

func TestCacheDescriptionDoesNotExposeProviderMetadata(t *testing.T) {
	description := cacheDescription("/config", "network-login", config.CredentialSpec{Provider: "bitwarden", Ref: "item-id"})
	for _, sensitiveMetadata := range []string{"bitwarden", "item-id", "password", "BW_SESSION"} {
		if strings.Contains(description, sensitiveMetadata) {
			t.Fatalf("cache description %q exposed %q", description, sensitiveMetadata)
		}
	}
	if !strings.HasPrefix(description, "retri:credential:") {
		t.Fatalf("cache description = %q", description)
	}
}

func TestConcurrentResolutionSharesSuccessAndFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		r := NewResolver(map[string]config.CredentialSpec{"shared": {Provider: "prompt"}})
		var calls atomic.Int32
		r.prompt = func(string) (string, error) {
			calls.Add(1)
			if fail {
				return "", errors.New("failed")
			}
			return "value", nil
		}
		var wg sync.WaitGroup
		for range 30 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				v, err := r.Resolve("shared")
				if (err != nil) != fail || (!fail && v != "value") {
					t.Error("unexpected resolution")
				}
			}()
		}
		wg.Wait()
		if calls.Load() != 1 {
			t.Fatalf("calls = %d", calls.Load())
		}
	}
}

func TestProviderFailureNeverFallsBack(t *testing.T) {
	t.Setenv("RETRI_SSH_PASSWORD", "fallback")
	r := NewResolver(map[string]config.CredentialSpec{"login": {Provider: "bitwarden", Ref: "item"}})
	r.run = func(string, []string, []byte) ([]byte, error) { return nil, errors.New("offline") }
	r.prompt = func(string) (string, error) { t.Fatal("unexpected prompt"); return "", nil }
	if _, err := r.Resolve("login"); err == nil {
		t.Fatal("expected provider failure")
	}
}

func TestRejectEmptyAndTerminalControlValues(t *testing.T) {
	for _, value := range []string{"", "a\nb", "a\rb", "a\x00b"} {
		r := NewResolver(map[string]config.CredentialSpec{"login": {Provider: "env", Ref: "RETRI_TEST_EMPTY"}})
		r.specs["login"] = config.CredentialSpec{Provider: "prompt"}
		r.prompt = func(string) (string, error) { return value, nil }
		if _, err := r.Resolve("login"); err == nil {
			t.Fatal("expected invalid value error")
		}
	}
}

func TestBitwardenProfileMismatchAndMalformedResponseAreSafe(t *testing.T) {
	for _, response := range []string{`{"serverUrl":"https://other","userId":"account","status":"unlocked"}`, `{"secret":"sensitive-value"`} {
		r := NewResolver(nil)
		r.run = func(_ string, args []string, _ []byte) ([]byte, error) {
			if args[0] != "status" {
				t.Fatal("must reject before reading item")
			}
			return []byte(response), nil
		}
		_, err := r.readBitwarden(config.CredentialSpec{Server: "https://vault.example", Account: "account"})
		if err == nil || strings.Contains(err.Error(), "sensitive-value") {
			t.Fatal("unsafe error")
		}
	}
}
