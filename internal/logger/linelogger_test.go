package logger

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

var errLogWrite = errors.New("log write failed")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errLogWrite }

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

func TestLineLogger_ReportsStickyWriteError(t *testing.T) {
	ll := NewLineLogger(failingWriter{}, false)
	input := []byte("line\n")

	if n, err := ll.Write(input); n != len(input) || !errors.Is(err, errLogWrite) {
		t.Fatalf("Write() = (%d, %v), want (%d, %v)", n, err, len(input), errLogWrite)
	}
	if !errors.Is(ll.Err(), errLogWrite) {
		t.Fatalf("Err() = %v, want %v", ll.Err(), errLogWrite)
	}
	if n, err := ll.Write([]byte("later\n")); n != 0 || !errors.Is(err, errLogWrite) {
		t.Fatalf("second Write() = (%d, %v), want (0, %v)", n, err, errLogWrite)
	}
}

func TestLineLogger_RawWriteErrorIsObservable(t *testing.T) {
	ll := NewLineLogger(failingWriter{}, false)
	ll.WriteRaw("header\n")
	if !errors.Is(ll.Err(), errLogWrite) {
		t.Fatalf("Err() = %v, want %v", ll.Err(), errLogWrite)
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

func TestLineLogger_CurrentLine(t *testing.T) {
	var buf bytes.Buffer
	ll := NewLineLogger(&buf, false)

	_, _ = ll.Write([]byte("old prompt\r\x1b[2Khost$ command"))
	if got, want := ll.CurrentLine(), "host$ command"; got != want {
		t.Fatalf("CurrentLine() = %q, want %q", got, want)
	}
	if buf.Len() != 0 {
		t.Fatalf("CurrentLine flushed output: %q", buf.String())
	}
}

func TestLineLogger_LogHeader(t *testing.T) {
	var buf bytes.Buffer
	ll := NewLineLogger(&buf, false)

	_, _ = ll.Write([]byte("some text\n"))
	ll.LogHeader("show version", false)

	output := buf.String()
	if !strings.Contains(output, "[EXEC] show version") {
		t.Errorf("expected header with command, got %q", output)
	}
	if !strings.Contains(output, "----------------------------------------") {
		t.Errorf("expected separator line, got %q", output)
	}
}

func TestLineLogger_RendersTerminalEditing(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "carriage return overwrites existing prompt",
			input: "user@host:~$ stale\r\x1b[2Kuser@host:~$ uptime\r\n",
			want:  "user@host:~$ uptime\n",
		},
		{
			name:  "backspace edits instead of duplicating text",
			input: "switch#show versioX\bn\r\n",
			want:  "switch#show version\n",
		},
		{
			name:  "erase from cursor removes stale suffix",
			input: "router#old prompt\rrouter#\x1b[Kshow clock\r\n",
			want:  "router#show clock\n",
		},
		{
			name:  "cursor movement overwrites selected characters",
			input: "sonic:~$ shXX ip\x1b[5D\x1b[2P\x1b[2@ow\r\n",
			want:  "sonic:~$ show ip\n",
		},
		{
			name:  "osc metadata and character set controls are omitted",
			input: "\x1b]0;admin@host: ~\x07\x1b(B\x1b[32madmin@host\x1b[0m$ true\r\n",
			want:  "admin@host$ true\n",
		},
		{
			name:  "utf8 survives byte split",
			input: "装置名: 東京\r\n",
			want:  "装置名: 東京\n",
		},
		{
			name:  "utf8 encoded c1 ansi control is omitted",
			input: "\xc2\x9b31mred\xc2\x9b0m\r\n",
			want:  "red\n",
		},
		{
			name:  "raw c1 bytes are preserved without charset guessing",
			input: "\x9dwindow title\x9cvisible\r\n",
			want:  "\x9dwindow title\x9cvisible\n",
		},
		{
			name:  "osc hyperlink terminated by st is omitted",
			input: "\x1b]8;;https://example.invalid\x1b\\link\x1b]8;;\x1b\\\r\n",
			want:  "link\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, chunkSize := range []int{len(tt.input), 3, 1} {
				var buf bytes.Buffer
				ll := NewLineLogger(&buf, false)
				writeInChunks(t, ll, []byte(tt.input), chunkSize)
				ll.Flush()
				if got := buf.String(); got != tt.want {
					t.Errorf("chunk size %d: got %q, want %q", chunkSize, got, tt.want)
				}
			}
		})
	}
}

func TestLineLogger_CursorOperations(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "absolute column", input: "abcdef\x1b[3GX\n", want: "abXdef\n"},
		{name: "cursor forward", input: "ab\x1b[2CX\n", want: "ab  X\n"},
		{name: "horizontal position", input: "abcdef\x1b[1;2HX\n", want: "aXcdef\n"},
		{name: "erase through cursor", input: "abcdef\x1b[3G\x1b[1KX\n", want: "  Xdef\n"},
		{name: "erase characters", input: "abcdef\x1b[4G\x1b[2XZ\n", want: "abcZ f\n"},
		{name: "dec save and restore", input: "ab\x1b7cd\x1b8X\n", want: "abXd\n"},
		{name: "ansi save and restore", input: "ab\x1b[scd\x1b[uX\n", want: "abXd\n"},
		{name: "tab stops", input: "a\tb\n", want: "a       b\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			ll := NewLineLogger(&buf, false)
			writeInChunks(t, ll, []byte(tt.input), 1)
			ll.Flush()
			if got := buf.String(); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLineLogger_ProcessAndWriteLineUsesTerminalRendering(t *testing.T) {
	var buf bytes.Buffer
	ll := NewLineLogger(&buf, false)

	ll.ProcessAndWriteLine([]byte("old prompt\r\x1b[2Kclean prompt"))

	if got, want := buf.String(), "clean prompt\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLineLogger_PreservesIncompleteUTF8Bytes(t *testing.T) {
	var buf bytes.Buffer
	ll := NewLineLogger(&buf, false)

	_, _ = ll.Write([]byte{0xe6, 0x97})
	ll.Flush()

	if got, want := buf.Bytes(), []byte{0xe6, 0x97, '\n'}; !bytes.Equal(got, want) {
		t.Fatalf("got %x, want %x", got, want)
	}
}

func TestLineLogger_PreservesLegacyEncodedBytes(t *testing.T) {
	tests := []struct {
		name string
		text []byte
	}{
		{name: "shift jis japanese", text: []byte{0x93, 0xfa, 0x96, 0x7b, 0x8c, 0xea}},
		{name: "euc jp japanese", text: []byte{0xc6, 0xfc, 0xcb, 0xdc, 0xb8, 0xec}},
		{name: "iso 8859 1", text: []byte{'c', 'a', 'f', 0xe9}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := append([]byte("switch#display "), tt.text...)
			input = append(input, '\r', '\n')
			want := append([]byte("switch#display "), tt.text...)
			want = append(want, '\n')

			for _, chunkSize := range []int{len(input), 2, 1} {
				var buf bytes.Buffer
				ll := NewLineLogger(&buf, false)
				writeInChunks(t, ll, input, chunkSize)
				ll.Flush()

				if got := buf.Bytes(); !bytes.Equal(got, want) {
					t.Fatalf("chunk size %d: got %x, want %x", chunkSize, got, want)
				}
				if bytes.Contains(buf.Bytes(), []byte("\xef\xbf\xbd")) {
					t.Fatalf("chunk size %d: replacement character was introduced: %x", chunkSize, buf.Bytes())
				}
			}
		})
	}
}

func TestLineLogger_ConvertsConfiguredEncodingToUTF8(t *testing.T) {
	shiftJIS := []byte{0x93, 0xfa, 0x96, 0x7b, 0x8c, 0xea}
	input := append([]byte("Router#display "), shiftJIS...)
	input = append(input, '\r', '\n')

	for _, chunkSize := range []int{len(input), 2, 1} {
		var buf bytes.Buffer
		ll, err := NewLineLoggerWithEncoding(&buf, false, "cp932", func(err error) {
			t.Fatalf("unexpected conversion warning: %v", err)
		})
		if err != nil {
			t.Fatal(err)
		}

		writeInChunks(t, ll, input, chunkSize)
		ll.Flush()

		if got, want := buf.String(), "Router#display 日本語\n"; got != want {
			t.Fatalf("chunk size %d: got %q, want %q", chunkSize, got, want)
		}
	}
}

func TestLineLogger_ConversionFailurePreservesOriginalBytes(t *testing.T) {
	var buf bytes.Buffer
	warnings := 0
	ll, err := NewLineLoggerWithEncoding(&buf, false, "shift_jis", func(error) {
		warnings++
	})
	if err != nil {
		t.Fatal(err)
	}

	input := []byte{'o', 'k', 0x82, '\n', 'x', 0x82, '\n'}
	_, _ = ll.Write(input)

	if got, want := buf.Bytes(), input; !bytes.Equal(got, want) {
		t.Fatalf("got %x, want original bytes %x", got, want)
	}
	if warnings != 1 {
		t.Fatalf("warnings = %d, want one warning per session", warnings)
	}
	if bytes.Contains(buf.Bytes(), []byte("�")) {
		t.Fatalf("replacement character was introduced: %x", buf.Bytes())
	}
}

func TestLineLogger_RawContentBypassesConfiguredEncoding(t *testing.T) {
	var buf bytes.Buffer
	ll, err := NewLineLoggerWithEncoding(&buf, false, "shift_jis", nil)
	if err != nil {
		t.Fatal(err)
	}

	ll.WriteRaw("機器ヘッダー\n")
	_, _ = ll.Write([]byte{0x93, 0xfa, 0x96, 0x7b, 0x8c, 0xea, '\n'})

	if got, want := buf.String(), "機器ヘッダー\n日本語\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLineLogger_PreservesEveryRawHighBitByte(t *testing.T) {
	var input, want []byte
	for value := 0x80; value <= 0xff; value++ {
		input = append(input, byte(value), '\r', '\n')
		want = append(want, byte(value), '\n')
	}

	for _, chunkSize := range []int{len(input), 7, 1} {
		var buf bytes.Buffer
		ll := NewLineLogger(&buf, false)
		writeInChunks(t, ll, input, chunkSize)
		ll.Flush()

		if got := buf.Bytes(); !bytes.Equal(got, want) {
			t.Fatalf("chunk size %d: raw bytes changed:\ngot  %x\nwant %x", chunkSize, got, want)
		}
	}
}

func TestLineLogger_CurrentLinePreservesOriginalBytes(t *testing.T) {
	var buf bytes.Buffer
	ll := NewLineLogger(&buf, false)
	input := []byte{'R', 'o', 'u', 't', 'e', 'r', '#', 0x93, 0xfa, 0x96, 0x7b}

	writeInChunks(t, ll, input, 1)

	if got := []byte(ll.CurrentLine()); !bytes.Equal(got, input) {
		t.Fatalf("CurrentLine() = %x, want %x", got, input)
	}
}

func TestLineLogger_DeviceTranscripts(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "ubuntu bash",
			input: "\x1b]0;ops@ubuntu: ~\x07\x1b[01;32mops@ubuntu\x1b[00m:\x1b[01;34m~\x1b[00m$ stale" +
				"\r\x1b[2Kops@ubuntu:~$ uptime\r\n 12:00:00 up 1 day\r\n" +
				"\x1b]0;ops@ubuntu: ~\x07ops@ubuntu:~$ ",
			want: "ops@ubuntu:~$ uptime\n 12:00:00 up 1 day\nops@ubuntu:~$\n",
		},
		{
			name: "debian bash",
			input: "\x1b[?2004hroot@debian:/etc# ip addrr\b \b\r\x1b[2K" +
				"root@debian:/etc# ip addr\r\n\x1b[?2004l1: lo: <LOOPBACK,UP>\r\nroot@debian:/etc# ",
			want: "root@debian:/etc# ip addr\n1: lo: <LOOPBACK,UP>\nroot@debian:/etc#\n",
		},
		{
			name: "cisco ios",
			input: "Router#show versoin\r\x1b[2KRouter#show version\r\n" +
				"Cisco IOS XE Software, Version VERSION\r\nRouter#",
			want: "Router#show version\nCisco IOS XE Software, Version VERSION\nRouter#\n",
		},
		{
			name: "arista eos",
			input: "\x1b[?2004hleaf01#show interfaces status\r\n\x1b[?2004l" +
				"Port  Name  Status\r\nEt1         connected\r\nleaf01#",
			want: "leaf01#show interfaces status\nPort  Name  Status\nEt1         connected\nleaf01#\n",
		},
		{
			name:  "juniper junos",
			input: "user@router> show version\r\nHostname: router\r\nModel: example-model\r\nuser@router> ",
			want:  "user@router> show version\nHostname: router\nModel: example-model\nuser@router>\n",
		},
		{
			name:  "huawei vrp",
			input: "<HUAWEI>display version\r\nHuawei Versatile Routing Platform Software\r\n<HUAWEI>",
			want:  "<HUAWEI>display version\nHuawei Versatile Routing Platform Software\n<HUAWEI>\n",
		},
		{
			name: "cumulus linux",
			input: "\x1b[32moperator@example-switch\x1b[0m:\x1b[34m~\x1b[0m$ nv show system\r\n" +
				"hostname  example-switch\r\noperator@example-switch:~$ ",
			want: "operator@example-switch:~$ nv show system\nhostname  example-switch\noperator@example-switch:~$\n",
		},
		{
			name: "sonic os",
			input: "\x1b]0;admin@sonic: ~\x07admin@sonic:~$ show version\r\n" +
				"SONiC Software Version: SONiC.VERSION\r\nadmin@sonic:~$ ",
			want: "admin@sonic:~$ show version\nSONiC Software Version: SONiC.VERSION\nadmin@sonic:~$\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, chunkSize := range []int{len(tt.input), 7, 1} {
				var buf bytes.Buffer
				ll := NewLineLogger(&buf, false)
				writeInChunks(t, ll, []byte(tt.input), chunkSize)
				ll.Flush()
				if got := buf.String(); got != tt.want {
					t.Errorf("chunk size %d: got:\n%q\nwant:\n%q", chunkSize, got, tt.want)
				}
			}
		})
	}
}

func TestLineLogger_FlushesPromptBeforeRawContent(t *testing.T) {
	var buf bytes.Buffer
	ll := NewLineLogger(&buf, false)

	_, _ = ll.Write([]byte("switch#"))
	ll.WriteRaw("FOOTER\n")

	if got, want := buf.String(), "switch#\nFOOTER\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLineLogger_KeepsPromptForCommandEchoAfterHeader(t *testing.T) {
	tests := []struct {
		name    string
		prompt  string
		command string
	}{
		{name: "cisco ios", prompt: "Router#", command: "show version"},
		{name: "arista eos", prompt: "leaf01#", command: "show hostname"},
		{name: "juniper junos", prompt: "user@router> ", command: "show version"},
		{name: "huawei vrp", prompt: "<HUAWEI>", command: "display version"},
		{name: "cumulus linux", prompt: "operator@example-switch:mgmt:~$ ", command: "nv con diff"},
		{name: "sonic os", prompt: "admin@sonic:~$ ", command: "show version"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			ll := NewLineLogger(&buf, false)

			_, _ = ll.Write([]byte(tt.prompt))
			ll.LogHeader(tt.command, true)
			_, _ = ll.Write([]byte(tt.command + "\r\noutput\r\n"))

			wantHeader := "----------------------------------------\n[EXEC] " + tt.command + "\n----------------------------------------\n"
			wantCommandLine := tt.prompt + tt.command + "\n"
			if got := buf.String(); !strings.Contains(got, wantHeader+wantCommandLine+"output\n") {
				t.Fatalf("log = %q, want header followed by %q", got, wantCommandLine)
			}
			if got := countExactLogLine(buf.String(), strings.TrimSpace(tt.prompt)); got != 0 {
				t.Fatalf("standalone prompt count = %d in %q, want zero", got, buf.String())
			}
		})
	}
}

func TestLineLogger_DoesNotRejoinPromptAfterTerminalRedraw(t *testing.T) {
	var buf bytes.Buffer
	ll := NewLineLogger(&buf, false)

	_, _ = ll.Write([]byte("switch#"))
	ll.LogHeader("show version", true)
	_, _ = ll.Write([]byte("\r\x1b[2Kshow version\r\nactual output\r\n"))

	got := buf.String()
	if strings.Contains(got, "switch#show version") {
		t.Fatalf("separately received prompt was synthesized into the redrawn line: %q", got)
	}
	if !strings.Contains(got, "show version\nactual output\n") {
		t.Fatalf("terminal redraw was not retained as displayed: %q", got)
	}
}

func TestLineLogger_FlushesUnterminatedOutputBeforeHeaderWithoutPrompt(t *testing.T) {
	var buf bytes.Buffer
	ll := NewLineLogger(&buf, false)

	_, _ = ll.Write([]byte("unterminated output"))
	ll.LogHeader("show version", false)

	if got := buf.String(); !strings.HasPrefix(got, "unterminated output\n\n----------------------------------------\n") {
		t.Fatalf("unterminated output was attached to a command echo: %q", got)
	}
}

func countExactLogLine(output, want string) int {
	count := 0
	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		if line == want {
			count++
		}
	}
	return count
}

func TestLineLogger_BoundsUntrustedCursorControls(t *testing.T) {
	var buf bytes.Buffer
	ll := NewLineLogger(&buf, false)

	_, _ = ll.Write([]byte("x\x1b[2147483647Cy\x1b[-1Cz\n"))

	if got := len([]rune(buf.String())); got > maxTerminalEdit+5 {
		t.Fatalf("cursor sequence expanded to %d runes", got)
	}
	if !strings.Contains(buf.String(), "y") || !strings.Contains(buf.String(), "z") {
		t.Fatalf("printable content was lost: %q", buf.String())
	}

	buf.Reset()
	ll = NewLineLogger(&buf, false)
	for range 10 {
		_, _ = ll.Write([]byte("\x1b[999999@"))
	}
	_, _ = ll.Write([]byte("end\n"))
	if got := len([]rune(buf.String())); got > maxTerminalEdit+len("end\n") {
		t.Fatalf("repeated insert controls expanded to %d runes", got)
	}
}

func writeInChunks(t *testing.T, w *LineLogger, data []byte, chunkSize int) {
	t.Helper()
	if chunkSize <= 0 {
		chunkSize = len(data)
	}
	for len(data) > 0 {
		n := min(chunkSize, len(data))
		if _, err := w.Write(data[:n]); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		data = data[n:]
	}
}
