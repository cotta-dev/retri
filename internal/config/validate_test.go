package config

import (
	"strings"
	"testing"
)

func TestValidate_ValidConfig(t *testing.T) {
	cfg := Config{
		DeviceTypes: map[string]DeviceConfig{
			"cisco_ios": {PromptRegex: "[#>] ?$"},
			"linux":     {},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid config, got error: %v", err)
	}
}

func TestValidate_InvalidRegex(t *testing.T) {
	cfg := Config{
		DeviceTypes: map[string]DeviceConfig{
			"bad_device": {PromptRegex: "[invalid"},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid regex, got nil")
	}
	if !strings.Contains(err.Error(), "bad_device") {
		t.Errorf("expected error to mention device name 'bad_device', got: %v", err)
	}
	if !strings.Contains(err.Error(), "prompt_regex") {
		t.Errorf("expected error to mention 'prompt_regex', got: %v", err)
	}
}

func TestValidate_EmptyRegex(t *testing.T) {
	cfg := Config{
		DeviceTypes: map[string]DeviceConfig{
			"device": {PromptRegex: ""},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("expected empty regex to be valid, got error: %v", err)
	}
}

func TestValidate_EmptyConfig(t *testing.T) {
	cfg := Config{}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected empty config to be valid, got error: %v", err)
	}
}

func TestValidateLogEncodingInEverySection(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{name: "defaults", cfg: Config{Defaults: GlobalOptions{CommonFields: CommonFields{LogEncoding: "guess"}}}, want: "defaults"},
		{name: "device type", cfg: Config{DeviceTypes: map[string]DeviceConfig{"ios": {CommonFields: CommonFields{LogEncoding: "guess"}}}}, want: `device_type "ios"`},
		{name: "group", cfg: Config{Groups: []GroupConfig{{Name: "switches", CommonFields: CommonFields{LogEncoding: "guess"}}}}, want: `group "switches"`},
		{name: "host", cfg: Config{Hosts: []HostConfig{{Host: "switch-01", CommonFields: CommonFields{LogEncoding: "guess"}}}}, want: `host "switch-01"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want section %q", err, tt.want)
			}
		})
	}
}

func TestValidateAcceptsSupportedLogEncodingAlias(t *testing.T) {
	cfg := Config{Defaults: GlobalOptions{CommonFields: CommonFields{LogEncoding: "Windows-31J"}}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateExpandsLogEncodingEnvironmentVariable(t *testing.T) {
	t.Setenv("DEVICE_LOG_ENCODING", "shift_jis")
	cfg := Config{Defaults: GlobalOptions{CommonFields: CommonFields{LogEncoding: "${DEVICE_LOG_ENCODING}"}}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
