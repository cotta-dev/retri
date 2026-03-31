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
