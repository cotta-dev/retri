package config

import (
	"reflect"
	"testing"
)

func TestExpandHostRange(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		want    []string
	}{
		{
			name:    "no range",
			pattern: "server1",
			want:    []string{"server1"},
		},
		{
			name:    "simple range",
			pattern: "switch-[01-03]",
			want:    []string{"switch-01", "switch-02", "switch-03"},
		},
		{
			name:    "zero-padded range",
			pattern: "host-[001-003]",
			want:    []string{"host-001", "host-002", "host-003"},
		},
		{
			name:    "single element range",
			pattern: "srv-[01-01]",
			want:    []string{"srv-01"},
		},
		{
			name:    "range with suffix",
			pattern: "rack[1-3]-sw",
			want:    []string{"rack1-sw", "rack2-sw", "rack3-sw"},
		},
		{
			name:    "IP-like pattern",
			pattern: "192.168.1.[1-3]",
			want:    []string{"192.168.1.1", "192.168.1.2", "192.168.1.3"},
		},
		{
			name:    "empty string",
			pattern: "",
			want:    []string{""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpandHostRange(tt.pattern)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExpandHostRange(%q) = %v, want %v", tt.pattern, got, tt.want)
			}
		})
	}
}

func TestExpandHostsInConfig(t *testing.T) {
	cfg := Config{
		Hosts: []HostConfig{
			{Host: "web-[01-02]", CommonFields: CommonFields{User: "admin"}},
			{Host: "db-server", CommonFields: CommonFields{User: "root"}},
		},
	}

	ExpandHostsInConfig(&cfg)

	if len(cfg.Hosts) != 3 {
		t.Fatalf("expected 3 hosts, got %d", len(cfg.Hosts))
	}

	expected := []string{"web-01", "web-02", "db-server"}
	for i, h := range cfg.Hosts {
		if h.Host != expected[i] {
			t.Errorf("host[%d] = %q, want %q", i, h.Host, expected[i])
		}
	}

	// Verify settings are preserved
	if cfg.Hosts[0].User != "admin" {
		t.Errorf("host[0].User = %q, want %q", cfg.Hosts[0].User, "admin")
	}
	if cfg.Hosts[2].User != "root" {
		t.Errorf("host[2].User = %q, want %q", cfg.Hosts[2].User, "root")
	}
}
