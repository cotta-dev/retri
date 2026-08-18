package executor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/cotta-dev/retri/internal/config"
)

func TestAutomatedPromptRegex(t *testing.T) {
	tests := []struct {
		name string
		host config.ResolvedHost
		want string
	}{
		{
			name: "linux shell default",
			host: config.ResolvedHost{DeviceType: config.DefaultDeviceType},
			want: config.DefaultShellPromptRegex,
		},
		{
			name: "network CLI default",
			host: config.ResolvedHost{DeviceType: "cisco_ios"},
			want: config.DefaultPromptRegex,
		},
		{
			name: "device override",
			host: config.ResolvedHost{
				DeviceType:   config.DefaultDeviceType,
				DeviceConfig: config.DeviceConfig{PromptRegex: `CUSTOM(?:#|>) ?$`},
			},
			want: `CUSTOM(?:#|>) ?$`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := automatedPromptRegex(tt.host)
			if got != tt.want {
				t.Fatalf("automatedPromptRegex() = %q, want %q", got, tt.want)
			}
			if _, err := regexp.Compile(got); err != nil {
				t.Fatalf("automatedPromptRegex() returned invalid regex %q: %v", got, err)
			}
		})
	}
}

func TestExecuteHostTask_LinuxUsesRealPTYTranscript(t *testing.T) {
	binDir := t.TempDir()
	writeFakeSSH(t, binDir, `#!/bin/sh
prompt='tester@linux-host:~$ '
printf '%s' "$prompt"
while IFS= read -r command; do
    case "$command" in
        'pwd')
            printf '/home/tester\r\n'
            ;;
        'sudo whoami')
            stty -echo
            printf '[sudo] password for tester: '
            IFS= read -r sudo_password
            printf '\r\n'
            stty echo
            if [ "$sudo_password" = 'sudo-secret' ]; then
                printf 'root\r\n'
            else
                printf 'bad password\r\n'
            fi
            unset sudo_password
            ;;
        'exit')
            printf 'logout\r\n'
            exit 0
            ;;
    esac
    printf '%s' "$prompt"
done
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	logDir := t.TempDir()
	noTimestamp := false
	rh := config.ResolvedHost{
		HostConfig: config.HostConfig{
			Host: "linux-host",
			CommonFields: config.CommonFields{
				User:          "tester",
				Commands:      []string{"pwd", "sudo whoami"},
				PromptTimeout: 2,
			},
		},
		DeviceConfig: config.DeviceConfig{ExitCommand: "exit"},
		DeviceType:   config.DefaultDeviceType,
	}
	defaults := config.GlobalOptions{
		Timestamp:      &noTimestamp,
		FilenameFormat: "{host}.log",
	}

	ExecuteHostTask(rh, defaults, HostTaskOptions{
		LogDir: logDir,
		Secret: "sudo-secret",
	})

	logBytes, err := os.ReadFile(filepath.Join(logDir, "linux-host.log"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(logBytes)
	for _, want := range []string{
		"[EXEC] pwd\n----------------------------------------\ntester@linux-host:~$ pwd\n/home/tester\n",
		"[EXEC] sudo whoami\n----------------------------------------\ntester@linux-host:~$ sudo whoami\n[sudo] password for tester:\nroot\n",
		"tester@linux-host:~$ exit\nlogout\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("real PTY transcript is missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "sudo-secret") {
		t.Fatalf("sudo secret leaked to the session log:\n%s", got)
	}
	if strings.Contains(got, "[ERROR] Interactive execution failed.") {
		t.Fatalf("clean Linux logout was reported as a failure:\n%s", got)
	}
}

func TestExecuteHostTask_LinuxBatchDoesNotPersistShellHistory(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is unavailable")
	}

	binDir := t.TempDir()
	writeFakeSSH(t, binDir, fmt.Sprintf("#!/bin/sh\nexec %q --noprofile --norc -i\n", bash))
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PS1", "tester@history-host:~$ ")
	t.Setenv("PROMPT_COMMAND", "")
	t.Setenv("HISTCONTROL", "")
	historyPath := filepath.Join(t.TempDir(), "bash_history")
	const existingHistory = "existing-command\n"
	if err := os.WriteFile(historyPath, []byte(existingHistory), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HISTFILE", historyPath)

	noTimestamp := false
	logDir := t.TempDir()
	rh := config.ResolvedHost{
		HostConfig: config.HostConfig{
			Host: "history-host",
			CommonFields: config.CommonFields{
				User:          "tester",
				Commands:      []string{"echo batch-marker"},
				PromptTimeout: 2,
			},
		},
		DeviceConfig: config.DeviceConfig{ExitCommand: "exit"},
		DeviceType:   config.DefaultDeviceType,
	}
	defaults := config.GlobalOptions{
		Timestamp:      &noTimestamp,
		FilenameFormat: "{host}.log",
	}

	ExecuteHostTask(rh, defaults, HostTaskOptions{LogDir: logDir})

	history, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(history); got != existingHistory {
		t.Fatalf("persistent shell history changed: got %q, want %q", got, existingHistory)
	}
	logBytes, err := os.ReadFile(filepath.Join(logDir, "history-host.log"))
	if err != nil {
		t.Fatal(err)
	}
	logText := string(logBytes)
	for _, command := range []string{disableShellHistoryCommand, "echo batch-marker"} {
		if !strings.Contains(logText, "[EXEC] "+command+"\n") {
			t.Fatalf("execution log is missing %q:\n%s", command, logText)
		}
	}
}

func TestExecuteHostTask_LogsExecHeaderForEveryCommandSource(t *testing.T) {
	binDir := t.TempDir()
	writeFakeSSH(t, binDir, `#!/bin/sh
prompt='tester@all-sources:~$ '
printf '%s' "$prompt"
while IFS= read -r command; do
    if [ "$command" = 'exit' ]; then
        printf 'logout\r\n'
        exit 0
    fi
    printf 'output:%s\r\n' "$command"
    printf '%s' "$prompt"
done
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	commandDir := t.TempDir()
	defaultFile := writeCommandFile(t, commandDir, "defaults.txt", "default-file")
	groupFile := writeCommandFile(t, commandDir, "group.txt", "group-file")
	deviceFile := writeCommandFile(t, commandDir, "device.txt", "device-file")
	hostFile := writeCommandFile(t, commandDir, "host.txt", "host-file")
	cliFile := writeCommandFile(t, commandDir, "cli.txt", "cli-file")

	rh := config.ResolvedHost{
		HostConfig: config.HostConfig{
			Host: "all-sources",
			CommonFields: config.CommonFields{
				User:          "tester",
				CommandFile:   hostFile,
				Commands:      []string{"host-list"},
				Command:       "host-single",
				PromptTimeout: 2,
			},
		},
		GroupConfigs: []config.GroupConfig{{
			CommonFields: config.CommonFields{
				CommandFile: groupFile,
				Commands:    []string{"group-list"},
				Command:     "group-single",
			},
		}},
		DeviceConfig: config.DeviceConfig{
			SetupCommands: []string{"device-setup"},
			ExitCommand:   "exit",
			CommonFields: config.CommonFields{
				CommandFile: deviceFile,
				Commands:    []string{"device-list"},
				Command:     "device-single",
			},
		},
		DeviceType: config.DefaultDeviceType,
	}
	noTimestamp := false
	defaults := config.GlobalOptions{
		Timestamp:      &noTimestamp,
		FilenameFormat: "{host}.log",
		CommonFields: config.CommonFields{
			CommandFile: defaultFile,
			Commands:    []string{"default-list"},
			Command:     "default-single",
		},
	}
	logDir := t.TempDir()

	ExecuteHostTask(rh, defaults, HostTaskOptions{
		CommandFile: cliFile,
		Command:     "cli-single",
		LogDir:      logDir,
	})

	logBytes, err := os.ReadFile(filepath.Join(logDir, "all-sources.log"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(logBytes)
	wantCommands := []string{
		disableShellHistoryCommand,
		"device-setup",
		"default-file", "default-list", "default-single",
		"group-file", "group-list", "group-single",
		"device-file", "device-list", "device-single",
		"host-file", "host-list", "host-single",
		"cli-file", "cli-single",
	}
	for _, command := range wantCommands {
		header := "[EXEC] " + command + "\n"
		if count := strings.Count(got, header); count != 1 {
			t.Fatalf("header count for %q = %d, want 1:\n%s", command, count, got)
		}
	}
	if count := strings.Count(got, "[EXEC] "); count != len(wantCommands) {
		t.Fatalf("[EXEC] header count = %d, want %d:\n%s", count, len(wantCommands), got)
	}
}

func writeFakeSSH(t *testing.T, dir, script string) {
	t.Helper()
	path := filepath.Join(dir, "ssh")
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
}
