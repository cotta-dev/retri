package executor

import (
	"reflect"
	"testing"
)

func TestSanitizedEnvironment(t *testing.T) {
	in := []string{
		"PATH=/usr/bin",
		"RETRI_SSH_PASSWORD=secret",
		"RETRI_SSH_SECRET=sudo",
		"BW_SESSION=session",
		"SSH_AUTH_SOCK=/tmp/agent.sock",
	}
	want := []string{"PATH=/usr/bin", "SSH_AUTH_SOCK=/tmp/agent.sock"}
	if got := sanitizedEnvironment(in); !reflect.DeepEqual(got, want) {
		t.Fatalf("sanitizedEnvironment() = %#v, want %#v", got, want)
	}
}
