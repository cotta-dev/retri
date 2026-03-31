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
	"golang.org/x/term"

	"github.com/cotta-dev/retri/internal/config"
	"github.com/cotta-dev/retri/internal/executor"
	"github.com/cotta-dev/retri/internal/logger"
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

	// Execution control
	Parallel    int  `short:"P" long:"parallel" description:"Parallel execution count (default: 5 or config 'auto')"`
	Debug       bool `short:"D" long:"debug" description:"Enable debug output"`
	NoTimestamp bool `short:"T" long:"no-timestamp" description:"Disable timestamp logging"`

	// Authentication (also available via RETRI_SSH_PASSWORD / RETRI_SSH_SECRET)
	Password    string `short:"p" long:"password" description:"SSH Password (default: $RETRI_SSH_PASSWORD or config)"`
	Secret      string `short:"s" long:"secret" description:"Sudo Secret (default: $RETRI_SSH_SECRET or config)"`
	ExitCommand string `short:"e" long:"exit-command" description:"Exit command for interactive sessions (default: exit)"`

	// Misc
	ConfigHelp bool `short:"C" long:"config-help" description:"Show config file documentation"`
	Version    bool `short:"v" long:"version" description:"Show version information"`
}

// Run is the main entry point for the application.
// version is the build version string. defaultConfigContent and helpContent are embedded resources.
func Run(version string, defaultConfigContent []byte, helpContent string) {
	log.SetFlags(0)

	// 1. Parse CLI arguments
	var opts Options
	parser := flags.NewParser(&opts, flags.Default)
	parser.Name = config.AppName
	parser.Usage = "[OPTIONS]"

	if _, err := parser.Parse(); err != nil {
		if flags.WroteHelp(err) {
			os.Exit(0)
		}
		os.Exit(1)
	}

	if opts.Version {
		fmt.Printf("%s version %s\n", config.AppName, version)
		os.Exit(0)
	}

	if opts.ConfigHelp {
		fmt.Println(helpContent)
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
		if err := os.MkdirAll(commandsDir, 0755); err != nil {
			log.Printf("[WARNING] Failed to create commands directory: %v", err)
		}
	}

	// 3. Load config
	cfg := config.Load(opts.ConfigFile, defaultConfigPath)

	// 4. Validate config
	if err := cfg.Validate(); err != nil {
		log.Fatalf("[ERROR] Config validation failed: %v", err)
	}

	// 5. Expand host ranges (e.g., switch-[01-05] -> switch-01, switch-02...)
	config.ExpandHostsInConfig(&cfg)

	// 6. Record mode: no target or command specified → start session recording
	if opts.Host == "" && opts.Group == "" && opts.Command == "" && opts.CommandFile == "" {
		runRecordMode(opts, cfg.Defaults)
		return
	}

	// 7. Resolve targets
	targets := config.ResolveTargets(cfg, opts.Host, opts.Group)
	if len(targets) == 0 {
		if opts.Host == "" && opts.Group == "" {
			fmt.Println("------------------------------------------------------------")
			parser.WriteHelp(os.Stdout)
			os.Exit(0)
		}
		log.Fatalf("[ERROR] No targets specified.")
	}

	// 8. Determine parallel count
	parallelCount := config.DetermineParallelCount(cfg.Defaults.Parallel, opts.Parallel)

	// 8a. Prompt for missing credentials (only for SSH targets, not record mode).
	//     Check each target after full config resolution; prompt once if any are missing.
	fallbackPassword, fallbackSecret := promptMissingCredentials(targets, cfg.Defaults, opts.Password, opts.Secret, opts.LogDir, opts.Suffix, opts.FilenameFormat, opts.TimestampFormat)

	// 9. Main execution loop (parallel)
	log.Printf("Starting tasks for %d hosts (Parallel: %d)...", len(targets), parallelCount)

	var wg sync.WaitGroup
	sem := make(chan struct{}, parallelCount)

	for _, target := range targets {
		wg.Add(1)
		go func(rh config.ResolvedHost) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			executor.ExecuteHostTask(rh, cfg.Defaults,
				opts.Command, opts.Password, opts.Secret,
				opts.LogDir, opts.Suffix, opts.FilenameFormat, opts.TimestampFormat,
				opts.ExitCommand, fallbackPassword, fallbackSecret,
				opts.NoTimestamp, opts.Debug)
		}(target)
	}

	wg.Wait()
	log.Println("All tasks finished.")
}

// promptMissingCredentials checks resolved settings for each target and prompts
// the user (hidden input) for any credential that is missing across all targets.
// Returns fallback values to be applied only to hosts that have no credential set.
func promptMissingCredentials(targets []config.ResolvedHost, defaults config.GlobalOptions, cliPassword, cliSecret, cliLogDir, cliSuffix, cliFilenameFormat, cliTimestampFormat string) (fallbackPassword, fallbackSecret string) {
	var missingPasswordHosts, missingSecretHosts []string

	for _, rh := range targets {
		_, pw, sec, _, _, _, _, _ := config.ResolveSettings(rh, defaults, cliPassword, cliSecret, cliLogDir, cliSuffix, cliFilenameFormat, cliTimestampFormat)
		if pw == "" {
			missingPasswordHosts = append(missingPasswordHosts, rh.HostConfig.Host)
		}
		if sec == "" {
			missingSecretHosts = append(missingSecretHosts, rh.HostConfig.Host)
		}
	}

	if len(missingPasswordHosts) > 0 {
		fmt.Fprintf(os.Stderr, "[INFO] SSH password not set for: %s\n", strings.Join(missingPasswordHosts, ", "))
		fmt.Fprint(os.Stderr, "SSH Password (leave blank to skip): ")
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err == nil {
			fallbackPassword = string(b)
		}
	}

	if len(missingSecretHosts) > 0 {
		fmt.Fprintf(os.Stderr, "[INFO] Sudo secret not set for: %s\n", strings.Join(missingSecretHosts, ", "))
		fmt.Fprint(os.Stderr, "Sudo Secret (leave blank to skip): ")
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err == nil {
			fallbackSecret = string(b)
		}
	}

	return
}

// runRecordMode starts a local shell session recording.
func runRecordMode(opts Options, defaults config.GlobalOptions) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "localhost"
	}

	// Resolve log settings from CLI flags / config defaults
	logDir := opts.LogDir
	if logDir == "" && defaults.LogDir != "" {
		logDir = defaults.LogDir
	}
	suffix := opts.Suffix
	if suffix == "" {
		suffix = defaults.Suffix
	}
	fileFmt := opts.FilenameFormat
	if fileFmt == "" {
		fileFmt = defaults.FilenameFormat
	}
	tsFmt := opts.TimestampFormat
	if tsFmt == "" {
		tsFmt = defaults.TimestampFormat
	}

	lg, logFile, logPath, err := logger.SetupLogger(hostname, logDir, fileFmt, tsFmt, suffix, opts.NoTimestamp, defaults.Timestamp)
	if err != nil {
		log.Fatalf("[ERROR] Failed to setup logger: %v", err)
	}
	defer func() { _ = logFile.Close() }()

	// Write session header
	header := fmt.Sprintf("%s\n SESSION LOG : %s\n START TIME  : %s\n%s\n",
		strings.Repeat("=", 60), hostname, time.Now().Format("2006-01-02 15:04:05"), strings.Repeat("=", 60))
	lg.WriteRaw(header)

	log.Printf("Recording session to: %s", logPath)
	log.Printf("Start your work. Type 'exit' or press Ctrl-D to end recording.")

	if err := executor.RunRecordSession(lg, opts.Debug); err != nil {
		log.Printf("[ERROR] Record session error: %v", err)
	}

	// Write session footer
	footer := fmt.Sprintf("\n%s\n LOG END     : %s\n%s\n",
		strings.Repeat("=", 60), time.Now().Format("2006-01-02 15:04:05"), strings.Repeat("=", 60))
	lg.WriteRaw(footer)

	log.Printf("Session log saved: %s", logPath)
}
