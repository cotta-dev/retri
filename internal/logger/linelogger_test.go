package logger

import (
	"bytes"
	"strings"
	"testing"
)

func TestLineLogger_Write(t *testing.T) {
	var buf bytes.Buffer
	ll := NewLineLogger(&buf, false) // timestamps disabled for easier testing

	_, _ = ll.Write([]byte("line one\nline two\n"))

	output := buf.String()
	if !strings.Contains(output, "line one") {
		t.Errorf("expected output to contain 'line one', got %q", output)
	}
	if !strings.Contains(output, "line two") {
		t.Errorf("expected output to contain 'line two', got %q", output)
	}
}

func TestLineLogger_WriteSplitLines(t *testing.T) {
	var buf bytes.Buffer
	ll := NewLineLogger(&buf, false)

	// Write data in chunks that split across lines
	_, _ = ll.Write([]byte("hel"))
	_, _ = ll.Write([]byte("lo\nwor"))
	_, _ = ll.Write([]byte("ld\n"))

	output := buf.String()
	if !strings.Contains(output, "hello") {
		t.Errorf("expected output to contain 'hello', got %q", output)
	}
	if !strings.Contains(output, "world") {
		t.Errorf("expected output to contain 'world', got %q", output)
	}
}

func TestLineLogger_DeduplicateEmptyLines(t *testing.T) {
	var buf bytes.Buffer
	ll := NewLineLogger(&buf, false)

	_, _ = ll.Write([]byte("text\n\n\n\nmore text\n"))

	output := buf.String()
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	// Count empty lines
	emptyCount := 0
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			emptyCount++
		}
	}
	if emptyCount > 1 {
		t.Errorf("expected at most 1 consecutive empty line, got %d in %q", emptyCount, output)
	}
}

func TestLineLogger_StripAnsi(t *testing.T) {
	var buf bytes.Buffer
	ll := NewLineLogger(&buf, false)

	_, _ = ll.Write([]byte("\x1b[31mred text\x1b[0m\n"))

	output := buf.String()
	if strings.Contains(output, "\x1b") {
		t.Errorf("expected ANSI codes to be stripped, got %q", output)
	}
	if !strings.Contains(output, "red text") {
		t.Errorf("expected output to contain 'red text', got %q", output)
	}
}

func TestLineLogger_StripCarriageReturn(t *testing.T) {
	var buf bytes.Buffer
	ll := NewLineLogger(&buf, false)

	_, _ = ll.Write([]byte("hello\r\n"))

	output := buf.String()
	if strings.Contains(output, "\r") {
		t.Errorf("expected carriage returns to be stripped, got %q", output)
	}
}

func TestLineLogger_WithTimestamp(t *testing.T) {
	var buf bytes.Buffer
	ll := NewLineLogger(&buf, true)

	_, _ = ll.Write([]byte("test line\n"))

	output := buf.String()
	// Timestamp format: [YYYY-MM-DD HH:MM:SS.mmm]
	if !strings.HasPrefix(output, "[") {
		t.Errorf("expected timestamp prefix, got %q", output)
	}
	if !strings.Contains(output, "test line") {
		t.Errorf("expected output to contain 'test line', got %q", output)
	}
}

func TestLineLogger_Flush(t *testing.T) {
	var buf bytes.Buffer
	ll := NewLineLogger(&buf, false)

	// Write incomplete line (no trailing newline)
	_, _ = ll.Write([]byte("incomplete"))

	// Buffer should not have written yet
	if buf.String() != "" {
		t.Errorf("expected empty output before flush, got %q", buf.String())
	}

	ll.Flush()

	output := buf.String()
	if !strings.Contains(output, "incomplete") {
		t.Errorf("expected flushed output to contain 'incomplete', got %q", output)
	}
}

func TestLineLogger_FlushEmpty(t *testing.T) {
	var buf bytes.Buffer
	ll := NewLineLogger(&buf, false)

	// Flush on empty buffer should be a no-op
	ll.Flush()

	if buf.String() != "" {
		t.Errorf("expected empty output after flushing empty buffer, got %q", buf.String())
	}
}

func TestLineLogger_LogHeader(t *testing.T) {
	var buf bytes.Buffer
	ll := NewLineLogger(&buf, false)

	_, _ = ll.Write([]byte("some text\n"))
	ll.LogHeader("show version")

	output := buf.String()
	if !strings.Contains(output, "[EXEC] show version") {
		t.Errorf("expected header with command, got %q", output)
	}
	if !strings.Contains(output, "----------------------------------------") {
		t.Errorf("expected separator line, got %q", output)
	}
}
