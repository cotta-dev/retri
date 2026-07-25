package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cotta-dev/retri/internal/config"
	"gopkg.in/yaml.v3"
)

func TestEmbeddedDefaultConfigUsesCurrentSchema(t *testing.T) {
	cfg := decodeDefaultConfig(t, defaultConfigContent)
	if len(cfg.Hosts) != 0 || len(cfg.Groups) != 0 {
		t.Fatalf("default config activates targets: hosts=%v groups=%v", cfg.Hosts, cfg.Groups)
	}

	content := string(defaultConfigContent)
	for _, field := range []string{
		`# Run "retri -C" to view all available options, priorities, and examples.`,
		`# commands: ["df -h", "uptime"]`,
		`# command_file: "commands.txt"`,
		`# prompt_timeout: 300`,
		`# timestamp_format: "YYYYMMDD_HHmmss"`,
	} {
		if !strings.Contains(content, field) {
			t.Errorf("default config is missing current setting example %q", field)
		}
	}
}

func TestEmbeddedDefaultGroupExampleIsUsable(t *testing.T) {
	lines := strings.Split(string(defaultConfigContent), "\n")
	inGroups := false
	for i, line := range lines {
		if line == "groups:" {
			inGroups = true
			continue
		}
		if inGroups && line != "" && !strings.HasPrefix(line, " ") {
			inGroups = false
		}
		if inGroups && strings.HasPrefix(line, "  # ") {
			lines[i] = strings.Replace(line, "  # ", "  ", 1)
		}
	}

	cfg := decodeDefaultConfig(t, []byte(strings.Join(lines, "\n")))
	if len(cfg.Groups) != 1 {
		t.Fatalf("enabled group example count = %d, want 1", len(cfg.Groups))
	}
	group := cfg.Groups[0]
	if group.Name != "web" || len(group.Hosts) != 1 || group.Hosts[0] != "myserver" {
		t.Fatalf("enabled group example = %#v", group)
	}
}

func TestEmbeddedConfigsPutGroupsBeforeHosts(t *testing.T) {
	configs := map[string]string{
		"initial config": string(defaultConfigContent),
		"config help":    helpContent,
	}

	for name, content := range configs {
		t.Run(name, func(t *testing.T) {
			groupsIndex := strings.Index(content, "\ngroups:\n")
			hostsIndex := strings.Index(content, "\nhosts:\n")
			if groupsIndex < 0 || hostsIndex < 0 {
				t.Fatalf("missing groups or hosts section")
			}
			if groupsIndex > hostsIndex {
				t.Fatalf("groups section appears after hosts section")
			}
		})
	}
}

func decodeDefaultConfig(t *testing.T, content []byte) config.Config {
	t.Helper()
	var cfg config.Config
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		t.Fatalf("embedded default config does not match current schema: %v", err)
	}
	return cfg
}
