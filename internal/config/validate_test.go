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
