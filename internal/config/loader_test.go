package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateDefault(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "sub", "config.yaml")
	content := []byte("defaults:\n  user: test\n")

	err := CreateDefault(configPath, content)
	if err != nil {
		t.Fatalf("CreateDefault failed: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read created config: %v", err)
	}

	if string(data) != string(content) {
		t.Errorf("config content mismatch: got %q, want %q", string(data), string(content))
	}
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0600); got != want {
		t.Fatalf("config permissions = %o, want %o", got, want)
	}
	dirInfo, err := os.Stat(filepath.Dir(configPath))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := dirInfo.Mode().Perm(), os.FileMode(0700); got != want {
		t.Fatalf("config directory permissions = %o, want %o", got, want)
	}
}

func TestCreateDefaultDoesNotOverwriteExistingConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	original := []byte("defaults:\n  user: existing\n")
	if err := os.WriteFile(configPath, original, 0600); err != nil {
		t.Fatal(err)
	}

	if err := CreateDefault(configPath, []byte("replacement\n")); err == nil {
		t.Fatal("CreateDefault() overwrote an existing config")
	}
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("existing config changed: got %q, want %q", got, original)
	}
}

func TestLoadCommandsFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	cmdFile := filepath.Join(tmpDir, "commands.txt")

	content := "df -h\n# this is a comment\nuptime\n\n  hostname  \n"
	if err := os.WriteFile(cmdFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	lines, err := LoadCommandsFromFile(cmdFile)
	if err != nil {
		t.Fatalf("LoadCommandsFromFile failed: %v", err)
	}

	expected := []string{"df -h", "uptime", "hostname"}
	if len(lines) != len(expected) {
		t.Fatalf("expected %d commands, got %d: %v", len(expected), len(lines), lines)
	}
	for i, line := range lines {
		if line != expected[i] {
			t.Errorf("command[%d] = %q, want %q", i, line, expected[i])
		}
	}
}

func TestLoadCommandsFromFile_NotFound(t *testing.T) {
	_, err := LoadCommandsFromFile("/nonexistent/path/commands.txt")
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}

func TestLoadLogCommandsOnly(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte("defaults:\n  log_commands_only: true\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := Load("", configPath)
	if cfg.Defaults.LogCommandsOnly == nil || !*cfg.Defaults.LogCommandsOnly {
		t.Fatalf("log_commands_only was not loaded: %#v", cfg.Defaults.LogCommandsOnly)
	}
}

func TestLoadLogEncoding(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := "defaults:\n  log_encoding: raw\ndevice_types:\n  cisco_ios:\n    log_encoding: shift_jis\n"
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := Load("", configPath)
	if got, want := cfg.Defaults.LogEncoding, "raw"; got != want {
		t.Fatalf("defaults.log_encoding = %q, want %q", got, want)
	}
	if got, want := cfg.DeviceTypes["cisco_ios"].LogEncoding, "shift_jis"; got != want {
		t.Fatalf("device_types.cisco_ios.log_encoding = %q, want %q", got, want)
	}
}
