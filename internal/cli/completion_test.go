package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cotta-dev/retri/internal/config"
)

func TestCompletionOptionsAtRoot(t *testing.T) {
	candidates := completionCandidates([]string{""}, 0)

	if !hasCompletion(candidates, "--log-dir") {
		t.Fatal("expected --log-dir completion")
	}
	if !hasCompletion(candidates, "--log-commands-only") {
		t.Fatal("expected --log-commands-only completion")
	}
	if !hasCompletion(candidates, "--log-encoding") {
		t.Fatal("expected --log-encoding completion")
	}
	if hasCompletion(candidates, "-d") {
		t.Fatal("did not expect duplicate short option at root")
	}
	candidate, ok := findCompletion(candidates, "--log-dir")
	if !ok || candidate.Display != "-d, --log-dir" {
		t.Fatalf("expected combined option display, got %#v", candidate)
	}
}

func TestCompletionSuggestsLogEncodingValue(t *testing.T) {
	candidates := completionCandidates([]string{"--log-encoding", ""}, 1)
	if len(candidates) != 1 || candidates[0].Kind != completionKindHint || !strings.Contains(candidates[0].Description, "shift_jis") {
		t.Fatalf("expected log encoding hint, got %#v", candidates)
	}
}

func TestCompletionKeepsExactShortOptionBeforeValue(t *testing.T) {
	candidates := completionCandidates([]string{"-d"}, 0)

	if len(candidates) != 1 || candidates[0].Value != "-d" {
		t.Fatalf("expected exact -d completion, got %#v", candidates)
	}
}

func TestCompletionSuggestsDirectoryAfterLogDirOption(t *testing.T) {
	candidates := completionCandidates([]string{"-d", ""}, 1)

	if len(candidates) != 1 || candidates[0].Kind != completionKindHint || candidates[0].Description == "" {
		t.Fatalf("expected directory hint, got %#v", candidates)
	}
}

func TestCompletionSSHHosts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mustWrite(t, filepath.Join(home, ".ssh", "config"), "Host app-01 app-02 *.example\n")
	mustWrite(t, filepath.Join(home, ".ssh", "known_hosts"), "db-01 ssh-ed25519 AAAA\n[db-02]:2222 ssh-ed25519 AAAA\n|1|hashed ssh-ed25519 AAAA\n")

	candidates := completionCandidates([]string{"app"}, 0)
	if !hasCompletion(candidates, "app-01") || !hasCompletion(candidates, "app-02") {
		t.Fatalf("expected ssh config hosts, got %#v", candidates)
	}
	if hasCompletion(candidates, "*.example") {
		t.Fatalf("did not expect wildcard host, got %#v", candidates)
	}

	candidates = completionCandidates([]string{"db"}, 0)
	if !hasCompletion(candidates, "db-01") || !hasCompletion(candidates, "db-02") {
		t.Fatalf("expected known_hosts entries, got %#v", candidates)
	}
}

func TestCompletionGroupsFromConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configPath := filepath.Join(home, ".config", "retri", "config.yaml")
	mustWrite(t, configPath, "groups:\n  - name: web\n  - name: db\n")

	candidates := completionCandidates([]string{"-g", ""}, 1)
	if len(candidates) != 1 || candidates[0].Kind != completionKindHint || !strings.Contains(candidates[0].Description, "<group name>") {
		t.Fatalf("expected group hint, got %#v", candidates)
	}
}

func TestShouldLogCommandsOnly(t *testing.T) {
	enabled := true
	disabled := false

	if !shouldLogCommandsOnly(Options{}, config.GlobalOptions{LogCommandsOnly: &enabled}) {
		t.Fatal("config should enable command/output-only logging")
	}
	if shouldLogCommandsOnly(Options{}, config.GlobalOptions{LogCommandsOnly: &disabled}) {
		t.Fatal("explicit config false should keep full-session logging")
	}
	if !shouldLogCommandsOnly(Options{LogCommandsOnly: true}, config.GlobalOptions{LogCommandsOnly: &disabled}) {
		t.Fatal("CLI flag should override config false")
	}
}

func TestPackagedCompletionScriptsMatchGeneratedScripts(t *testing.T) {
	tests := []struct {
		shell string
		path  string
	}{
		{shell: "bash", path: filepath.Join("..", "..", "completions", "retri.bash")},
		{shell: "zsh", path: filepath.Join("..", "..", "completions", "retri.zsh")},
		{shell: "fish", path: filepath.Join("..", "..", "completions", "retri.fish")},
	}

	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			want, err := os.ReadFile(tt.path)
			if err != nil {
				t.Fatal(err)
			}

			var got bytes.Buffer
			if err := writeCompletionScript(&got, tt.shell); err != nil {
				t.Fatal(err)
			}

			if got.String() != string(want) {
				t.Fatalf("generated %s completion script differs from packaged file", tt.shell)
			}
		})
	}
}

func hasCompletion(candidates []completionCandidate, value string) bool {
	_, ok := findCompletion(candidates, value)
	return ok
}

func findCompletion(candidates []completionCandidate, value string) (completionCandidate, bool) {
	for _, candidate := range candidates {
		if candidate.Value == value {
			return candidate, true
		}
	}
	return completionCandidate{}, false
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
}
