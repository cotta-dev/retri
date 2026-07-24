package logger

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSetupKeepsFallbackInSingleLogFile(t *testing.T) {
	dir := t.TempDir()
	timestamp := false
	ll, file, logPath, err := Setup("switch", SetupOptions{
		Directory:        dir,
		FilenameFormat:   "{host}.log",
		Encoding:         "shift_jis",
		DefaultTimestamp: &timestamp,
	})
	if err != nil {
		t.Fatal(err)
	}

	original := []byte{'b', 'a', 'd', ':', 0x82, '\n'}
	_, _ = ll.Write(original)
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "switch.log" {
		t.Fatalf("created files = %v, want only switch.log", entries)
	}
	info, err := entries[0].Info()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0600); got != want {
		t.Fatalf("log permissions = %o, want %o", got, want)
	}
	got, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("log = %x, want original bytes %x", got, original)
	}
}

func TestSetupRejectsEncodingBeforeCreatingFiles(t *testing.T) {
	dir := t.TempDir()
	if _, _, _, err := Setup("switch", SetupOptions{Directory: dir, FilenameFormat: "{host}.log", Encoding: "auto"}); err == nil {
		t.Fatal("Setup() accepted an unsupported encoding")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("created files after validation error: %v", entries)
	}
}

func TestSetupRejectsUnsafeFilename(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		options SetupOptions
	}{
		{name: "parent traversal", host: "switch", options: SetupOptions{FilenameFormat: "../escape.log"}},
		{name: "host separator", host: "../switch", options: SetupOptions{FilenameFormat: "{host}.log"}},
		{name: "suffix separator", host: "switch", options: SetupOptions{FilenameFormat: "{host}{suffix}.log", Suffix: "../escape"}},
		{name: "control character", host: "switch\nforged", options: SetupOptions{FilenameFormat: "{host}.log"}},
		{name: "control character outside filename", host: "switch\nforged", options: SetupOptions{FilenameFormat: "fixed.log"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.options.Directory = dir
			if _, _, _, err := Setup(tt.host, tt.options); err == nil {
				t.Fatal("Setup() accepted an unsafe filename")
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("created files after validation error: %v", entries)
			}
		})
	}
}

func TestSetupDoesNotOverwriteExistingLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "switch.log")
	original := []byte("existing log\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}

	if _, _, _, err := Setup("switch", SetupOptions{Directory: dir, FilenameFormat: "{host}.log"}); err == nil {
		t.Fatal("Setup() overwrote an existing log")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("existing log changed: got %q, want %q", got, original)
	}
}

func TestSetupDoesNotFollowExistingLogSymlink(t *testing.T) {
	dir := t.TempDir()
	victimPath := filepath.Join(dir, "victim")
	original := []byte("do not replace\n")
	if err := os.WriteFile(victimPath, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victimPath, filepath.Join(dir, "switch.log")); err != nil {
		t.Fatal(err)
	}

	if _, _, _, err := Setup("switch", SetupOptions{Directory: dir, FilenameFormat: "{host}.log"}); err == nil {
		t.Fatal("Setup() followed an existing log symlink")
	}
	got, err := os.ReadFile(victimPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("symlink target changed: got %q, want %q", got, original)
	}
}

func TestFinalizeFlushesAndClosesLog(t *testing.T) {
	dir := t.TempDir()
	timestamp := false
	ll, file, logPath, err := Setup("switch", SetupOptions{
		Directory:        dir,
		FilenameFormat:   "{host}.log",
		DefaultTimestamp: &timestamp,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ll.Write([]byte("pending prompt")); err != nil {
		t.Fatal(err)
	}
	if err := Finalize(ll, file); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "pending prompt\n" {
		t.Fatalf("final log = %q, want pending line", got)
	}
	if _, err := file.Write([]byte("late")); err == nil {
		t.Fatal("log file remained open after Finalize()")
	}
}
