package executor

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/cotta-dev/retri/internal/config"
	"github.com/cotta-dev/retri/internal/logger"
)

// HostTaskOptions contains CLI overrides and runtime credential fallbacks for
// one automated host execution.
type HostTaskOptions struct {
	Command          string
	CommandFile      string
	Password         string
	Secret           string
	LogDir           string
	Suffix           string
	FilenameFormat   string
	TimestampFormat  string
	LogEncoding      string
	ExitCommand      string
	FallbackPassword string
	FallbackSecret   string
	NoTimestamp      bool
	Debug            bool
}

// ExecuteHostTask runs the full command execution workflow for a single host.
// Credential fallbacks apply only when the resolved config/env/CLI value is empty.
func ExecuteHostTask(rh config.ResolvedHost, defaults config.GlobalOptions, options HostTaskOptions) {
	// 1. Resolve settings through the priority chain
	user, password, secret, logDir, suffix, filenameFormat, timestampFormat, promptTimeout :=
		config.ResolveSettings(rh, defaults, options.Password, options.Secret, options.LogDir, options.Suffix, options.FilenameFormat, options.TimestampFormat)
	logEncoding := config.ResolveLogEncoding(rh, defaults, options.LogEncoding)

	// Apply fallbacks only for hosts that have no password/secret configured.
	if password == "" && options.FallbackPassword != "" {
		password = options.FallbackPassword
	}
	if secret == "" && options.FallbackSecret != "" {
		secret = options.FallbackSecret
	}

	// 2. Collect commands from all layers
	allCommands := CollectCommands(rh, defaults, options.CommandFile, options.Command)

	if rh.HostConfig.Host == "" || len(allCommands) == 0 {
		log.Printf("[%s] Skip: Missing host or commands", rh.HostConfig.Host)
		return
	}

	if options.Debug {
		log.Printf("[%s] Device: %s, User: %s, Timeout: %v, Log encoding: %s", rh.HostConfig.Host, rh.DeviceType, user, promptTimeout, logEncoding)
	}

	// 3. Set up logger
	lg, logFile, logPath, err := logger.Setup(rh.HostConfig.Host, logger.SetupOptions{
		Directory:        logDir,
		FilenameFormat:   filenameFormat,
		TimestampFormat:  timestampFormat,
		Suffix:           suffix,
		Encoding:         logEncoding,
		NoTimestamp:      options.NoTimestamp,
		DefaultTimestamp: defaults.Timestamp,
	})
	if err != nil {
		log.Printf("[%s] [ERROR] Failed to setup logger: %v", rh.HostConfig.Host, err)
		return
	}
	logFinalized := false
	defer func() {
		if !logFinalized {
			if err := logger.Finalize(lg, logFile); err != nil {
				log.Printf("[%s] [ERROR] Failed to finalize log: %v", rh.HostConfig.Host, err)
			}
		}
	}()

	// Write log header (no timestamp for header block)
	header := fmt.Sprintf("%s\n TARGET HOST : %s\n DEVICE TYPE : %s\n START TIME  : %s\n%s\n",
		strings.Repeat("=", 60), rh.HostConfig.Host, rh.DeviceType, time.Now().Format("2006-01-02 15:04:05"), strings.Repeat("=", 60))
	lg.WriteRaw(header)
	if err := lg.Err(); err != nil {
		log.Printf("[%s] [ERROR] Failed to write log header: %v", rh.HostConfig.Host, err)
		return
	}

	log.Printf("[%s] Executing %d commands...", rh.HostConfig.Host, len(allCommands))

	// 4. Execute via SSH
	// Build a private per-host snapshot. DeviceConfig is shared by hosts with
	// the same device type, so its setup slice must never be appended to in
	// place during parallel execution.
	// Automated Linux commands must not be appended to the remote shell's
	// persistent history. Keep this out of network-device sessions, where an
	// unknown shell command could alter CLI behavior.
	disableShellHistory := rh.DeviceType == config.DefaultDeviceType
	fullCmdList := buildExecutionCommands(rh.DeviceConfig.SetupCommands, allCommands, disableShellHistory)

	// Use a real PTY for Linux and network devices alike. This keeps the remote
	// shell/CLI prompt, its actual command echo, and output in one terminal
	// transcript instead of manufacturing a Linux prompt around direct-SSH
	// output.
	promptRegex := automatedPromptRegex(rh)
	exitCommand := rh.DeviceConfig.ExitCommand
	if options.ExitCommand != "" {
		exitCommand = options.ExitCommand
	}
	executionSucceeded := RunInteractive(rh.HostConfig.Host, user, fullCmdList, lg, lg, password, secret, promptRegex, exitCommand, promptTimeout, options.Debug)
	if !executionSucceeded {
		lg.ProcessAndWriteLine([]byte("[ERROR] Interactive execution failed."))
	}

	// Log footer (no timestamp for footer block)
	footer := fmt.Sprintf("\n%s\n LOG END     : %s\n%s\n",
		strings.Repeat("=", 60), time.Now().Format("2006-01-02 15:04:05"), strings.Repeat("=", 60))
	lg.WriteRaw(footer)
	if err := lg.Err(); err != nil {
		log.Printf("[%s] [ERROR] Failed to write log: %v", rh.HostConfig.Host, err)
		return
	}
	closeErr := logger.Finalize(lg, logFile)
	logFinalized = true
	if closeErr != nil {
		log.Printf("[%s] [ERROR] Failed to finalize log: %v", rh.HostConfig.Host, closeErr)
		return
	}

	if executionSucceeded {
		log.Printf("[%s] Completed. Log saved: %s", rh.HostConfig.Host, logPath)
	} else {
		log.Printf("[%s] [FAILURE] Execution failed. Partial log saved: %s", rh.HostConfig.Host, logPath)
	}
}

func automatedPromptRegex(rh config.ResolvedHost) string {
	if rh.DeviceConfig.PromptRegex != "" {
		return rh.DeviceConfig.PromptRegex
	}
	if rh.DeviceType == config.DefaultDeviceType {
		return config.DefaultShellPromptRegex
	}
	return config.DefaultPromptRegex
}
