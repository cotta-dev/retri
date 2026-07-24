package executor

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/cotta-dev/retri/internal/logger"
	"github.com/creack/pty"
)

func TestCommandOutputRecorder_RecordsCommandAndOutputOnly(t *testing.T) {
	recorder, output := newTestCommandOutputRecorder()

	recorder.ObserveOutput([]byte("Welcome to Ubuntu 26.04\r\nLast login: today\r\n\x1b[32mops@host\x1b[0m:~$ "))
	recorder.ObserveInput([]byte("echo alpha\r"))
	recorder.ObserveOutput([]byte("echo alpha\r\nalpha\r\nops@host:~$ "))
	recorder.Flush()

	want := "ops@host:~$ echo alpha\nalpha\n"
	if got := output.String(); got != want {
		t.Fatalf("log = %q, want %q", got, want)
	}
}

func TestCommandOutputRecorder_RecordsEmptyAndIgnoresCancelledInput(t *testing.T) {
	recorder, output := newTestCommandOutputRecorder()

	recorder.ObserveOutput([]byte("host$ "))
	recorder.ObserveInput([]byte("\r"))
	recorder.ObserveOutput([]byte("\r\nhost$ "))
	recorder.ObserveInput([]byte("unfinished\x03"))
	recorder.ObserveOutput([]byte("unfinished^C\r\nhost$ "))
	recorder.ObserveInput([]byte("printf ok\r"))
	recorder.ObserveOutput([]byte("printf ok\r\nok\r\nhost$ "))
	recorder.Flush()

	if got, want := output.String(), "host$\nhost$ printf ok\nok\n"; got != want {
		t.Fatalf("log = %q, want %q", got, want)
	}
}

func TestCommandOutputRecorder_RecordsConsecutiveEmptySubmissions(t *testing.T) {
	recorder, output := newTestCommandOutputRecorder()

	recorder.ObserveOutput([]byte("host$ "))
	recorder.ObserveInput([]byte("\r\r"))
	recorder.ObserveOutput([]byte("\r\nhost$ \r\nhost$ "))
	recorder.Flush()

	if got, want := output.String(), "host$\nhost$\n"; got != want {
		t.Fatalf("log = %q, want %q", got, want)
	}
}

func TestCommandOutputRecorder_UsesRenderedHistoryAndEditing(t *testing.T) {
	recorder, output := newTestCommandOutputRecorder()

	recorder.ObserveOutput([]byte("host$ "))
	recorder.ObserveInput([]byte("\x1b[A\r"))
	recorder.ObserveOutput([]byte("stale command\r\x1b[2Khost$ git status\r\nOn branch main\r\nhost$ "))
	recorder.Flush()

	if got, want := output.String(), "host$ git status\nOn branch main\n"; got != want {
		t.Fatalf("log = %q, want %q", got, want)
	}
}

func TestCommandOutputRecorder_DoesNotLogCredentials(t *testing.T) {
	recorder, output := newTestCommandOutputRecorder()

	recorder.ObserveOutput([]byte("Are you sure you want to continue connecting (yes/no/[fingerprint])? "))
	recorder.ObserveInput([]byte("yes\r"))
	recorder.ObserveOutput([]byte("yes\r\nWarning: permanently added router\r\n"))

	// Login authentication happens before any command and must be discarded.
	recorder.ObserveOutput([]byte("admin@router's password: "))
	recorder.ObserveInput([]byte("login-secret\r"))
	recorder.ObserveOutput([]byte("\r\nrouter#"))

	recorder.ObserveInput([]byte("enable\r"))
	recorder.ObserveOutput([]byte("enable\r\nPassword: "))
	recorder.ObserveInput([]byte("enable-secret\r"))
	recorder.ObserveOutput([]byte("\r\nPrivilege level 15\r\nrouter#"))
	recorder.ObserveInput([]byte("show clock\r"))
	recorder.ObserveOutput([]byte("show clock\r\n12:00:00 UTC\r\nrouter#"))
	recorder.Flush()

	got := output.String()
	if strings.Contains(got, "secret") || strings.Contains(got, "password") || strings.Contains(got, "Password") {
		t.Fatalf("credential data leaked to log: %q", got)
	}
	if !strings.Contains(got, "router#enable\n") || !strings.Contains(got, "Privilege level 15\n") || !strings.Contains(got, "12:00:00 UTC\n") {
		t.Fatalf("command output missing after credential prompt: %q", got)
	}
}

func TestCommandOutputRecorder_DoesNotLogEmptyCredentialSubmission(t *testing.T) {
	recorder, output := newTestCommandOutputRecorder()

	recorder.ObserveOutput([]byte("Password: "))
	recorder.ObserveInput([]byte("\r"))
	recorder.ObserveOutput([]byte("\r\nAuthentication failed\r\nRouter#"))
	recorder.ObserveInput([]byte("\r"))
	recorder.ObserveOutput([]byte("\r\nRouter#"))
	recorder.Flush()

	if got, want := output.String(), "Router#\n"; got != want {
		t.Fatalf("log = %q, want %q", got, want)
	}
}

func TestCommandOutputRecorder_NetworkDeviceTranscript(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		input  string
		body   string
		want   string
	}{
		{name: "cisco ios", prompt: "Router#", input: "show version\r", body: "show version\r\nCisco IOS XE VERSION\r\nRouter#", want: "Router#show version\nCisco IOS XE VERSION\n"},
		{name: "arista eos", prompt: "leaf#", input: "show hostname\r", body: "show hostname\r\nHostname: leaf\r\nleaf#", want: "leaf#show hostname\nHostname: leaf\n"},
		{name: "juniper junos", prompt: "user@router> ", input: "show version\r", body: "show version\r\nModel: example-model\r\nuser@router> ", want: "user@router> show version\nModel: example-model\n"},
		{name: "huawei vrp", prompt: "<HUAWEI>", input: "display version\r", body: "display version\r\nVRP software\r\n<HUAWEI>", want: "<HUAWEI>display version\nVRP software\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder, output := newTestCommandOutputRecorder()
			writeRecorderChunks(recorder, []byte("login banner\r\n"+tt.prompt), 1)
			recorder.ObserveInput([]byte(tt.input))
			writeRecorderChunks(recorder, []byte(tt.body), 1)
			recorder.Flush()
			if got := output.String(); got != tt.want {
				t.Fatalf("log = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCommandOutputRecorder_PreservesLegacyEncodedOutput(t *testing.T) {
	recorder, output := newTestCommandOutputRecorder()
	recorder.ObserveOutput([]byte("Router#"))
	recorder.ObserveInput([]byte("show interface description\r"))

	transcript := []byte("show interface description\r\nGi0/0  ")
	transcript = append(transcript, []byte{0x93, 0xfa, 0x96, 0x7b, 0x8c, 0xea}...)
	transcript = append(transcript, []byte("\r\nRouter#")...)
	writeRecorderChunks(recorder, transcript, 1)
	recorder.Flush()

	want := []byte("Router#show interface description\nGi0/0  ")
	want = append(want, []byte{0x93, 0xfa, 0x96, 0x7b, 0x8c, 0xea}...)
	want = append(want, '\n')
	if got := output.Bytes(); !bytes.Equal(got, want) {
		t.Fatalf("log = %x, want %x", got, want)
	}
}

func TestCommandOutputRecorder_ConvertsConfiguredNetworkEncoding(t *testing.T) {
	var output bytes.Buffer
	target, err := logger.NewLineLoggerWithEncoding(&output, false, "shift_jis", func(err error) {
		t.Fatalf("unexpected conversion warning: %v", err)
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder := NewCommandOutputRecorder(target)
	recorder.ObserveOutput([]byte("Router#"))
	recorder.ObserveInput([]byte("show interface description\r"))

	transcript := []byte("show interface description\r\nGi0/0  ")
	transcript = append(transcript, []byte{0x93, 0xfa, 0x96, 0x7b, 0x8c, 0xea}...)
	transcript = append(transcript, []byte("\r\nRouter#")...)
	writeRecorderChunks(recorder, transcript, 1)
	recorder.Flush()

	if got, want := output.String(), "Router#show interface description\nGi0/0  日本語\n"; got != want {
		t.Fatalf("log = %q, want %q", got, want)
	}
}

func TestCommandOutputRecorder_NetworkContextHelp(t *testing.T) {
	recorder, output := newTestCommandOutputRecorder()

	writeRecorderChunks(recorder, []byte("login banner\r\nRouter#"), 1)
	recorder.ObserveInput([]byte("show ?"))
	writeRecorderChunks(recorder, []byte("show ?\r\n  interfaces  Interface status\r\n  version     Software version\r\nRouter#show "), 1)
	recorder.ObserveInput([]byte("version\r"))
	writeRecorderChunks(recorder, []byte("version\r\nCisco IOS XE VERSION\r\nRouter#"), 1)
	recorder.Flush()

	want := "Router#show ?\n  interfaces  Interface status\n  version     Software version\nRouter#show version\nCisco IOS XE VERSION\n"
	if got := output.String(); got != want {
		t.Fatalf("log = %q, want %q", got, want)
	}
}

func TestCommandOutputRecorder_DiscardsCancelledNetworkContextHelp(t *testing.T) {
	recorder, output := newTestCommandOutputRecorder()

	recorder.ObserveOutput([]byte("Router#"))
	recorder.ObserveInput([]byte("show ?"))
	recorder.ObserveOutput([]byte("show ?\r\n  version  Software version\r\nRouter#show "))
	recorder.ObserveInput([]byte("\x03"))
	recorder.ObserveOutput([]byte("^C\r\nRouter#"))
	recorder.ObserveInput([]byte("show clock\r"))
	recorder.ObserveOutput([]byte("show clock\r\n12:00:00 UTC\r\nRouter#"))
	recorder.Flush()

	want := "Router#show clock\n12:00:00 UTC\n"
	if got := output.String(); got != want {
		t.Fatalf("log = %q, want %q", got, want)
	}
}

func TestCommandOutputRecorder_PagerKeysDoNotDelayOutput(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		pager  string
	}{
		{name: "cisco ios", prompt: "Router#", pager: "--More--"},
		{name: "arista eos", prompt: "leaf#", pager: "--More-- or (q)uit"},
		{name: "juniper junos", prompt: "user@mx> ", pager: "---(more 51%)---"},
		{name: "huawei vrp", prompt: "<HUAWEI>", pager: "---- More ----"},
		{name: "cumulus linux less", prompt: "cumulus@leaf:~$ ", pager: ":"},
		{name: "sonic os less", prompt: "admin@sonic:~$ ", pager: "(END)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder, output := newTestCommandOutputRecorder()
			recorder.ObserveOutput([]byte(tt.prompt))
			recorder.ObserveInput([]byte("show running-config\r"))
			writeRecorderChunks(recorder, []byte("show running-config\r\npage one\r\n"+tt.pager), 1)

			// Space and q are pager controls, not submitted commands. Page two
			// must already be durable even if the session ends without another
			// Enter key after leaving the pager.
			recorder.ObserveInput([]byte(" "))
			writeRecorderChunks(recorder, []byte("\r\x1b[2Kpage two\r\n"+tt.pager), 1)
			recorder.ObserveInput([]byte("q"))
			writeRecorderChunks(recorder, []byte("\r\x1b[2K"+tt.prompt), 1)
			recorder.Flush()

			want := tt.prompt + "show running-config\npage one\npage two\n"
			if got := output.String(); got != want {
				t.Fatalf("log = %q, want %q", got, want)
			}
		})
	}
}

func TestCommandOutputRecorder_RemovesCompletedPagerLines(t *testing.T) {
	recorder, output := newTestCommandOutputRecorder()

	recorder.ObserveOutput([]byte("Router#"))
	recorder.ObserveInput([]byte("show log\r"))
	recorder.ObserveOutput([]byte("show log\r\npage one\r\n--More--\r\n"))
	recorder.ObserveInput([]byte(" "))
	recorder.ObserveOutput([]byte("page two\r\nRouter#"))
	recorder.Flush()

	want := "Router#show log\npage one\npage two\n"
	if got := output.String(); got != want {
		t.Fatalf("log = %q, want %q", got, want)
	}
}

func TestCommandOutputRecorder_BracketedMultilinePaste(t *testing.T) {
	recorder, output := newTestCommandOutputRecorder()

	recorder.ObserveOutput([]byte("host$ "))
	recorder.ObserveInput([]byte("\x1b[200~echo one\n"))
	recorder.ObserveOutput([]byte("echo one\r\n> "))
	recorder.ObserveInput([]byte("echo two\x1b[201~\r"))
	recorder.ObserveOutput([]byte("echo two\r\none\r\ntwo\r\nhost$ "))
	recorder.Flush()

	want := "host$ echo one\n> echo two\none\ntwo\n"
	if got := output.String(); got != want {
		t.Fatalf("log = %q, want %q", got, want)
	}
}

func TestCommandOutputRecorder_HandlesQueuedSubmissions(t *testing.T) {
	recorder, output := newTestCommandOutputRecorder()

	recorder.ObserveOutput([]byte("host$ "))
	// Model a single stdin read containing an empty Enter followed immediately
	// by two commands before their PTY echoes are read.
	recorder.ObserveInput([]byte("\recho one\recho two\r"))
	recorder.ObserveOutput([]byte("\r\nhost$ echo one\r\none\r\nhost$ echo two\r\ntwo\r\nhost$ "))
	recorder.Flush()

	want := "host$\nhost$ echo one\none\nhost$ echo two\ntwo\n"
	if got := output.String(); got != want {
		t.Fatalf("log = %q, want %q", got, want)
	}
}

func TestCommandOutputRecorder_WithBashPTY(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is unavailable")
	}

	recorder, output := newTestCommandOutputRecorder()
	cmd := exec.Command(bash, "--noprofile", "--norc", "-i")
	cmd.Env = append(os.Environ(), "PS1=prompt$ ", "TERM=xterm", "HISTFILE=/dev/null")
	ptmx, err := pty.Start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ptmx.Close() }()
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	outputCh := make(chan []byte, 32)
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		buf := make([]byte, 4096)
		for {
			n, readErr := ptmx.Read(buf)
			if n > 0 {
				chunk := append([]byte(nil), buf[:n]...)
				recorder.ObserveOutput(chunk)
				outputCh <- chunk
			}
			if readErr != nil {
				return
			}
		}
	}()

	waitForRecorderOutput(t, outputCh, "prompt$ ")
	sendRecorderInput(t, ptmx, recorder, "echo alpha\r")
	waitForRecorderOutput(t, outputCh, "alpha\nprompt$ ")
	sendRecorderInput(t, ptmx, recorder, "\r")
	waitForRecorderOutput(t, outputCh, "prompt$ ")
	sendRecorderInput(t, ptmx, recorder, "\x1b[A\r")
	waitForRecorderOutput(t, outputCh, "alpha\nprompt$ ")
	sendRecorderInput(t, ptmx, recorder, "exit\r")
	waitForRecorderOutput(t, outputCh, "exit")

	_ = ptmx.Close()
	<-doneCh
	recorder.Flush()

	got := output.String()
	if countRecorderLine(got, "prompt$ echo alpha") != 2 || countRecorderLine(got, "alpha") != 2 {
		t.Fatalf("bash command/history log is incomplete: %q", got)
	}
	if countRecorderLine(got, "prompt$") != 1 {
		t.Fatalf("bash empty submission was not logged exactly once: %q", got)
	}
	if strings.Contains(got, "cannot set terminal process group") || strings.Contains(got, "\x1b") {
		t.Fatalf("bash log contains session or input noise: %q", got)
	}
}

func countRecorderLine(output, want string) int {
	count := 0
	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		if line == want {
			count++
		}
	}
	return count
}

func newTestCommandOutputRecorder() (*CommandOutputRecorder, *bytes.Buffer) {
	var output bytes.Buffer
	target := logger.NewLineLogger(&output, false)
	return NewCommandOutputRecorder(target), &output
}

func writeRecorderChunks(recorder *CommandOutputRecorder, data []byte, size int) {
	for len(data) > 0 {
		n := min(size, len(data))
		recorder.ObserveOutput(data[:n])
		data = data[n:]
	}
}

func sendRecorderInput(t *testing.T, ptmx *os.File, recorder *CommandOutputRecorder, input string) {
	t.Helper()
	recorder.ObserveInput([]byte(input))
	if _, err := ptmx.Write([]byte(input)); err != nil {
		t.Fatal(err)
	}
}

func waitForRecorderOutput(t *testing.T, outputCh <-chan []byte, want string) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	var seen strings.Builder
	for {
		select {
		case chunk, ok := <-outputCh:
			if !ok {
				t.Fatalf("PTY closed before %q; saw %q", want, seen.String())
			}
			seen.Write(chunk)
			visible := strings.ReplaceAll(logger.StripAnsi(seen.String()), "\r", "")
			if strings.Contains(visible, want) {
				return
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for %q; saw %q", want, seen.String())
		}
	}
}
