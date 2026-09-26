package cli

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jessevdk/go-flags"

	"github.com/cotta-dev/retri/internal/config"
	"github.com/cotta-dev/retri/internal/credentials"
	"github.com/cotta-dev/retri/internal/executor"
	"github.com/cotta-dev/retri/internal/logencoding"
	"github.com/cotta-dev/retri/internal/logger"
	"github.com/cotta-dev/retri/internal/updater"
)

// Options defines the CLI flags.
type Options struct {
	ConfigFile string `short:"c" long:"config" description:"Config file path (default: ~/.config/retri/config.yaml)"`

	// Target selection
	Host  string `short:"H" long:"host" description:"Target single host"`
	Group string `short:"g" long:"group" description:"Target group"`

	// Command specification
	CommandFile string `short:"f" long:"command-file" description:"Command file path"`
	Command     string `long:"command" description:"Single command to execute"`

	// Log settings
	LogDir          string `short:"d" long:"log-dir" description:"Log directory override (default: ~/retri-logs)"`
	FilenameFormat  string `short:"F" long:"filename-format" description:"Log filename format override (default: {host}_{timestamp}{suffix}.log)"`
	TimestampFormat string `short:"t" long:"timestamp-format" description:"Timestamp format override (default: YYYYMMDD_HHmmss)"`
	Suffix          string `short:"S" long:"suffix" description:"Filename suffix override"`
	LogEncoding     string `long:"log-encoding" description:"Terminal output encoding (default: raw)"`
	LogCommandsOnly bool   `long:"log-commands-only" description:"Record only submitted commands and their output in session mode"`

	// Execution control
	Parallel    int  `short:"P" long:"parallel" description:"Parallel execution count (default: 5 or config 'auto')"`
	Debug       bool `short:"D" long:"debug" description:"Enable debug output"`
	NoTimestamp bool `short:"T" long:"no-timestamp" description:"Disable timestamp logging"`

	// Authentication (also available via RETRI_SSH_PASSWORD / RETRI_SSH_SECRET)
	Password               string `short:"p" long:"password" description:"SSH password literal override; command-line values may be visible to other processes"`
	Secret                 string `short:"s" long:"secret" description:"Sudo/enable secret literal override; command-line values may be visible to other processes"`
	ExitCommand            string `short:"e" long:"exit-command" description:"Exit command for interactive sessions (default: exit)"`
	CredentialCacheRefresh bool   `long:"credential-cache-refresh" description:"Discard selected credential caches and obtain fresh values from their configured sources"`
	CredentialCacheClear   bool   `long:"credential-cache-clear" description:"Clear this config's current credential cache entries and exit"`

	// Misc
	Completion string `long:"completion" choice:"bash" choice:"zsh" choice:"fish" description:"Generate shell completion script (bash, zsh, or fish)"`
	ConfigHelp bool   `short:"C" long:"config-help" description:"Show config file documentation"`
	Version    bool   `short:"v" long:"version" description:"Show version information"`
	Update     bool   `short:"u" long:"update" description:"Update retri to the latest version"`
}

// Run is the main entry point for the application.
// version is the build version string. defaultConfigContent and helpContent are embedded resources.
func Run(version string, defaultConfigContent []byte, helpContent string) {
	log.SetFlags(0)

	if isCompletionRequest() {
		runCompletion(os.Args[1:])
		os.Exit(0)
	}

	// 1. Parse CLI arguments
	var opts Options
	parser := flags.NewParser(&opts, flags.Default)
	parser.Name = config.AppName
	parser.Usage = "[OPTIONS] [hostname]\n\n" +
		"  retri                  Start local work session recording\n" +
		"  retri <hostname>       SSH to host and record the session\n" +
		"  retri [OPTIONS]        Execute commands and collect logs\n\n" +
		"  Note: <hostname> is ignored when -H, -g, --command, or -f is specified."

	remaining, err := parser.Parse()
	if err != nil {
		if flags.WroteHelp(err) {
			os.Exit(0)
		}
		os.Exit(1)
	}

	if opts.Version {
		fmt.Printf("%s version %s\n", config.AppName, version)
		os.Exit(0)
	}

	if opts.Update {
		if err := updater.Run(version); err != nil {
			log.Fatalf("[ERROR] Update failed: %v", err)
		}
		os.Exit(0)
	}

	if opts.ConfigHelp {
		fmt.Println(helpContent)
		os.Exit(0)
	}

	if opts.Completion != "" {
		if err := writeCompletionScript(os.Stdout, opts.Completion); err != nil {
			log.Fatalf("[ERROR] Failed to generate completion script: %v", err)
		}
		os.Exit(0)
	}

	// 2. Prepare config file paths
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("[ERROR] Failed to get home directory: %v", err)
	}
	configDir := filepath.Join(homeDir, ".config", config.AppName)
	defaultConfigPath := filepath.Join(configDir, "config.yaml")
	commandsDir := filepath.Join(configDir, config.CommandsDirName)

	// Create default config if it doesn't exist
	if _, err := os.Stat(defaultConfigPath); os.IsNotExist(err) {
		if err := config.CreateDefault(defaultConfigPath, defaultConfigContent); err != nil {
			log.Printf("[WARNING] Failed to create default config: %v", err)
		} else {
			fmt.Printf("\n[INFO] Initial setup complete.\n[INFO] Config created at: %s\n", defaultConfigPath)
		}
	}
	// Create commands directory
	if _, err := os.Stat(commandsDir); os.IsNotExist(err) {
		if err := os.MkdirAll(commandsDir, 0700); err != nil {
			log.Printf("[WARNING] Failed to create commands directory: %v", err)
		}
	}

	// 3. Load config
	cfg := config.Load(opts.ConfigFile, defaultConfigPath)

	// 4. Validate config
	if err := cfg.Validate(); err != nil {
		// Validation walks credential fields. Do not pass that error to the
		// process logger: CodeQL and future validators must not turn a config
		// value into a clear-text log record. Detailed diagnostics remain
		// available to callers of config.Validate and tests.
		log.Fatal("[ERROR] Config validation failed; review the configuration")
	}
	if _, err := logencoding.Lookup(opts.LogEncoding); err != nil {
		log.Fatalf("[ERROR] Invalid --log-encoding: %v", err)
	}
	if opts.CredentialCacheClear {
		r := credentials.NewResolver(cfg.Credentials)
		r.Configure(credentialNamespace(opts.ConfigFile), false, credentials.ChildEnvironment(cfg, os.Environ(), true))
		err := r.Clear()
		r.Close()
		if err != nil {
			log.Fatalf("[ERROR] Failed to clear credential cache: %v", err)
		}
		fmt.Println("Credential cache cleared for current definitions.")
		return
	}
	childEnv := credentials.ChildEnvironment(cfg, os.Environ(), false)

	// 5. Check for a newer release once per day when enabled.
	updateCheckEnabled := cfg.Defaults.UpdateCheck == nil || *cfg.Defaults.UpdateCheck
	updater.MaybeNotify(version, updateCheckEnabled)

	// 6. Expand host ranges (e.g., switch-[01-05] -> switch-01, switch-02...)
	config.ExpandHostsInConfig(&cfg)

	// 7. Record mode: no target or command specified → start session recording
	if opts.Host == "" && opts.Group == "" && opts.Command == "" && opts.CommandFile == "" {
		if len(remaining) == 1 {
			// retri <hostname> → SSH to host and record session
			runSSHRecordMode(opts, remaining[0], cfg.Defaults, childEnv)
		} else {
			// retri (no args) → record local shell session
			runRecordMode(opts, cfg.Defaults, childEnv)
		}
		return
	}

	// 8. Resolve targets
	targets := config.ResolveTargets(cfg, opts.Host, opts.Group)
	if len(targets) == 0 {
		if opts.Host == "" && opts.Group == "" {
			fmt.Println("------------------------------------------------------------")
			parser.WriteHelp(os.Stdout)
			os.Exit(0)
		}
		log.Fatalf("[ERROR] No targets specified.")
	}

	// 9. Determine parallel count
	// Plan commands before acquiring secrets; skipped hosts need no providers.
	var runnable []config.ResolvedHost
	var plannedCommands [][]string
	for _, target := range targets {
		commands := executor.CollectCommands(target, cfg.Defaults, opts.CommandFile, opts.Command)
		if target.HostConfig.Host == "" || len(commands) == 0 {
			log.Printf("[%s] Skip: Missing host or commands", target.HostConfig.Host)
			continue
		}
		runnable = append(runnable, target)
		plannedCommands = append(plannedCommands, commands)
	}
	targets = runnable
	if len(targets) == 0 {
		return
	}
	parallelCount := config.DetermineParallelCount(cfg.Defaults.Parallel, opts.Parallel)

	// 9a. Resolve named credentials before starting parallel host work. Shared
	// prompt providers are therefore requested once per retri invocation.
	resolvedCredentials, err := resolveTargetCredentials(targets, cfg, opts)
	if err != nil {
		log.Fatalf("[ERROR] Failed to resolve credentials: %v", err)
	}
	defer clear(resolvedCredentials)

	// 10. Main execution loop (parallel)
	log.Printf("Starting tasks for %d hosts (Parallel: %d)...", len(targets), parallelCount)

	var wg sync.WaitGroup
	sem := make(chan struct{}, parallelCount)

	for i, target := range targets {
		wg.Add(1)
		auth := resolvedCredentials[i]
		go func(rh config.ResolvedHost, auth resolvedHostCredentials) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			executor.ExecuteHostTask(rh, cfg.Defaults, executor.HostTaskOptions{
				Commands:        plannedCommands[i],
				Command:         opts.Command,
				CommandFile:     opts.CommandFile,
				Password:        auth.Password,
				Secret:          auth.Secret,
				Environment:     childEnv,
				LogDir:          opts.LogDir,
				Suffix:          opts.Suffix,
				FilenameFormat:  opts.FilenameFormat,
				TimestampFormat: opts.TimestampFormat,
				LogEncoding:     opts.LogEncoding,
				ExitCommand:     opts.ExitCommand,
				NoTimestamp:     opts.NoTimestamp,
				Debug:           opts.Debug,
			})
		}(target, auth)
	}

	wg.Wait()
	log.Println("All tasks finished.")
}

// runSSHRecordMode SSHes to host and records the interactive session to a log file.
func runSSHRecordMode(opts Options, host string, defaults config.GlobalOptions, env []string) {
	lg, logFile, logPath, err := logger.Setup(host, sessionLogOptions(opts, defaults))
	if err != nil {
		log.Fatalf("[ERROR] Failed to setup logger: %v", err)
	}
	logFinalized := false
	defer func() {
		if !logFinalized {
			if err := logger.Finalize(lg, logFile); err != nil {
				log.Printf("[ERROR] Failed to finalize session log: %v", err)
			}
		}
	}()

	header := fmt.Sprintf("%s\n SESSION LOG : %s\n START TIME  : %s\n%s\n",
		strings.Repeat("=", 60), host, time.Now().Format("2006-01-02 15:04:05"), strings.Repeat("=", 60))
	lg.WriteRaw(header)
	if err := lg.Err(); err != nil {
		log.Printf("[ERROR] Failed to write session log: %v", err)
		return
	}

	log.Printf("SSH to %s — recording session to: %s", host, logPath)

	commandsOnly := shouldLogCommandsOnly(opts, defaults)
	if commandsOnly {
		log.Printf("Command/output-only logging enabled.")
	}
	if err := executor.RunSSHRecordSession(host, "", lg, commandsOnly, opts.Debug, env); err != nil {
		log.Printf("[ERROR] SSH session error: %v", err)
	}

	footer := fmt.Sprintf("\n%s\n LOG END     : %s\n%s\n",
		strings.Repeat("=", 60), time.Now().Format("2006-01-02 15:04:05"), strings.Repeat("=", 60))
	if err := finishSessionLog(lg, logFile, footer); err != nil {
		logFinalized = true
		log.Printf("[ERROR] Failed to finalize session log: %v", err)
		return
	}
	logFinalized = true

	log.Printf("Session log saved: %s", logPath)
}

// runRecordMode starts a local shell session recording.
func runRecordMode(opts Options, defaults config.GlobalOptions, env []string) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "localhost"
	}

	lg, logFile, logPath, err := logger.Setup(hostname, sessionLogOptions(opts, defaults))
	if err != nil {
		log.Fatalf("[ERROR] Failed to setup logger: %v", err)
	}
	logFinalized := false
	defer func() {
		if !logFinalized {
			if err := logger.Finalize(lg, logFile); err != nil {
				log.Printf("[ERROR] Failed to finalize session log: %v", err)
			}
		}
	}()

	// Write session header
	header := fmt.Sprintf("%s\n SESSION LOG : %s\n START TIME  : %s\n%s\n",
		strings.Repeat("=", 60), hostname, time.Now().Format("2006-01-02 15:04:05"), strings.Repeat("=", 60))
	lg.WriteRaw(header)
	if err := lg.Err(); err != nil {
		log.Printf("[ERROR] Failed to write session log: %v", err)
		return
	}

	log.Printf("Recording session to: %s", logPath)
	log.Printf("Start your work. Type 'exit' or press Ctrl-D to end recording.")

	commandsOnly := shouldLogCommandsOnly(opts, defaults)
	if commandsOnly {
		log.Printf("Command/output-only logging enabled.")
	}
	if err := executor.RunRecordSession(lg, commandsOnly, opts.Debug, env); err != nil {
		log.Printf("[ERROR] Record session error: %v", err)
	}

	// Write session footer
	footer := fmt.Sprintf("\n%s\n LOG END     : %s\n%s\n",
		strings.Repeat("=", 60), time.Now().Format("2006-01-02 15:04:05"), strings.Repeat("=", 60))
	if err := finishSessionLog(lg, logFile, footer); err != nil {
		logFinalized = true
		log.Printf("[ERROR] Failed to finalize session log: %v", err)
		return
	}
	logFinalized = true

	log.Printf("Session log saved: %s", logPath)
}

func shouldLogCommandsOnly(opts Options, defaults config.GlobalOptions) bool {
	return opts.LogCommandsOnly || (defaults.LogCommandsOnly != nil && *defaults.LogCommandsOnly)
}

func sessionLogOptions(opts Options, defaults config.GlobalOptions) logger.SetupOptions {
	return logger.SetupOptions{
		Directory:        firstNonEmpty(opts.LogDir, defaults.LogDir),
		FilenameFormat:   firstNonEmpty(opts.FilenameFormat, defaults.FilenameFormat),
		TimestampFormat:  firstNonEmpty(opts.TimestampFormat, defaults.TimestampFormat),
		Suffix:           firstNonEmpty(opts.Suffix, defaults.Suffix),
		Encoding:         config.ResolveLogEncoding(config.ResolvedHost{}, defaults, opts.LogEncoding),
		NoTimestamp:      opts.NoTimestamp,
		DefaultTimestamp: defaults.Timestamp,
	}
}

func firstNonEmpty(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func finishSessionLog(lg *logger.LineLogger, file *os.File, footer string) error {
	lg.WriteRaw(footer)
	return logger.Finalize(lg, file)
}
