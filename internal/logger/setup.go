package logger

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/cotta-dev/retri/internal/logencoding"
)

// SetupOptions controls creation of one session log.
type SetupOptions struct {
	Directory        string
	FilenameFormat   string
	TimestampFormat  string
	Suffix           string
	Encoding         string
	NoTimestamp      bool
	DefaultTimestamp *bool
}

// toGoTimeFormat converts human-readable symbols to Go's reference time format.
// Supported: YYYY MM DD HH mm ss
func toGoTimeFormat(f string) string {
	f = strings.ReplaceAll(f, "YYYY", "2006")
	f = strings.ReplaceAll(f, "MM", "01")
	f = strings.ReplaceAll(f, "DD", "02")
	f = strings.ReplaceAll(f, "HH", "15")
	f = strings.ReplaceAll(f, "mm", "04")
	f = strings.ReplaceAll(f, "ss", "05")
	return f
}

// Setup creates one private session log and optionally converts terminal output
// to UTF-8. It never overwrites an existing path or creates a sidecar .raw file.
// Malformed input falls back to the rendered source bytes in the same log.
func Setup(host string, options SetupOptions) (*LineLogger, *os.File, string, error) {
	codec, err := logencoding.Lookup(options.Encoding)
	if err != nil {
		return nil, nil, "", err
	}
	if strings.ContainsFunc(host, unicode.IsControl) {
		return nil, nil, "", fmt.Errorf("unsafe host label %q: control characters are not allowed", host)
	}
	filename, err := logFilename(host, options)
	if err != nil {
		return nil, nil, "", err
	}

	dir, err := resolveLogDirectory(options.Directory)
	if err != nil {
		return nil, nil, "", err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, nil, "", fmt.Errorf("create log directory %q: %w", dir, err)
	}

	logPath := filepath.Join(dir, filename)
	f, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, nil, "", fmt.Errorf("create log file %q: %w", logPath, err)
	}

	warn := func(err error) {
		log.Printf("[%s] [WARNING] log_encoding %q could not decode terminal output; preserving the rendered source bytes in %s: %v", host, codec.Name(), logPath, err)
	}
	return newLineLogger(f, timestampEnabled(options), codec, warn), f, logPath, nil
}

// Finalize flushes buffered terminal output, syncs it to storage, and closes
// the file. It reports any write, sync, or close failure.
func Finalize(lineLogger *LineLogger, file *os.File) error {
	lineLogger.Flush()
	writeErr := lineLogger.Err()
	syncErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(writeErr, syncErr, closeErr)
}

func resolveLogDirectory(dir string) (string, error) {
	if dir == "" {
		dir = "~/retri-logs"
	}
	if dir[0] != '~' {
		return dir, nil
	}
	if dir != "~" && !strings.HasPrefix(dir, "~/") {
		return "", fmt.Errorf("unsupported home directory path %q; use ~ or ~/path", dir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	if dir == "~" {
		return home, nil
	}
	return filepath.Join(home, strings.TrimPrefix(dir, "~/")), nil
}

func logFilename(host string, options SetupOptions) (string, error) {
	fileFmt := options.FilenameFormat
	if fileFmt == "" {
		fileFmt = "{host}_{timestamp}{suffix}.log"
	}
	timeFmt := options.TimestampFormat
	if timeFmt == "" {
		timeFmt = "YYYYMMDD_HHmmss"
	}
	suffix := options.Suffix
	if suffix != "" && !strings.HasPrefix(suffix, "_") {
		suffix = "_" + suffix
	}

	filename := strings.NewReplacer(
		"{host}", host,
		"{timestamp}", time.Now().Format(toGoTimeFormat(timeFmt)),
		"{suffix}", suffix,
	).Replace(fileFmt)
	if !isSafeLogFilename(filename) {
		return "", fmt.Errorf("unsafe log filename %q: filename_format, host, and suffix must produce one local filename without path separators or control characters", filename)
	}
	return filename, nil
}

func isSafeLogFilename(filename string) bool {
	if filename == "" || filename == "." || filename == ".." || filepath.IsAbs(filename) || filepath.Base(filename) != filename || strings.ContainsAny(filename, `/\`) {
		return false
	}
	return !strings.ContainsFunc(filename, unicode.IsControl)
}

func timestampEnabled(options SetupOptions) bool {
	enabled := true
	if options.DefaultTimestamp != nil {
		enabled = *options.DefaultTimestamp
	}
	return enabled && !options.NoTimestamp
}
