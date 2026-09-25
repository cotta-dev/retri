package credentials

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

const providerTimeout = 30 * time.Second
const maxProviderOutput = 1024 * 1024

type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	if len(p) > maxProviderOutput-b.Len() {
		return 0, fmt.Errorf("provider output exceeds limit")
	}
	return b.Buffer.Write(p)
}

func runCommand(name string, args []string, stdin []byte) ([]byte, error) {
	return commandRunner(os.Environ())(name, args, stdin)
}

func commandRunner(env []string) commandFunc {
	return commandRunnerWithTimeout(env, providerTimeout)
}

func commandRunnerWithTimeout(env []string, timeout time.Duration) commandFunc {
	return func(name string, args []string, stdin []byte) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Env = env
		cmd.WaitDelay = time.Second
		if stdin != nil {
			cmd.Stdin = bytes.NewReader(stdin)
		}
		var output boundedOutput
		cmd.Stdout = &output
		// Discard stderr: provider diagnostics may contain secret material.
		if err := cmd.Run(); err != nil {
			clear(output.Bytes())
			if ctx.Err() != nil {
				return nil, fmt.Errorf("provider timed out")
			}
			return nil, fmt.Errorf("provider command failed: %w", err)
		}
		return output.Bytes(), nil
	}
}
