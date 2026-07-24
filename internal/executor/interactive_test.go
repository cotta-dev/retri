package executor

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cotta-dev/retri/internal/logger"
)

func TestHandlePrompts_ReportsLogWriteFailure(t *testing.T) {
	reader := &chunkReadWriter{data: []byte("Router#"), chunkSize: 1}
	doneCh := make(chan error, 1)
	handlePrompts(reader, failingLogWriter{}, "", "", `[#>] ?$`, make(chan struct{}, 1), nil, doneCh, false)

	if err := <-doneCh; !errors.Is(err, errSessionLogWrite) {
		t.Fatalf("handlePrompts() error = %v, want %v", err, errSessionLogWrite)
	}
}

func TestHandlePrompts_ReportsPasswordResponseFailure(t *testing.T) {
	reader := &failingResponseReadWriter{data: []byte("Password: ")}
	doneCh := make(chan error, 1)
	var output bytes.Buffer

	handlePrompts(reader, &output, "secret", "", `[#>] ?$`, make(chan struct{}, 1), nil, doneCh, false)

	if err := <-doneCh; !errors.Is(err, errSessionLogWrite) {
		t.Fatalf("handlePrompts() error = %v, want %v", err, errSessionLogWrite)
	}
}

func TestWriteInteractiveInputRejectsShortWrite(t *testing.T) {
	if err := writeInteractiveInput(shortWriter{}, "show version\n"); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("writeInteractiveInput() error = %v, want %v", err, io.ErrShortWrite)
	}
}

func TestValidateInteractiveExit(t *testing.T) {
	if err := validateInteractiveExit(nil, true); err != nil {
		t.Fatalf("clean exit rejected: %v", err)
	}
	if err := validateInteractiveExit(commandExitError(t, 1), true); err != nil {
		t.Fatalf("remote command status inherited by exit was rejected: %v", err)
	}
	if err := validateInteractiveExit(commandExitError(t, 255), true); err == nil {
		t.Fatal("SSH transport failure was accepted")
	}
	if err := validateInteractiveExit(commandSignalError(t), true); err == nil {
		t.Fatal("signal-based termination was accepted")
	}
	if err := validateInteractiveExit(nil, false); err == nil {
		t.Fatal("session ending before exit was accepted")
	}
}

func TestRunInteractive_CumulusPromptCommandOrderAndInheritedExitStatus(t *testing.T) {
	dir := t.TempDir()
	fakeSSH := filepath.Join(dir, "ssh")
	script := `#!/bin/sh
prompt='operator@example-switch:mgmt:~$ '
printf '%s' "$prompt"
while IFS= read -r command; do
    case "$command" in
        'nv con diff')
            printf 'diff output\r\n'
            ;;
        'nv con show')
            printf '%s\r\n' '- header:' '    model: example-model' '    version: Cumulus Linux VERSION'
            ;;
        'sudo cat /etc/ifplugd/action.d/ifupdown')
            printf '%s\r\n' 'cat: /etc/ifplugd/action.d/ifupdown: No such file or directory'
            ;;
        'exit')
            printf 'logout\r\n'
            exit 1
            ;;
    esac
    printf '%s' "$prompt"
done
`
	if err := os.WriteFile(fakeSSH, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var output bytes.Buffer
	ll := logger.NewLineLogger(&output, false)
	commands := []string{"nv con diff", "nv con show", "sudo cat /etc/ifplugd/action.d/ifupdown"}
	if ok := RunInteractive(
		"example-switch",
		"operator",
		commands,
		ll,
		ll,
		"",
		"",
		`\$ ?$`,
		"exit",
		2*time.Second,
		false,
	); !ok {
		t.Fatalf("RunInteractive() reported a clean Cumulus logout as failure:\n%s", output.String())
	}
	ll.Flush()

	got := output.String()
	for _, command := range commands {
		want := "[EXEC] " + command + "\n----------------------------------------\n" +
			"operator@example-switch:mgmt:~$ " + command + "\n"
		if !strings.Contains(got, want) {
			t.Fatalf("command was not logged after its header:\nwant fragment %q\nlog:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "operator@example-switch:mgmt:~$ exit\nlogout\n") {
		t.Fatalf("exit/logout sequence missing from log:\n%s", got)
	}
	if got := countExactLines(output.String(), "operator@example-switch:mgmt:~$"); got != 0 {
		t.Fatalf("standalone prompt count = %d, want zero:\n%s", got, output.String())
	}
}

func commandExitError(t *testing.T, code int) error {
	t.Helper()
	err := exec.Command("sh", "-c", fmt.Sprintf("exit %d", code)).Run()
	if err == nil {
		t.Fatalf("exit %d unexpectedly succeeded", code)
	}
	return err
}

func commandSignalError(t *testing.T) error {
	t.Helper()
	err := exec.Command("sh", "-c", "kill -TERM $$").Run()
	if err == nil {
		t.Fatal("signal termination unexpectedly succeeded")
	}
	return err
}

type terminalFixture struct {
	name        string
	promptRegex string
	transcript  string
	wantPrompt  string
}

func TestHandlePrompts_NetworkDeviceTranscripts(t *testing.T) {
	fixtures := []terminalFixture{
		{
			name:        "cisco ios exec and config prompts",
			promptRegex: `(?:[A-Za-z0-9._-]+)(?:\([^\r\n)]*\))?[#>] ?$`,
			transcript:  "Router#show version\r\nCisco IOS XE Software, Version VERSION\r\nRouter(config)#",
			wantPrompt:  "Router(config)#",
		},
		{
			name:        "arista eos",
			promptRegex: `[#>] ?$`,
			transcript:  "leaf01#show hostname\r\nHostname: leaf01\r\nleaf01#",
			wantPrompt:  "leaf01#",
		},
		{
			name:        "juniper junos",
			promptRegex: `[%#>] ?$`,
			transcript:  "user@mx> show version\r\nHostname: mx\r\nuser@mx>",
			wantPrompt:  "user@mx>",
		},
		{
			name:        "huawei vrp user view",
			promptRegex: `(?:<[^>]+>|\[[^]]+\]) ?$`,
			transcript:  "<HUAWEI>display version\r\nVRP (R) software\r\n<HUAWEI>",
			wantPrompt:  "<HUAWEI>",
		},
		{
			name:        "huawei vrp system view",
			promptRegex: `(?:<[^>]+>|\[[^]]+\]) ?$`,
			transcript:  "<HUAWEI>system-view\r\nEnter system view\r\n[HUAWEI]",
			wantPrompt:  "[HUAWEI]",
		},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			for _, chunkSize := range []int{len(fixture.transcript), 5, 1} {
				reader := &chunkReadWriter{data: []byte(fixture.transcript), chunkSize: chunkSize}
				var output bytes.Buffer
				ll := logger.NewLineLogger(&output, false)
				promptCh := make(chan struct{}, 10)
				doneCh := make(chan error, 1)

				handlePrompts(reader, ll, "", "", fixture.promptRegex, promptCh, nil, doneCh, false)
				<-doneCh
				ll.Flush()

				if len(promptCh) == 0 {
					t.Fatalf("chunk size %d: prompt was not detected", chunkSize)
				}
				if got := countExactLines(output.String(), fixture.wantPrompt); got != 1 {
					t.Fatalf("chunk size %d: prompt count = %d in %q", chunkSize, got, output.String())
				}
			}
		})
	}
}

func countExactLines(output, want string) int {
	count := 0
	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		if line == want {
			count++
		}
	}
	return count
}

func TestHandlePrompts_SendsPasswordWithoutLoggingSecret(t *testing.T) {
	reader := &chunkReadWriter{data: []byte("admin@host's password: "), chunkSize: 1}
	var output bytes.Buffer
	ll := logger.NewLineLogger(&output, false)
	doneCh := make(chan error, 1)

	handlePrompts(reader, ll, "top-secret", "", `[#>] ?$`, make(chan struct{}, 1), nil, doneCh, false)
	<-doneCh
	ll.Flush()

	if got, want := reader.writes.String(), "top-secret\n"; got != want {
		t.Fatalf("password response = %q, want %q", got, want)
	}
	if strings.Contains(output.String(), "top-secret") {
		t.Fatalf("password leaked to log: %q", output.String())
	}
}

func TestHandlePrompts_PreservesLegacyEncodedOutput(t *testing.T) {
	transcript := []byte("Router#show interfaces description\r\nGi0/0  ")
	transcript = append(transcript, []byte{0x93, 0xfa, 0x96, 0x7b, 0x8c, 0xea}...)
	transcript = append(transcript, []byte("\r\nRouter#")...)
	reader := &chunkReadWriter{data: transcript, chunkSize: 1}
	var output bytes.Buffer
	ll := logger.NewLineLogger(&output, false)
	promptCh := make(chan struct{}, 10)
	doneCh := make(chan error, 1)

	handlePrompts(reader, ll, "", "", `[#>] ?$`, promptCh, nil, doneCh, false)
	<-doneCh
	ll.Flush()

	wantLine := []byte("Gi0/0  ")
	wantLine = append(wantLine, []byte{0x93, 0xfa, 0x96, 0x7b, 0x8c, 0xea}...)
	wantLine = append(wantLine, '\n')
	if !bytes.Contains(output.Bytes(), wantLine) {
		t.Fatalf("legacy-encoded line changed: log=%x, want line=%x", output.Bytes(), wantLine)
	}
	if len(promptCh) == 0 {
		t.Fatal("ASCII prompt was not detected after legacy-encoded output")
	}
}

func TestHandlePrompts_ConvertsConfiguredNetworkEncoding(t *testing.T) {
	transcript := []byte("Router#show interfaces description\r\nGi0/0  ")
	transcript = append(transcript, []byte{0x93, 0xfa, 0x96, 0x7b, 0x8c, 0xea}...)
	transcript = append(transcript, []byte("\r\nRouter#")...)
	reader := &chunkReadWriter{data: transcript, chunkSize: 1}
	var output bytes.Buffer
	ll, err := logger.NewLineLoggerWithEncoding(&output, false, "cp932", func(err error) {
		t.Fatalf("unexpected conversion warning: %v", err)
	})
	if err != nil {
		t.Fatal(err)
	}
	promptCh := make(chan struct{}, 10)
	doneCh := make(chan error, 1)

	handlePrompts(reader, ll, "", "", `[#>] ?$`, promptCh, nil, doneCh, false)
	<-doneCh
	ll.Flush()

	if !strings.Contains(output.String(), "Gi0/0  日本語\n") {
		t.Fatalf("configured encoding was not converted: %q", output.String())
	}
	if len(promptCh) == 0 {
		t.Fatal("ASCII prompt was not detected after converted output")
	}
}

type chunkReadWriter struct {
	data      []byte
	chunkSize int
	writes    bytes.Buffer
}

func (rw *chunkReadWriter) Read(p []byte) (int, error) {
	if len(rw.data) == 0 {
		return 0, io.EOF
	}
	n := min(len(p), len(rw.data))
	if rw.chunkSize > 0 {
		n = min(n, rw.chunkSize)
	}
	copy(p, rw.data[:n])
	rw.data = rw.data[n:]
	return n, nil
}

func (rw *chunkReadWriter) Write(p []byte) (int, error) {
	return rw.writes.Write(p)
}

type failingResponseReadWriter struct {
	data []byte
}

func (rw *failingResponseReadWriter) Read(p []byte) (int, error) {
	if len(rw.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, rw.data)
	rw.data = rw.data[n:]
	return n, nil
}

func (*failingResponseReadWriter) Write([]byte) (int, error) {
	return 0, errSessionLogWrite
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) {
	return len(p) - 1, nil
}
