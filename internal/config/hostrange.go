package config

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var hostRangeRe = regexp.MustCompile(`\[(\d+)-(\d+)\]`)

// ExpandHostRange expands a host pattern like "switch-[01-05]" into individual hostnames.
// If no range pattern is found, the original pattern is returned as a single-element slice.
func ExpandHostRange(pattern string) []string {
	matches := hostRangeRe.FindStringSubmatchIndex(pattern)

	if matches == nil {
		return []string{pattern}
	}

	startIdx, endIdx := matches[0], matches[1]
	prefix := pattern[:startIdx]
	suffix := pattern[endIdx:]

	rangeStartStr := pattern[matches[2]:matches[3]]
	rangeEndStr := pattern[matches[4]:matches[5]]

	start, _ := strconv.Atoi(rangeStartStr)
	end, _ := strconv.Atoi(rangeEndStr)

	padding := 0
	if strings.HasPrefix(rangeStartStr, "0") || len(rangeStartStr) == len(rangeEndStr) {
		padding = len(rangeStartStr)
	}

	var result []string
	for i := start; i <= end; i++ {
		numStr := strconv.Itoa(i)
		if padding > 0 {
			numStr = fmt.Sprintf("%0*d", padding, i)
		}
		result = append(result, prefix+numStr+suffix)
	}
	return result
}

// ExpandHostsInConfig expands host range patterns in all host entries.
func ExpandHostsInConfig(cfg *Config) {
	var expandedHosts []HostConfig
	for _, h := range cfg.Hosts {
		names := ExpandHostRange(h.Host)
		for _, name := range names {
			newH := h
			newH.Host = name
			expandedHosts = append(expandedHosts, newH)
		}
	}
	cfg.Hosts = expandedHosts
}
