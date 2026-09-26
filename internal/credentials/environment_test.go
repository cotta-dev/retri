package credentials

import (
	"github.com/cotta-dev/retri/internal/config"
	"reflect"
	"testing"
)

func TestChildEnvironmentTracksAllCredentialInputs(t *testing.T) {
	cfg := config.Config{Defaults: config.GlobalOptions{CommonFields: config.CommonFields{Password: "${LEGACY}"}}, Credentials: map[string]config.CredentialSpec{"login": {Provider: "env", Ref: "CUSTOM_PASSWORD"}, "sudo": {Provider: "literal", Value: "prefix-$PART"}}}
	env := []string{"PATH=/bin", "SSH_AUTH_SOCK=/agent", "CUSTOM_PASSWORD=x", "LEGACY=y", "PART=z", "RETRI_SSH_PASSWORD=a", "BW_SESSION=b", "BW_PASSWORD=c"}
	if got := ChildEnvironment(cfg, env, false); !reflect.DeepEqual(got, env[:2]) {
		t.Fatalf("unsafe child env: %v", got)
	}
	want := append(append([]string{}, env[:2]...), "BW_SESSION=b", "BW_PASSWORD=c")
	if got := ChildEnvironment(cfg, env, true); !reflect.DeepEqual(got, want) {
		t.Fatal("provider env lost required values")
	}
}
