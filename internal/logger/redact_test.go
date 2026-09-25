package logger

import (
	"bytes"
	"strings"
	"testing"
)

func TestSecretWriterHandlesEverySplitAndOverlappingSecrets(t *testing.T) {
	input := "start abcdef / secret end"
	for split := 0; split <= len(input); split++ {
		var out bytes.Buffer
		w := NewSecretWriter(&out, "abc", "abcdef", "secret")
		if _, err := w.Write([]byte(input[:split])); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(input[split:])); err != nil {
			t.Fatal(err)
		}
		if err := w.Flush(); err != nil {
			t.Fatal(err)
		}
		if out.String() != "start [REDACTED] / [REDACTED] end" {
			t.Fatalf("split %d: %q", split, out.String())
		}
	}
}

func TestRenderedSecretIsRedacted(t *testing.T) {
	var out bytes.Buffer
	l := NewLineLogger(&out, false)
	l.RedactSecrets("secret")
	_, err := l.Write([]byte("sec\x1b[31mret\n"))
	if err != nil {
		t.Fatal(err)
	}
	l.Flush()
	if strings.Contains(out.String(), "secret") || !strings.Contains(out.String(), "[REDACTED]") {
		t.Fatal("rendered secret leaked")
	}
}
