package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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

// SetupLogger creates the log directory, file, and LineLogger for a host.
func SetupLogger(host, dir, fileFmt, timeFmt, suffix string, noTimestamp bool, defaultTimestamp *bool) (*LineLogger, *os.File, string, error) {
	if dir == "" {
		dir = "~/retri-logs"
	}
	if strings.HasPrefix(dir, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, nil, "", fmt.Errorf("failed to get home directory: %w", err)
		}
		dir = filepath.Join(home, dir[1:])
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, nil, "", err
	}

	// Determine timestamp setting
	tsEnabled := true
	if defaultTimestamp != nil {
		tsEnabled = *defaultTimestamp
	}
	if noTimestamp {
		tsEnabled = false
	}

	// Filename format
	if fileFmt == "" {
		fileFmt = "{host}_{timestamp}{suffix}.log"
	}
	if timeFmt == "" {
		timeFmt = "YYYYMMDD_HHmmss"
	}

	nowStr := time.Now().Format(toGoTimeFormat(timeFmt))
	if suffix != "" && !strings.HasPrefix(suffix, "_") {
		suffix = "_" + suffix
	}

	filename := strings.ReplaceAll(fileFmt, "{host}", host)
	filename = strings.ReplaceAll(filename, "{timestamp}", nowStr)
	filename = strings.ReplaceAll(filename, "{suffix}", suffix)

	logPath := filepath.Join(dir, filename)
	f, err := os.Create(logPath)
	if err != nil {
		return nil, nil, "", err
	}

	return NewLineLogger(f, tsEnabled), f, logPath, nil
}
