package executor

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/cotta-dev/retri/internal/config"
)

func TestCollectCommandsIncludesEverySource(t *testing.T) {
	dir := t.TempDir()
	defaultFile := writeCommandFile(t, dir, "defaults.txt", "default-file")
	groupFile := writeCommandFile(t, dir, "group.txt", "group-file")
	deviceFile := writeCommandFile(t, dir, "device.txt", "device-file")
	hostFile := writeCommandFile(t, dir, "host.txt", "host-file")
	cliFile := writeCommandFile(t, dir, "cli.txt", "cli-file")

	defaults := config.GlobalOptions{CommonFields: config.CommonFields{
		CommandFile: defaultFile,
		Commands:    []string{"default-list"},
		Command:     "default-single",
	}}
	host := config.ResolvedHost{
		GroupConfigs: []config.GroupConfig{{
			CommonFields: config.CommonFields{
				CommandFile: groupFile,
				Commands:    []string{"group-list"},
				Command:     "group-single",
			},
		}},
		DeviceConfig: config.DeviceConfig{CommonFields: config.CommonFields{
			CommandFile: deviceFile,
			Commands:    []string{"device-list"},
			Command:     "device-single",
		}},
		HostConfig: config.HostConfig{
			Host: "test-host",
			CommonFields: config.CommonFields{
				CommandFile: hostFile,
				Commands:    []string{"host-list"},
				Command:     "host-single",
			},
		},
	}

	got := CollectCommands(host, defaults, cliFile, "cli-single")
	want := []string{
		"default-file", "default-list", "default-single",
		"group-file", "group-list", "group-single",
		"device-file", "device-list", "device-single",
		"host-file", "host-list", "host-single",
		"cli-file", "cli-single",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CollectCommands() = %#v, want %#v", got, want)
	}
}

func TestBuildExecutionCommandsDoesNotAliasSharedSetupCommands(t *testing.T) {
	sharedSetup := make([]string, 1, 8)
	sharedSetup[0] = "terminal length 0"

	first := buildExecutionCommands(sharedSetup, []string{"show host-a"})
	second := buildExecutionCommands(sharedSetup, []string{"show host-b"})

	first[0] = "changed setup"
	first[1] = "changed host command"
	if got, want := second, []string{"terminal length 0", "show host-b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("second host command snapshot changed through aliasing: got %#v, want %#v", got, want)
	}
	if sharedSetup[0] != "terminal length 0" {
		t.Fatalf("shared setup command was mutated: %#v", sharedSetup)
	}
	for i, command := range sharedSetup[:cap(sharedSetup)] {
		if i > 0 && command != "" {
			t.Fatalf("shared setup backing array was overwritten at index %d: %q", i, command)
		}
	}
}

func writeCommandFile(t *testing.T, dir, name string, commands ...string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	var content []byte
	for _, command := range commands {
		content = append(content, command...)
		content = append(content, '\n')
	}
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
