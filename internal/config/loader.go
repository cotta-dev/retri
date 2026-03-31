package config

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Load reads and parses a YAML config file.
// If path is specified, it is used directly. Otherwise, defaultPath is tried.
func Load(path string, defaultPath string) Config {
	var data []byte
	var err error

	if path != "" {
		data, err = os.ReadFile(path)
		if err != nil {
			log.Fatalf("[ERROR] Config not found: %s", path)
		}
	} else if _, statErr := os.Stat(defaultPath); statErr == nil {
		data, err = os.ReadFile(defaultPath)
		if err != nil {
			log.Fatalf("[ERROR] Failed to read config: %s: %v", defaultPath, err)
		}
	}

	var cfg Config
	if len(data) > 0 {
		dec := yaml.NewDecoder(bytes.NewReader(data))
		dec.KnownFields(true)
		if err := dec.Decode(&cfg); err != nil {
			log.Fatalf("[ERROR] YAML parse error: %v", err)
		}
	}
	return cfg
}

// CreateDefault writes the default config file to path, creating parent directories as needed.
func CreateDefault(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	if err := os.WriteFile(path, content, 0644); err != nil {
		return fmt.Errorf("failed to write default config: %w", err)
	}
	return nil
}

// LoadCommandsFromFile reads commands from a file, one per line.
// Lines starting with "#" and empty lines are skipped.
// If the file is not found at the given path and it's relative,
// it falls back to looking in ~/.config/retri/commands/.
func LoadCommandsFromFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		// If file not found and path is relative, try the commands directory
		if os.IsNotExist(err) && !filepath.IsAbs(path) {
			homeDir, homeErr := os.UserHomeDir()
			if homeErr != nil {
				return nil, fmt.Errorf("failed to get home directory: %w", homeErr)
			}
			fallbackPath := filepath.Join(homeDir, ".config", AppName, CommandsDirName, path)
			f2, err2 := os.Open(fallbackPath)
			if err2 == nil {
				f = f2
				err = nil
			}
		}
	}

	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}
