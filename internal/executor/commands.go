package executor

import (
	"log"

	"github.com/cotta-dev/retri/internal/config"
)

// CollectCommands gathers commands from all config layers.
// Commands are aggregated (not overridden) in order: defaults + groups + device_types + hosts + CLI.
func CollectCommands(rh config.ResolvedHost, defaults config.GlobalOptions, cliCommand string) []string {
	var cmds []string

	add := func(fp config.FieldProvider) {
		c := fp.Common()
		// Load from command file
		if c.CommandFile != "" {
			lines, err := config.LoadCommandsFromFile(c.CommandFile)
			if err == nil {
				cmds = append(cmds, lines...)
			} else {
				log.Printf("[%s] [ERROR] Failed to load commands from %s: %v", rh.HostConfig.Host, c.CommandFile, err)
			}
		}
		// Command list
		if len(c.Commands) > 0 {
			cmds = append(cmds, c.Commands...)
		}
		// Single command
		if c.Command != "" {
			cmds = append(cmds, c.Command)
		}
	}

	// Priority: Defaults + Groups + DeviceType + Host + CLI
	// (all are concatenated, not overridden)
	d := defaults
	add(&d)
	for i := range rh.GroupConfigs {
		add(&rh.GroupConfigs[i])
	}
	dc := rh.DeviceConfig
	add(&dc)
	hc := rh.HostConfig
	add(&hc)
	if cliCommand != "" {
		cmds = append(cmds, cliCommand)
	}

	return cmds
}
