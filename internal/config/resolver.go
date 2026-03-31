package config

import (
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// ResolveTargets builds the list of resolved hosts based on CLI arguments.
// If argHost is specified, only that host is returned.
// If argGroup is specified, all hosts in that group are returned.
// If neither is specified, all configured hosts are returned.
func ResolveTargets(cfg Config, argHost, argGroup string) []ResolvedHost {
	hostMap := make(map[string]HostConfig)
	for _, h := range cfg.Hosts {
		hostMap[h.Host] = h
	}

	groupMap := make(map[string]GroupConfig)
	for _, g := range cfg.Groups {
		if g.Name != "" {
			groupMap[g.Name] = g
		}
	}

	// Helper: find all groups a host belongs to
	findGroups := func(hName string, hConfGroups []string) []GroupConfig {
		var groups []GroupConfig

		// 1. Groups explicitly listed in the host definition
		for _, gName := range hConfGroups {
			if g, ok := groupMap[gName]; ok {
				groups = append(groups, g)
			}
		}

		// 2. Groups that list this host in their hosts field
		for _, g := range cfg.Groups {
			alreadyAdded := false
			for _, existing := range groups {
				if existing.Name == g.Name {
					alreadyAdded = true
					break
				}
			}
			if alreadyAdded {
				continue
			}

			for _, pattern := range g.Hosts {
				expanded := ExpandHostRange(pattern)
				for _, expandedHost := range expanded {
					if expandedHost == hName {
						groups = append(groups, g)
						goto NextGroup
					}
				}
			}
		NextGroup:
		}
		return groups
	}

	// Helper: get device type config
	findDeviceConfig := func(typeName string) DeviceConfig {
		if val, ok := cfg.DeviceTypes[typeName]; ok {
			return val
		}
		return DeviceConfig{}
	}

	// Helper: create a resolved host object
	createResolvedHost := func(hConf HostConfig) ResolvedHost {
		groups := findGroups(hConf.Host, hConf.Groups)

		// Determine device type (Defaults < Group < Host)
		dType := cfg.Defaults.DeviceType
		for _, g := range groups {
			if g.DeviceType != "" {
				dType = g.DeviceType
			}
		}
		if hConf.DeviceType != "" {
			dType = hConf.DeviceType
		}
		if dType == "" {
			dType = DefaultDeviceType
		}

		return ResolvedHost{
			HostConfig:   hConf,
			GroupConfigs: groups,
			DeviceType:   dType,
			DeviceConfig: findDeviceConfig(dType),
		}
	}

	var results []ResolvedHost

	// Case 1: Single host specified (-H)
	if argHost != "" {
		hConf, exists := hostMap[argHost]
		if !exists {
			// Allow execution on hosts not in config
			hConf = HostConfig{Host: argHost}
		}
		results = append(results, createResolvedHost(hConf))
		return results
	}

	// Case 2: Group specified (-g)
	if argGroup != "" {
		grpConf, exists := groupMap[argGroup]
		targetHostNames := make(map[string]bool)

		if exists {
			for _, pattern := range grpConf.Hosts {
				expanded := ExpandHostRange(pattern)
				for _, h := range expanded {
					targetHostNames[h] = true
				}
			}
		}

		// Also include hosts that reference this group
		for _, h := range cfg.Hosts {
			for _, gTag := range h.Groups {
				if gTag == argGroup {
					targetHostNames[h.Host] = true
				}
			}
		}

		if len(targetHostNames) == 0 {
			log.Printf("[WARNING] Group '%s' not found or empty.", argGroup)
			return nil
		}

		for hName := range targetHostNames {
			hConf, hExists := hostMap[hName]
			if !hExists {
				hConf = HostConfig{Host: hName}
			}
			results = append(results, createResolvedHost(hConf))
		}
		return results
	}

	// Case 3: All hosts (no arguments)
	if len(cfg.Hosts) > 0 {
		for _, h := range cfg.Hosts {
			results = append(results, createResolvedHost(h))
		}
		return results
	}

	return nil
}

// ResolveSettings resolves configuration values for a host using the priority chain:
// defaults < groups < device_types < hosts < environment variables < CLI.
// Returns user, password, secret, logDir, suffix, filenameFormat, timestampFormat, and promptTimeout.
func ResolveSettings(rh ResolvedHost, defaults GlobalOptions, cliPassword, cliSecret, cliLogDir, cliSuffix, cliFilenameFormat, cliTimestampFormat string) (user, password, secret, logDir, suffix, filenameFormat, timestampFormat string, promptTimeout time.Duration) {
	// Generic string resolver through the priority chain
	resolveStr := func(getField func(FieldProvider) string, envKey string, cliVal string) string {
		val := getField(&defaults)
		for i := range rh.GroupConfigs {
			if v := getField(&rh.GroupConfigs[i]); v != "" {
				val = v
			}
		}
		dc := rh.DeviceConfig
		if v := getField(&dc); v != "" {
			val = v
		}
		hc := rh.HostConfig
		if v := getField(&hc); v != "" {
			val = v
		}
		// Expand ${VAR} or $VAR in config values
		val = os.ExpandEnv(val)
		// Environment variable override (higher priority than config, lower than CLI)
		if envKey != "" && val == "" {
			val = os.Getenv(envKey)
		}
		if cliVal != "" {
			val = cliVal
		}
		return val
	}

	user = resolveStr(func(fp FieldProvider) string { return fp.Common().User }, "", "")
	password = resolveStr(func(fp FieldProvider) string { return fp.Common().Password }, "RETRI_SSH_PASSWORD", cliPassword)
	secret = resolveStr(func(fp FieldProvider) string { return fp.Common().Secret }, "RETRI_SSH_SECRET", cliSecret)
	logDir = resolveStr(func(fp FieldProvider) string { return fp.Common().LogDir }, "", cliLogDir)

	// Suffix only from defaults and groups (not from device_types/hosts)
	suffix = defaults.Suffix
	for _, g := range rh.GroupConfigs {
		if g.Suffix != "" {
			suffix = g.Suffix
		}
	}
	if cliSuffix != "" {
		suffix = cliSuffix
	}

	// Filename/timestamp format (defaults only + CLI)
	filenameFormat = defaults.FilenameFormat
	if cliFilenameFormat != "" {
		filenameFormat = cliFilenameFormat
	}
	timestampFormat = defaults.TimestampFormat
	if cliTimestampFormat != "" {
		timestampFormat = cliTimestampFormat
	}

	// Timeout resolution (int field, handled separately)
	promptTimeoutSec := defaults.PromptTimeout
	for _, g := range rh.GroupConfigs {
		if g.PromptTimeout > 0 {
			promptTimeoutSec = g.PromptTimeout
		}
	}
	if rh.DeviceConfig.PromptTimeout > 0 {
		promptTimeoutSec = rh.DeviceConfig.PromptTimeout
	}
	if rh.HostConfig.PromptTimeout > 0 {
		promptTimeoutSec = rh.HostConfig.PromptTimeout
	}
	if promptTimeoutSec <= 0 {
		promptTimeoutSec = int(DefaultPromptTimeout / time.Second)
	}
	promptTimeout = time.Duration(promptTimeoutSec) * time.Second

	return
}

// DetermineParallelCount calculates the parallel execution count from config and CLI values.
func DetermineParallelCount(configVal string, cliVal int) int {
	count := DefaultParallel

	if configVal != "" {
		if strings.ToLower(configVal) == "auto" {
			count = runtime.NumCPU()
		} else {
			if val, err := strconv.Atoi(configVal); err == nil && val > 0 {
				count = val
			}
		}
	}

	// CLI value takes precedence
	if cliVal > 0 {
		count = cliVal
	}

	if count < 1 {
		count = 1
	}
	return count
}
