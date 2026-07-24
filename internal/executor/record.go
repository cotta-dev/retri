package executor

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/cotta-dev/retri/internal/config"
	"github.com/cotta-dev/retri/internal/logger"
	"github.com/creack/pty"
	"golang.org/x/term"
)

// RunSSHRecordSession opens an interactive SSH session to host (as user, empty = current OS user)
// in a PTY and records all I/O to the logger. Returns when the SSH session exits.
func RunSSHRecordSession(host, user string, lg *logger.LineLogger, commandsOnly, debug bool) error {
	args := []string{"-t"}
	if user != "" {
		args = append(args, "-l", user)
	}
	args = append(args, "--", host)

	c := exec.Command("ssh", args...)
	c.Env = os.Environ()

	ptmx, err := pty.Start(c)
	if err != nil {
		return err
	}
	defer func() { _ = ptmx.Close() }()

	_ = pty.InheritSize(os.Stdin, ptmx)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	go func() {
		for range sigCh {
			_ = pty.InheritSize(os.Stdin, ptmx)
		}
	}()
	defer signal.Stop(sigCh)

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	recorder := newCommandOutputRecorder(lg, commandsOnly)
	go forwardRecordInput(ptmx, os.Stdin, recorder)

	buf := make([]byte, config.ReadBufferSize)
	var logErr error
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			logErr = writeRecordedOutput(lg, recorder, buf[:n])
			_, _ = os.Stdout.Write(buf[:n])
			if logErr != nil {
				if c.Process != nil {
					_ = c.Process.Kill()
				}
				break
			}
		}
		if err != nil {
			break
		}
	}

	waitErr := c.Wait()
	if logErr != nil {
		return fmt.Errorf("write session log: %w", logErr)
	}
	if err := flushRecordedOutput(lg, recorder); err != nil {
		return fmt.Errorf("flush session log: %w", err)
	}
	if waitErr != nil {
		return fmt.Errorf("SSH session: %w", waitErr)
	}

	return nil
}

// RunRecordSession starts the user's shell in a PTY and records all output to the logger.
// It relays stdin/stdout so the user interacts normally while all I/O is captured.
func RunRecordSession(lg *logger.LineLogger, commandsOnly, debug bool) error {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	c := exec.Command(shell)
	// Set argv[0] to "-<shell>" to start as a login shell,
	// so that .bash_profile / .zprofile (and thus .bashrc / .zshrc) are sourced.
	c.Args[0] = "-" + filepath.Base(shell)
	c.Env = os.Environ()

	// Start PTY with current terminal size
	ptmx, err := pty.Start(c)
	if err != nil {
		return err
	}
	defer func() { _ = ptmx.Close() }()

	// Inherit terminal size
	_ = pty.InheritSize(os.Stdin, ptmx)

	// Handle SIGWINCH to propagate terminal resizes
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	go func() {
		for range sigCh {
			_ = pty.InheritSize(os.Stdin, ptmx)
		}
	}()
	defer signal.Stop(sigCh)

	// Set stdin to raw mode so keystrokes are passed through directly
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	defer func() { _ = term.Restore(int(os.Stdin.Fd()), oldState) }()

	recorder := newCommandOutputRecorder(lg, commandsOnly)

	// Forward stdin to PTY
	go forwardRecordInput(ptmx, os.Stdin, recorder)

	// Read PTY output -> display on stdout + write to log
	buf := make([]byte, config.ReadBufferSize)
	var logErr error
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			logErr = writeRecordedOutput(lg, recorder, buf[:n])
			_, _ = os.Stdout.Write(buf[:n])
			if logErr != nil {
				if c.Process != nil {
					_ = c.Process.Kill()
				}
				break
			}
		}
		if err != nil {
			break
		}
	}

	waitErr := c.Wait()
	if logErr != nil {
		return fmt.Errorf("write session log: %w", logErr)
	}
	if err := flushRecordedOutput(lg, recorder); err != nil {
		return fmt.Errorf("flush session log: %w", err)
	}
	if waitErr != nil {
		return fmt.Errorf("shell session: %w", waitErr)
	}

	return nil
}

func newCommandOutputRecorder(lg *logger.LineLogger, enabled bool) *CommandOutputRecorder {
	if !enabled {
		return nil
	}
	return NewCommandOutputRecorder(lg)
}

func forwardRecordInput(dst io.Writer, src io.Reader, recorder *CommandOutputRecorder) {
	buf := make([]byte, config.ReadBufferSize)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			input := buf[:n]
			if recorder != nil {
				recorder.ObserveInput(input)
			}
			if _, writeErr := dst.Write(input); writeErr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func writeRecordedOutput(lg *logger.LineLogger, recorder *CommandOutputRecorder, output []byte) error {
	if recorder != nil {
		recorder.ObserveOutput(output)
		return lg.Err()
	}
	_, err := lg.Write(output)
	return err
}

func flushRecordedOutput(lg *logger.LineLogger, recorder *CommandOutputRecorder) error {
	if recorder != nil {
		recorder.Flush()
	} else {
		lg.Flush()
	}
	return lg.Err()
}
