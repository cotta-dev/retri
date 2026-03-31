package config

// CommonFields contains settings shared across multiple config sections.
// Embedded with yaml:",inline" to flatten fields in YAML.
type CommonFields struct {
	User          string   `yaml:"user"`
	Password      string   `yaml:"password"`
	Secret        string   `yaml:"secret"`
	Command       string   `yaml:"command"`
	Commands      []string `yaml:"commands"`
	CommandFile   string   `yaml:"command_file"`
	LogDir        string   `yaml:"log_dir"`
	PromptTimeout int      `yaml:"prompt_timeout"`
}

// FieldProvider is implemented by config sections that contain CommonFields.
type FieldProvider interface {
	Common() *CommonFields
}

// Config represents the entire config.yaml structure.
type Config struct {
	Defaults    GlobalOptions           `yaml:"defaults"`
	DeviceTypes map[string]DeviceConfig `yaml:"device_types"`
	Hosts       []HostConfig            `yaml:"hosts"`
	Groups      []GroupConfig           `yaml:"groups"`
}

// GlobalOptions represents the "defaults" section.
type GlobalOptions struct {
	CommonFields    `yaml:",inline"`
	Timestamp       *bool  `yaml:"timestamp"`
	FilenameFormat  string `yaml:"filename_format"`
	TimestampFormat string `yaml:"timestamp_format"`
	Suffix          string `yaml:"suffix"`
	DeviceType      string `yaml:"device_type"`
	Parallel        string `yaml:"parallel"`
}

// Common returns the embedded CommonFields.
func (g *GlobalOptions) Common() *CommonFields { return &g.CommonFields }

// DeviceConfig represents a single entry in the "device_types" section.
type DeviceConfig struct {
	CommonFields  `yaml:",inline"`
	SetupCommands []string `yaml:"setup_commands"`
	PromptRegex   string   `yaml:"prompt_regex"`
	ExitCommand   string   `yaml:"exit_command"`
}

// Common returns the embedded CommonFields.
func (d *DeviceConfig) Common() *CommonFields { return &d.CommonFields }

// GroupConfig represents a single entry in the "groups" section.
type GroupConfig struct {
	CommonFields `yaml:",inline"`
	Name         string   `yaml:"name"`
	Hosts        []string `yaml:"hosts"`
	Suffix       string   `yaml:"suffix"`
	DeviceType   string   `yaml:"device_type"`
}

// Common returns the embedded CommonFields.
func (g *GroupConfig) Common() *CommonFields { return &g.CommonFields }

// HostConfig represents a single entry in the "hosts" section.
type HostConfig struct {
	CommonFields `yaml:",inline"`
	Host         string   `yaml:"host"`
	Groups       []string `yaml:"groups"`
	DeviceType   string   `yaml:"device_type"`
}

// Common returns the embedded CommonFields.
func (h *HostConfig) Common() *CommonFields { return &h.CommonFields }

// ResolvedHost holds a host with all its associated group and device type configurations resolved.
type ResolvedHost struct {
	HostConfig   HostConfig
	GroupConfigs []GroupConfig
	DeviceConfig DeviceConfig
	DeviceType   string // Resolved device type name ("linux", "cisco_ios", etc.)
}
