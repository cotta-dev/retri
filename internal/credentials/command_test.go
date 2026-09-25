package credentials

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestProviderCommandDoesNotExposeStderr(t *testing.T) {
	_, err := commandRunner([]string{})("sh", []string{"-c", "printf 'private-credential' >&2; exit 1"}, nil)
	if err == nil || strings.Contains(err.Error(), "private-credential") {
		t.Fatal("provider diagnostics leaked")
	}
}

func TestProviderCommandTimeout(t *testing.T) {
	start := time.Now()
	_, err := commandRunnerWithTimeout([]string{}, 20*time.Millisecond)("sh", []string{"-c", "exec sleep 10"}, nil)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatal("missing timeout")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("provider was not cancelled")
	}
}

func TestProviderOutputIsBounded(t *testing.T) {
	var b boundedOutput
	if _, err := b.Write(bytes.Repeat([]byte("x"), maxProviderOutput)); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Write([]byte("x")); err == nil {
		t.Fatal("output was not bounded")
	}
}
