package config

import "time"

const (
	// AppName is the application name.
	AppName = "retri"

	// CommandsDirName is the subdirectory name for command files.
	CommandsDirName = "commands"

	// DefaultParallel is the default number of parallel executions.
	DefaultParallel = 5

	// DefaultPromptTimeout is the default timeout for interactive prompt responses.
	DefaultPromptTimeout = 300 * time.Second

	// DefaultLogDir is the default log directory path.
	DefaultLogDir = "~/retri-logs"

	// DefaultFilenameFormat is the default log filename format.
	DefaultFilenameFormat = "{host}_{timestamp}{suffix}.log"

	// DefaultTimestampFormat is the default timestamp format for log filenames.
	DefaultTimestampFormat = "YYYYMMDD_HHmmss"

	// DefaultPromptRegex is the default regex for detecting network device prompts.
	DefaultPromptRegex = `[#>] ?$`

	// DefaultDeviceType is the default device type.
	DefaultDeviceType = "linux"

	// PTYRows is the default PTY terminal row count.
	PTYRows = 80

	// PTYCols is the default PTY terminal column count.
	PTYCols = 200

	// ConnectionCheckTimeout is the timeout for SSH connection checks.
	ConnectionCheckTimeout = 15 * time.Second

	// ReadBufferSize is the buffer size for reading PTY output.
	ReadBufferSize = 1024

	// LineBufferMaxSize is the maximum line buffer size for prompt detection.
	LineBufferMaxSize = 4096

	// InteractiveInitialWait is the wait time for the initial prompt in interactive mode.
	InteractiveInitialWait = 15 * time.Second

	// InteractiveCommandDelay is the delay between command header and sending the command.
	InteractiveCommandDelay = 100 * time.Millisecond

	// InteractiveExitDelay is the delay before sending exit after all commands finish.
	InteractiveExitDelay = 500 * time.Millisecond

	// BaseBatchTimeout is the base timeout for batch Linux command execution.
	BaseBatchTimeout = 600 * time.Second

	// PerCommandBatchTimeout is the additional timeout per command for batch execution.
	PerCommandBatchTimeout = 180 * time.Second
)
