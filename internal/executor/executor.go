package executor

import (
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/cotta-dev/retri/internal/config"
	"github.com/cotta-dev/retri/internal/logger"
)

// ExecuteHostTask runs the full command execution workflow for a single host.
func ExecuteHostTask(rh config.ResolvedHost, defaults config.GlobalOptions, cliCommand, cliPassword, cliSecret, cliLogDir, cliSuffix, cliFilenameFormat, cliTimestampFormat, cliExitCommand string, noTimestamp, debug bool) {
	// 1. Resolve settings through the priority chain
	user, password, secret, logDir, suffix, filenameFormat, timestampFormat, promptTimeout :=
		config.ResolveSettings(rh, defaults, cliPassword, cliSecret, cliLogDir, cliSuffix, cliFilenameFormat, cliTimestampFormat)

	// 2. Collect commands from all layers
	allCommands := CollectCommands(rh, defaults, cliCommand)

	if rh.HostConfig.Host == "" || len(allCommands) == 0 {
		log.Printf("[%s] Skip: Missing host or commands", rh.HostConfig.Host)
		return
	}

	if debug {
		log.Printf("[%s] Device: %s, User: %s, Timeout: %v", rh.HostConfig.Host, rh.DeviceType, user, promptTimeout)
	}

	// 3. Set up logger
	lg, logFile, logPath, err := logger.SetupLogger(rh.HostConfig.Host, logDir, filenameFormat, timestampFormat, suffix, noTimestamp, defaults.Timestamp)
	if err != nil {
		log.Printf("[%s] [ERROR] Failed to setup logger: %v", rh.HostConfig.Host, err)
		return
	}
	defer func() { _ = logFile.Close() }()

	// Write log header (no timestamp for header block)
	header := fmt.Sprintf("%s\n TARGET HOST : %s\n DEVICE TYPE : %s\n START TIME  : %s\n%s\n",
		strings.Repeat("=", 60), rh.HostConfig.Host, rh.DeviceType, time.Now().Format("2006-01-02 15:04:05"), strings.Repeat("=", 60))
	lg.WriteRaw(header)

	log.Printf("[%s] Executing %d commands...", rh.HostConfig.Host, len(allCommands))

	// 4. Execute via SSH
	// Combine setup commands and main commands
	fullCmdList := append(rh.DeviceConfig.SetupCommands, allCommands...)

	if rh.DeviceType == "linux" {
		// Linux: non-interactive SSH exec for clean output
		// Connection check first
		if !RunCommand(rh.HostConfig.Host, user, "echo 'CONNECTION_CHECK_OK'", io.Discard, io.Discard, password, secret, config.ConnectionCheckTimeout, debug) {
			log.Printf("[%s] [FAILURE] SSH Connection failed.", rh.HostConfig.Host)
			return
		}

		// Build batch command script
		batchCmd := BuildBatchCommand(fullCmdList, password, secret)

		// Timeout: base + per-command addition
		batchTimeout := config.BaseBatchTimeout + time.Duration(len(fullCmdList))*config.PerCommandBatchTimeout
		success := RunCommand(rh.HostConfig.Host, user, batchCmd, lg, lg, password, secret, batchTimeout, debug)

		if !success {
			lg.ProcessAndWriteLine([]byte("[ERROR] Batch execution failed or timed out."))
		}
	} else {
		// Network devices: interactive SSH with PTY
		promptRegex := rh.DeviceConfig.PromptRegex
		if promptRegex == "" {
			promptRegex = config.DefaultPromptRegex
		}
		exitCommand := rh.DeviceConfig.ExitCommand
		if cliExitCommand != "" {
			exitCommand = cliExitCommand
		}
		RunInteractive(rh.HostConfig.Host, user, fullCmdList, lg, lg, password, secret, promptRegex, exitCommand, promptTimeout, debug)
	}

	// Log footer (no timestamp for footer block)
	footer := fmt.Sprintf("\n%s\n LOG END     : %s\n%s\n",
		strings.Repeat("=", 60), time.Now().Format("2006-01-02 15:04:05"), strings.Repeat("=", 60))
	lg.WriteRaw(footer)

	log.Printf("[%s] Completed. Log saved: %s", rh.HostConfig.Host, logPath)
}
