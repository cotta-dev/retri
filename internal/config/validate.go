package config

import (
	"fmt"
	"regexp"
)

// Validate checks the config for errors that would cause runtime failures.
// This includes validating prompt_regex patterns in device_types.
func (c *Config) Validate() error {
	for name, dt := range c.DeviceTypes {
		if dt.PromptRegex != "" {
			if _, err := regexp.Compile(dt.PromptRegex); err != nil {
				return fmt.Errorf("device_type '%s': invalid prompt_regex '%s': %w", name, dt.PromptRegex, err)
			}
		}
	}
	return nil
}
