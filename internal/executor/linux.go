package executor

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var sudoRe = regexp.MustCompile(`\bsudo\b`)

// BuildBatchCommand creates a single shell command string that executes all commands
// in sequence, with headers for logging. sudo commands are automatically converted
// to use password piping (echo 'pass' | sudo -S ...).
func BuildBatchCommand(commands []string, password, secret string) string {
	var sb strings.Builder
	sudoPass := password
	if secret != "" {
		sudoPass = secret
	}

	for _, cmd := range commands {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" || strings.HasPrefix(cmd, "#") {
			continue
		}

		// Escape command for logging
		safeCmdLog := strings.ReplaceAll(cmd, "\"", "\\\"")

		// Auto-convert sudo commands to pipe password via stdin
		execCmd := cmd
		if sudoPass != "" && sudoRe.MatchString(cmd) {
			safePass := strings.ReplaceAll(sudoPass, "'", "'\\''")
			replacement := fmt.Sprintf("echo '%s' | sudo -S -p '' ", safePass)
			execCmd = sudoRe.ReplaceAllString(cmd, replacement)
		}

		// Write header and command
		fmt.Fprintf(&sb, "echo \"\"; echo \"----------------------------------------\"; echo \"[EXEC] %s\"; echo \"----------------------------------------\"; %s; ", safeCmdLog, execCmd)
	}
	return sb.String()
}

// RunCommand executes a command via SSH without PTY (direct exec).
// Used for Linux servers where clean output without control characters is desired.
func RunCommand(host, user, cmd string, stdout, stderr io.Writer, password, secret string, timeout time.Duration, debug bool) bool {
	destination := host
	if user != "" {
		destination = user + "@" + host
	}

	// Build SSH command with TERM=dumb to suppress color codes
	c := exec.Command("ssh", destination, cmd)
	c.Env = append(os.Environ(), "TERM=dumb")
	c.Stdout = stdout
	c.Stderr = stderr

	if debug {
		log.Printf("[%s] Executing Raw SSH...", host)
	}

	if err := c.Start(); err != nil {
		log.Printf("[%s] [ERROR] Exec failed: %v", host, err)
		return false
	}

	done := make(chan error, 1)
	go func() {
		done <- c.Wait()
	}()

	select {
	case <-time.After(timeout):
		if c.Process != nil {
			_ = c.Process.Kill()
		}
		log.Printf("[%s] [ERROR] Command timed out.", host)
		return false
	case err := <-done:
		if err != nil {
			if debug {
				log.Printf("[%s] Command returned error: %v", host, err)
			}
			// Exit code 255 typically indicates an SSH connection error
			if exitErr, ok := err.(*exec.ExitError); ok {
				return exitErr.ExitCode() != 255
			}
			return false
		}
		return true
	}
}
