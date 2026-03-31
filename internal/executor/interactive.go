package executor

import (
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/creack/pty"
	"github.com/cotta-dev/retri/internal/config"
	"github.com/cotta-dev/retri/internal/logger"
)

// RunInteractive executes commands on a network device using PTY-based interactive SSH.
// Network devices (Cisco IOS, etc.) have their own CLI instead of a shell, requiring
// a "wait for prompt -> send command" interaction pattern.
func RunInteractive(host, user string, commands []string, tw *logger.LineLogger, w io.Writer, password, secret, promptRegex, exitCommand string, timeout time.Duration, debug bool) bool {
	destination := host
	if user != "" {
		destination = user + "@" + host
	}

	// Force TTY allocation with -t
	c := exec.Command("ssh", "-t", destination)
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
	doneCh := make(chan struct{})        // Signals read loop completion

	// Goroutine: monitor terminal output, handle prompts and password requests
	go handlePrompts(ptmx, w, password, secret, promptRegex, promptCh, expectEchoCh, doneCh, debug)

	// Goroutine: send commands sequentially
	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)

		// Wait for initial prompt
		select {
		case <-promptCh:
			// Prompt appeared
		case <-time.After(config.InteractiveInitialWait):
			// Try pressing Enter if prompt doesn't appear
			_, _ = ptmx.Write([]byte("\n"))
		}

		for _, cmd := range commands {
			cmd = strings.TrimSpace(cmd)
			if cmd == "" || strings.HasPrefix(cmd, "#") {
				continue
			}

			// Drain any accumulated prompt notifications
			for len(promptCh) > 0 {
				<-promptCh
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
			tw.LogHeader(cmd)
			time.Sleep(config.InteractiveCommandDelay)

			if debug {
				log.Printf("[%s] Sending: %s", host, cmd)
			}

			// Send command
			_, err := ptmx.Write([]byte(cmd + "\n"))
			if err != nil {
				errCh <- err
				return
			}

			// Wait for next prompt after command
			select {
			case <-promptCh:
				// Success
			case <-time.After(timeout):
				log.Printf("[%s] [WARN] Timeout waiting for prompt after: %s", host, cmd)
			}
		}

		// Send exit after all commands
		if exitCommand == "" {
			exitCommand = "exit"
		}
		time.Sleep(config.InteractiveExitDelay)
		_, _ = ptmx.Write([]byte(exitCommand + "\n"))
	}()

	// Wait for process to finish
	_ = c.Wait()
	<-doneCh // Wait for read loop to finish

	return true
}

// handlePrompts monitors terminal output and handles:
// 1. Writing output to the log
// 2. Automatic password/sudo prompt responses
// 3. Command prompt detection (e.g., "#") -> notification via promptCh
func handlePrompts(r io.Reader, w io.Writer, password, secret, promptRegex string, promptCh chan struct{}, expectEchoCh chan string, doneCh chan struct{}, debug bool) {
	defer close(doneCh)

	var re *regexp.Regexp
	if promptRegex != "" {
		var err error
		re, err = regexp.Compile(promptRegex)
		if err != nil {
			log.Printf("[ERROR] Invalid prompt regex '%s': %v", promptRegex, err)
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
			_, _ = w.Write(data)

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
					_, _ = rw.Write([]byte(toSend + "\n"))
					if debug {
						log.Printf("[DEBUG] Password sent triggered by: %s", lastLineTrimmed)
					}
				}
			} else if strings.Contains(strings.ToLower(lastLineTrimmed), "are you sure you want to continue") {
				// SSH fingerprint confirmation
				if rw, ok := r.(io.Writer); ok {
					_, _ = rw.Write([]byte("yes\n"))
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
}
