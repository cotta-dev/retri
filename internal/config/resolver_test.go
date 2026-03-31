package config

import (
	"testing"
	"time"
)

func TestResolveTargets_SingleHost(t *testing.T) {
	cfg := Config{
		Hosts: []HostConfig{
			{Host: "server1", CommonFields: CommonFields{User: "admin"}},
			{Host: "server2"},
		},
	}

	targets := ResolveTargets(cfg, "server1", "")
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].HostConfig.Host != "server1" {
		t.Errorf("expected host 'server1', got %q", targets[0].HostConfig.Host)
	}
	if targets[0].HostConfig.User != "admin" {
		t.Errorf("expected user 'admin', got %q", targets[0].HostConfig.User)
	}
}

func TestResolveTargets_UnknownHost(t *testing.T) {
	cfg := Config{}

	targets := ResolveTargets(cfg, "unknown-host", "")
	if len(targets) != 1 {
		t.Fatalf("expected 1 target for unknown host, got %d", len(targets))
	}
	if targets[0].HostConfig.Host != "unknown-host" {
		t.Errorf("expected host 'unknown-host', got %q", targets[0].HostConfig.Host)
	}
	if targets[0].DeviceType != DefaultDeviceType {
		t.Errorf("expected device type %q, got %q", DefaultDeviceType, targets[0].DeviceType)
	}
}

func TestResolveTargets_Group(t *testing.T) {
	cfg := Config{
		Groups: []GroupConfig{
			{Name: "web", Hosts: []string{"web1", "web2"}},
		},
		Hosts: []HostConfig{
			{Host: "web1", CommonFields: CommonFields{User: "deploy"}},
		},
	}

	targets := ResolveTargets(cfg, "", "web")
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}

	// Check that the known host has its config
	for _, target := range targets {
		if target.HostConfig.Host == "web1" && target.HostConfig.User != "deploy" {
			t.Errorf("expected user 'deploy' for web1, got %q", target.HostConfig.User)
		}
	}
}

func TestResolveTargets_GroupFromHostDefinition(t *testing.T) {
	cfg := Config{
		Groups: []GroupConfig{
			{Name: "mygroup"},
		},
		Hosts: []HostConfig{
			{Host: "server1", Groups: []string{"mygroup"}},
		},
	}

	targets := ResolveTargets(cfg, "", "mygroup")
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].HostConfig.Host != "server1" {
		t.Errorf("expected host 'server1', got %q", targets[0].HostConfig.Host)
	}
}

func TestResolveTargets_AllHosts(t *testing.T) {
	cfg := Config{
		Hosts: []HostConfig{
			{Host: "host1"},
			{Host: "host2"},
			{Host: "host3"},
		},
	}

	targets := ResolveTargets(cfg, "", "")
	if len(targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(targets))
	}
}

func TestResolveTargets_EmptyConfig(t *testing.T) {
	cfg := Config{}

	targets := ResolveTargets(cfg, "", "")
	if targets != nil {
		t.Errorf("expected nil targets, got %v", targets)
	}
}

func TestResolveTargets_DeviceTypeResolution(t *testing.T) {
	cfg := Config{
		Defaults: GlobalOptions{DeviceType: "linux"},
		Groups: []GroupConfig{
			{Name: "switches", DeviceType: "cisco_ios", Hosts: []string{"sw1"}},
		},
		Hosts: []HostConfig{
			{Host: "sw1"},
			{Host: "sw2", DeviceType: "arista_eos"},
		},
		DeviceTypes: map[string]DeviceConfig{
			"cisco_ios":  {PromptRegex: "[#>] ?$"},
			"arista_eos": {PromptRegex: "[#>$] ?$"},
		},
	}

	// sw1 should inherit cisco_ios from group
	targets := ResolveTargets(cfg, "sw1", "")
	if targets[0].DeviceType != "cisco_ios" {
		t.Errorf("expected device type 'cisco_ios', got %q", targets[0].DeviceType)
	}

	// sw2 should use its own device type
	targets = ResolveTargets(cfg, "sw2", "")
	if targets[0].DeviceType != "arista_eos" {
		t.Errorf("expected device type 'arista_eos', got %q", targets[0].DeviceType)
	}
}

func TestDetermineParallelCount(t *testing.T) {
	tests := []struct {
		name      string
		configVal string
		cliVal    int
		minExpect int
	}{
		{"default", "", 0, DefaultParallel},
		{"config value", "10", 0, 10},
		{"cli overrides config", "10", 3, 3},
		{"auto", "auto", 0, 1}, // at least 1
		{"invalid config", "abc", 0, DefaultParallel},
		{"minimum is 1", "0", 0, DefaultParallel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetermineParallelCount(tt.configVal, tt.cliVal)
			if tt.name == "auto" {
				if got < tt.minExpect {
					t.Errorf("expected at least %d, got %d", tt.minExpect, got)
				}
			} else {
				if got != tt.minExpect {
					t.Errorf("expected %d, got %d", tt.minExpect, got)
				}
			}
		})
	}
}

func TestResolveSettings_Priority(t *testing.T) {
	rh := ResolvedHost{
		HostConfig: HostConfig{
			CommonFields: CommonFields{User: "host-user"},
		},
		GroupConfigs: []GroupConfig{
			{CommonFields: CommonFields{User: "group-user"}},
		},
		DeviceConfig: DeviceConfig{
			CommonFields: CommonFields{User: "device-user"},
		},
	}
	defaults := GlobalOptions{
		CommonFields: CommonFields{User: "default-user"},
	}

	user, _, _, _, _, _, _, _ := ResolveSettings(rh, defaults, "", "", "", "", "", "")
	if user != "host-user" {
		t.Errorf("expected host-user (highest priority), got %q", user)
	}
}

func TestResolveSettings_PromptTimeout(t *testing.T) {
	rh := ResolvedHost{
		HostConfig:   HostConfig{},
		GroupConfigs: nil,
		DeviceConfig: DeviceConfig{},
	}
	defaults := GlobalOptions{}

	_, _, _, _, _, _, _, timeout := ResolveSettings(rh, defaults, "", "", "", "", "", "")
	if timeout != DefaultPromptTimeout {
		t.Errorf("expected default timeout %v, got %v", DefaultPromptTimeout, timeout)
	}
}

func TestResolveSettings_PromptTimeoutOverride(t *testing.T) {
	rh := ResolvedHost{
		HostConfig:   HostConfig{CommonFields: CommonFields{PromptTimeout: 60}},
		GroupConfigs: nil,
		DeviceConfig: DeviceConfig{},
	}
	defaults := GlobalOptions{CommonFields: CommonFields{PromptTimeout: 120}}

	_, _, _, _, _, _, _, timeout := ResolveSettings(rh, defaults, "", "", "", "", "", "")
	if timeout != 60*time.Second {
		t.Errorf("expected 60s timeout, got %v", timeout)
	}
}
