package executor

import (
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/creack/pty"
	"github.com/cotta-dev/retri/internal/config"
	"github.com/cotta-dev/retri/internal/logger"
	"golang.org/x/term"
)

// RunSSHRecordSession opens an interactive SSH session to host (as user, empty = current OS user)
// in a PTY and records all I/O to the logger. Returns when the SSH session exits.
func RunSSHRecordSession(host, user string, lg *logger.LineLogger, debug bool) error {
	args := []string{"-t"}
	if user != "" {
		args = append(args, "-l", user)
	}
	args = append(args, host)

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

	go func() {
		_, _ = io.Copy(ptmx, os.Stdin)
	}()

	buf := make([]byte, config.ReadBufferSize)
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			_, _ = os.Stdout.Write(buf[:n])
			_, _ = lg.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}

	_ = c.Wait()
	lg.Flush()

	return nil
}

// RunRecordSession starts the user's shell in a PTY and records all output to the logger.
// It relays stdin/stdout so the user interacts normally while all I/O is captured.
func RunRecordSession(lg *logger.LineLogger, debug bool) error {
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

	// Forward stdin to PTY
	go func() {
		_, _ = io.Copy(ptmx, os.Stdin)
	}()

	// Read PTY output -> display on stdout + write to log
	buf := make([]byte, config.ReadBufferSize)
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			_, _ = os.Stdout.Write(buf[:n])
			_, _ = lg.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}

	_ = c.Wait()
	lg.Flush()

	return nil
}
