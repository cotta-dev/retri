package main

import (
	_ "embed"
	"runtime/debug"

	"github.com/cotta-dev/retri/internal/cli"
)

//go:embed configs/config.yaml
var defaultConfigContent []byte

//go:embed configs/config-help.txt
var helpContent string

// Version is set via ldflags by GoReleaser (e.g., -X main.Version=1.2.3).
// When installed via "go install", the module version is read from build info instead.
var Version = "dev"

func main() {
	if Version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			Version = info.Main.Version
		}
	}
	cli.Run(Version, defaultConfigContent, helpContent)
}
