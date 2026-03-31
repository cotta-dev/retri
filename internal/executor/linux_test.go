package executor

import (
	"strings"
	"testing"
)

func TestBuildBatchCommand(t *testing.T) {
	tests := []struct {
		name     string
		commands []string
		password string
		secret   string
		check    func(t *testing.T, result string)
	}{
		{
			name:     "simple commands",
			commands: []string{"df -h", "uptime"},
			check: func(t *testing.T, result string) {
				if !strings.Contains(result, "[EXEC] df -h") {
					t.Error("expected [EXEC] df -h header")
				}
				if !strings.Contains(result, "df -h;") {
					t.Error("expected df -h command")
				}
				if !strings.Contains(result, "[EXEC] uptime") {
					t.Error("expected [EXEC] uptime header")
				}
			},
		},
		{
			name:     "skip empty and comment lines",
			commands: []string{"", "# comment", "  ", "hostname"},
			check: func(t *testing.T, result string) {
				if strings.Contains(result, "# comment") {
					t.Error("expected comments to be skipped")
				}
				if !strings.Contains(result, "hostname") {
					t.Error("expected hostname command")
				}
			},
		},
		{
			name:     "sudo with password",
			commands: []string{"sudo systemctl status sshd"},
			password: "mypass",
			check: func(t *testing.T, result string) {
				if !strings.Contains(result, "echo 'mypass' | sudo -S -p ''") {
					t.Error("expected sudo to be converted to password piping")
				}
			},
		},
		{
			name:     "sudo with secret overrides password",
			commands: []string{"sudo cat /etc/shadow"},
			password: "sshpass",
			secret:   "sudopass",
			check: func(t *testing.T, result string) {
				if !strings.Contains(result, "echo 'sudopass' | sudo -S -p ''") {
					t.Error("expected secret to be used for sudo, not password")
				}
			},
		},
		{
			name:     "sudo without password",
			commands: []string{"sudo hostname"},
			password: "",
			secret:   "",
			check: func(t *testing.T, result string) {
				if strings.Contains(result, "sudo -S") {
					t.Error("expected no password piping when password is empty")
				}
				if !strings.Contains(result, "sudo hostname") {
					t.Error("expected sudo command to remain unchanged")
				}
			},
		},
		{
			name:     "password with single quotes",
			commands: []string{"sudo test"},
			password: "it's",
			check: func(t *testing.T, result string) {
				if !strings.Contains(result, "'it'\\''s'") {
					t.Error("expected single quotes to be escaped")
				}
			},
		},
		{
			name:     "command with double quotes",
			commands: []string{`echo "hello"`},
			check: func(t *testing.T, result string) {
				if !strings.Contains(result, `[EXEC] echo \"hello\"`) {
					t.Error("expected double quotes to be escaped in log header")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildBatchCommand(tt.commands, tt.password, tt.secret)
			tt.check(t, result)
		})
	}
}
