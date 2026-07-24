package config

import (
	"fmt"
	"os"
	"regexp"
	"sort"

	"github.com/cotta-dev/retri/internal/logencoding"
)

// Validate checks the config for errors that would cause runtime failures.
// This includes validating prompt_regex patterns in device_types.
func (c *Config) Validate() error {
	if err := validateLogEncoding("defaults", c.Defaults.LogEncoding); err != nil {
		return err
	}

	deviceNames := make([]string, 0, len(c.DeviceTypes))
	for name := range c.DeviceTypes {
		deviceNames = append(deviceNames, name)
	}
	sort.Strings(deviceNames)
	for _, name := range deviceNames {
		dt := c.DeviceTypes[name]
		if dt.PromptRegex != "" {
			if _, err := regexp.Compile(dt.PromptRegex); err != nil {
				return fmt.Errorf("device_type '%s': invalid prompt_regex '%s': %w", name, dt.PromptRegex, err)
			}
		}
		if err := validateLogEncoding(fmt.Sprintf("device_type %q", name), dt.LogEncoding); err != nil {
			return err
		}
	}
	for i, group := range c.Groups {
		label := fmt.Sprintf("groups[%d]", i)
		if group.Name != "" {
			label = fmt.Sprintf("group %q", group.Name)
		}
		if err := validateLogEncoding(label, group.LogEncoding); err != nil {
			return err
		}
	}
	for i, host := range c.Hosts {
		label := fmt.Sprintf("hosts[%d]", i)
		if host.Host != "" {
			label = fmt.Sprintf("host %q", host.Host)
		}
		if err := validateLogEncoding(label, host.LogEncoding); err != nil {
			return err
		}
	}
	return nil
}

func validateLogEncoding(section, value string) error {
	if _, err := logencoding.Lookup(os.ExpandEnv(value)); err != nil {
		return fmt.Errorf("%s: %w", section, err)
	}
	return nil
}
