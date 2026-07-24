package executor

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/cotta-dev/retri/internal/config"
	"github.com/cotta-dev/retri/internal/logger"
	"github.com/creack/pty"
)

// RunInteractive executes commands through a real PTY-backed SSH session.
// Linux shells and network CLIs both use a "wait for prompt -> send command"
// interaction pattern so the log can retain the terminal prompt, command echo,
// and output as one chronological transcript.
func RunInteractive(host, user string, commands []string, tw *logger.LineLogger, w io.Writer, password, secret, promptRegex, exitCommand string, timeout time.Duration, debug bool) bool {
	destination := host
	if user != "" {
		destination = user + "@" + host
	}

	// Force TTY allocation with -t
	c := exec.Command("ssh", "-t", "--", destination)
	c.Env = append(os.Environ(), "TERM=dumb")

	// Start PTY (pseudo-terminal)
	ptmx, err := pty.StartWithSize(c, &pty.Winsize{
		Rows: config.PTYRows,
		Cols: config.PTYCols,
		X:    0,
		Y:    0,
	})
	if err != nil {
		log.Printf("[%s] [ERROR] Failed to start PTY: %v", host, err)
		return false
	}
	defer func() { _ = ptmx.Close() }()

	// Channels for coordination between goroutines
	promptCh := make(chan struct{}, 10)  // Notifies when a prompt is detected
	expectEchoCh := make(chan string, 1) // Used to ignore command echo-back
	doneCh := make(chan error, 1)        // Reports log writer failure or read-loop completion
	sessionDone := make(chan struct{})   // Cancels command waits when SSH exits

	// Goroutine: monitor terminal output, handle prompts and password requests
	go handlePrompts(ptmx, w, password, secret, promptRegex, promptCh, expectEchoCh, doneCh, debug)

	// Goroutine: send commands sequentially
	commandResultCh := make(chan interactiveCommandResult, 1)
	go func() {
		var result interactiveCommandResult
		promptReady := false
		defer func() {
			commandResultCh <- result
			close(commandResultCh)
		}()
		fail := func(err error) {
			result.err = err
			if c.Process != nil {
				_ = c.Process.Kill()
			}
		}

		// Wait for initial prompt
		select {
		case <-promptCh:
			promptReady = true
		case <-time.After(config.InteractiveInitialWait):
			// Try pressing Enter if prompt doesn't appear
			if err := writeInteractiveInput(ptmx, "\n"); err != nil {
				fail(err)
				return
			}
		case <-sessionDone:
			return
		}

		for _, cmd := range commands {
			cmd = strings.TrimSpace(cmd)
			if cmd == "" || strings.HasPrefix(cmd, "#") {
				continue
			}

			// Drain any accumulated prompt notifications
			for len(promptCh) > 0 {
				<-promptCh
				promptReady = true
			}

			// Register the command for echo-back detection
			select {
			case expectEchoCh <- cmd:
			default:
				select {
				case <-expectEchoCh:
				default:
				}
				expectEchoCh <- cmd
			}

			// Write command header to log
			// When prompt detection succeeds, preserve it for the command echo.
			// After a timeout, flush arbitrary unterminated output first.
			tw.LogHeader(cmd, promptReady)
			if err := tw.Err(); err != nil {
				fail(err)
				return
			}
			select {
			case <-time.After(config.InteractiveCommandDelay):
			case <-sessionDone:
				return
			}

			if debug {
				log.Printf("[%s] Sending: %s", host, cmd)
			}

			// Send command
			if err := writeInteractiveInput(ptmx, cmd+"\n"); err != nil {
				fail(err)
				return
			}

			// Wait for next prompt after command
			select {
			case <-promptCh:
				promptReady = true
			case <-time.After(timeout):
				promptReady = false
				log.Printf("[%s] [WARN] Timeout waiting for prompt after: %s", host, cmd)
			case <-sessionDone:
				return
			}
		}

		// Send exit after all commands
		if exitCommand == "" {
			exitCommand = "exit"
		}
		select {
		case <-time.After(config.InteractiveExitDelay):
		case <-sessionDone:
			return
		}
		if err := writeInteractiveInput(ptmx, exitCommand+"\n"); err != nil {
			fail(err)
			return
		}
		result.exitSent = true
	}()

	// Wait for both the SSH process and read loop. A log failure terminates the
	// process so an unread PTY cannot block forever after the writer stops.
	waitCh := make(chan error, 1)
	go func() { waitCh <- c.Wait() }()
	var readErr, waitErr error
	select {
	case readErr = <-doneCh:
		close(sessionDone)
		if readErr != nil && c.Process != nil {
			_ = c.Process.Kill()
		}
		waitErr = <-waitCh
	case waitErr = <-waitCh:
		close(sessionDone)
		readErr = <-doneCh
	}
	if readErr != nil {
		log.Printf("[%s] [ERROR] %v", host, readErr)
		return false
	}
	commandResult := <-commandResultCh
	if commandResult.err != nil {
		log.Printf("[%s] [ERROR] Interactive command failed: %v", host, commandResult.err)
		return false
	}
	if err := validateInteractiveExit(waitErr, commandResult.exitSent); err != nil {
		log.Printf("[%s] [ERROR] SSH session failed: %v", host, err)
		return false
	}
	if err := tw.Err(); err != nil {
		log.Printf("[%s] [ERROR] Failed to write session log: %v", host, err)
		return false
	}

	return true
}

type interactiveCommandResult struct {
	err      error
	exitSent bool
}

// validateInteractiveExit separates SSH transport failure (255) and signals
// from a remote shell's status. "exit" without an explicit status inherits the
// last command's status, so a clean logout can legitimately return 1..254.
func validateInteractiveExit(waitErr error, exitSent bool) error {
	if !exitSent {
		if waitErr != nil {
			return fmt.Errorf("session ended before the exit command completed: %w", waitErr)
		}
		return errors.New("session ended before the exit command completed")
	}
	if waitErr == nil {
		return nil
	}

	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		exitCode := exitErr.ExitCode()
		if exitCode >= 0 && exitCode != 255 {
			return nil
		}
	}
	return waitErr
}

// handlePrompts monitors terminal output and handles:
// 1. Writing output to the log
// 2. Automatic password/sudo prompt responses
// 3. Command prompt detection (e.g., "#") -> notification via promptCh
func handlePrompts(r io.Reader, w io.Writer, password, secret, promptRegex string, promptCh chan struct{}, expectEchoCh chan string, doneCh chan<- error, debug bool) {
	defer close(doneCh)

	var re *regexp.Regexp
	if promptRegex != "" {
		var err error
		re, err = regexp.Compile(promptRegex)
		if err != nil {
			log.Printf("[ERROR] Invalid prompt regex '%s': %v", promptRegex, err)
			doneCh <- fmt.Errorf("invalid prompt regex %q: %w", promptRegex, err)
			return
		}
	}

	// Password prompt detection (case insensitive)
	// Matches lines ending with "password:" or "password for <user>:"
	pwdRe := regexp.MustCompile(`(?i)(password|passphrase)(?: for .*)?[:\?]\s*$`)

	buf := make([]byte, config.ReadBufferSize)
	var lineBuffer []byte
	var expectedEcho string

	for {
		n, err := r.Read(buf)
		if n > 0 {
			data := buf[:n]

			// Write output to log
			if _, writeErr := w.Write(data); writeErr != nil {
				doneCh <- fmt.Errorf("write session log: %w", writeErr)
				return
			}

			if debug {
				_, _ = os.Stdout.Write(data)
			}

			// Check for expected echo-back command
			select {
			case cmd := <-expectEchoCh:
				expectedEcho = strings.TrimSpace(cmd)
			default:
			}

			// Buffer data for analysis
			lineBuffer = append(lineBuffer, data...)
			if len(lineBuffer) > config.LineBufferMaxSize {
				lineBuffer = lineBuffer[len(lineBuffer)-config.LineBufferMaxSize:]
			}

			str := string(lineBuffer)
			cleanStr := logger.StripAnsi(str)

			// Get the last line (prompt area)
			lines := strings.Split(cleanStr, "\n")
			var lastLine string
			if len(lines) > 0 {
				lastLine = lines[len(lines)-1]
			}
			lastLineTrimmed := strings.TrimSpace(lastLine)

			// Password prompt detection
			if pwdRe.MatchString(lastLineTrimmed) {
				toSend := password
				// Detect sudo password prompts
				if strings.Contains(strings.ToLower(lastLineTrimmed), "sudo") || strings.Contains(strings.ToLower(lastLineTrimmed), "password for") {
					if secret != "" {
						toSend = secret
					}
				}
				if rw, ok := r.(io.Writer); ok && toSend != "" {
					lineBuffer = nil
					if err := writeInteractiveInput(rw, toSend+"\n"); err != nil {
						doneCh <- fmt.Errorf("send password response: %w", err)
						return
					}
					if debug {
						log.Printf("[DEBUG] Password sent triggered by: %s", lastLineTrimmed)
					}
				}
			} else if strings.Contains(strings.ToLower(lastLineTrimmed), "are you sure you want to continue") {
				// SSH fingerprint confirmation
				if rw, ok := r.(io.Writer); ok {
					lineBuffer = nil
					if err := writeInteractiveInput(rw, "yes\n"); err != nil {
						doneCh <- fmt.Errorf("send host-key response: %w", err)
						return
					}
				}
			}

			// Echo-back detection: skip prompt detection until echo is consumed
			if expectedEcho != "" {
				if strings.Contains(cleanStr, expectedEcho) {
					expectedEcho = ""
				} else {
					continue
				}
			}

			// Command completion prompt detection
			if re != nil && promptCh != nil {
				trimmed := strings.TrimRight(cleanStr, " \t\r\n")
				lines := strings.Split(trimmed, "\n")
				if len(lines) > 0 {
					lastLine := lines[len(lines)-1]
					if re.MatchString(lastLine) {
						select {
						case promptCh <- struct{}{}:
							lineBuffer = nil
						default:
						}
					}
				}
			}
		}
		if err != nil {
			break
		}
	}
	doneCh <- nil
}

func writeInteractiveInput(w io.Writer, input string) error {
	n, err := io.WriteString(w, input)
	if err == nil && n != len(input) {
		err = io.ErrShortWrite
	}
	return err
}
