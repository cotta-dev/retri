# Retri

[English](README.md) | [日本語](README.ja.md)

[![CI](https://github.com/cotta-dev/retri/actions/workflows/ci.yml/badge.svg)](https://github.com/cotta-dev/retri/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/cotta-dev/retri)](https://github.com/cotta-dev/retri/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

SSH Log Collector & Command Executor

The name **Retri** comes from **Retriever** — the dog breed famous for fetching things back. Just like a retriever, this tool goes out to your servers and brings back logs.

It was born from a desire to replicate the **TeraTerm log + macro workflow** in WSL (Windows Subsystem for Linux): define your commands in a config file, and Retri handles SSH connections, execution, and log saving automatically.

## Key Features

* **Local Session Recording**: Run without any arguments to record your current shell session to a log file — just like TeraTerm's log function.
* **SSH Session Recording**: Pass a hostname as an argument to SSH into a remote host and record the entire session automatically.
* **Automated Command Execution**: Execute commands across multiple hosts and save timestamped logs — equivalent to a TeraTerm macro.
* **Agentless**: Works with standard SSH. No software required on remote hosts.
* **Dependency Free**: Single binary (statically linked). Download and run.
* **Network Device Support**: Handles interactive PTY sessions for Cisco IOS, Arista EOS, Juniper, Huawei, etc. Passwords are never written to logs.
* **Original Output Bytes Preserved**: Printable terminal output is written without guessing or converting its character encoding. ASCII/ESC terminal controls are rendered separately.
* **Parallel Execution**: Run commands on multiple servers concurrently.
* **SSH Config Aware**: Fully supports `~/.ssh/config` (aliases, proxy jumps, identity files).

## Installation

### Ubuntu/Debian (recommended)

```bash
curl -fsSL $(curl -fsSL https://api.github.com/repos/cotta-dev/retri/releases/latest \
  | grep browser_download_url | grep "$(dpkg --print-architecture).deb" | cut -d'"' -f4) \
  -o /tmp/retri.deb && sudo apt-get install -y /tmp/retri.deb
```

Or download the `.deb` manually from the [Releases page](https://github.com/cotta-dev/retri/releases):

```bash
cp retri_VERSION_amd64.deb /tmp/
sudo apt-get install -y /tmp/retri_VERSION_amd64.deb
```

### Build from Source

```bash
git clone https://github.com/cotta-dev/retri.git
cd retri
CGO_ENABLED=0 go build -o retri -ldflags="-s -w" .
```

### Install with Go

```bash
CGO_ENABLED=0 go install github.com/cotta-dev/retri@latest
```

## Usage

### Record a Local Work Session (no arguments)

Running `retri` without any arguments starts recording your current shell session to a log file.

```bash
retri
# → starts logging to ~/retri-logs/hostname_YYYYMMDD_HHmmss.log
# → type 'exit' or press Ctrl-D to stop recording
```

To keep only submitted command lines and their output, enable commands-only
logging. Login banners, idle prompts, input redraws, and credential prompts are
omitted; an empty submission is retained as a prompt-only line. The terminal
display is unchanged.

```bash
retri --log-commands-only
```

It can also be enabled by default for local and SSH session recording:

```yaml
defaults:
  log_commands_only: true
```

### SSH + Record Session (hostname as argument)

Pass a hostname to SSH into the remote host and record the entire interactive session.

```bash
retri myserver
# → SSHes to myserver and records the session to ~/retri-logs/myserver_YYYYMMDD_HHmmss.log
# → type 'exit' to disconnect and stop recording
```

Commands-only logging works for interactive SSH sessions, including network
device CLIs:

```bash
retri --log-commands-only myserver
```

Retri does not guess the device encoding. It first renders terminal operations
such as ANSI controls, carriage-return redraws, and cursor edits. The default
`log_encoding: raw` then writes the surviving source bytes of each rendered
line without character decoding, replacement, or re-encoding. It is the
highest-fidelity choice for evidentiary logs, but it is not a byte-for-byte
capture of the raw PTY stream: terminal controls and overwritten text are
removed, tabs are expanded, and trailing whitespace and repeated blank lines
are normalized. To normalize a known device encoding to UTF-8 for readability,
set it on a device type, group, host, or defaults:

```yaml
device_types:
  cisco_ios_jp:
    log_encoding: shift_jis
    prompt_regex: "[#>] ?$"
```

Supported values are `raw`, `utf-8`, `shift_jis` (including CP932 /
Windows-31J), `euc-jp`, `iso-8859-1`, `windows-1252`, `gb18030`, `gbk`, `big5`,
and `euc-kr`. `--log-encoding` overrides the configuration. If a rendered line
is malformed for the configured encoding, Retri warns once and keeps that
line's rendered source bytes in the same `.log`; it never creates a `.raw`
sidecar. Such a fallback can make that `.log` contain mixed encodings.

New log files are private (`0600`) and Retri refuses to overwrite an existing
log path. A generated log filename must be a single local filename; path
separators and control characters are rejected.

### Automate Commands and Collect Logs

Run a command on a single host (using `~/.ssh/config` alias):
```bash
retri --host myserver --command "df -h"
```

Run commands on a group of servers defined in config:
```bash
retri --group web_servers
```

### Command Line Options

See [docs/cli-options.md](docs/cli-options.md) for the full option reference.

### Shell Completion

The `.deb` package installs shell completion automatically for bash, zsh, and fish.

For source builds or `go install`, generate and load a completion script manually:

```bash
# bash
source <(retri --completion bash)

# zsh
source <(retri --completion zsh)

# fish
retri --completion fish | source
```

Completion includes option descriptions, value hints for options, and SSH host
aliases from `~/.ssh/config` / `known_hosts` for `retri <hostname>`.

## Configuration

On the first run, retri automatically creates a default configuration file at `~/.config/retri/config.yaml`.

### Example `config.yaml`

See [docs/config-reference.yaml](docs/config-reference.yaml) for the complete parameter reference of each section.

### Environment Variables & Security

Avoid hardcoding passwords in the config file. Retri supports `${VAR}` expansion:

```bash
export COMMON_SSH_PASSWORD="my_secret_password"
```

```yaml
defaults:
  password: "${COMMON_SSH_PASSWORD}"
```

Fallback environment variables (lowest priority):

| Variable | Description |
| :--- | :--- |
| `RETRI_SSH_PASSWORD` | Default SSH password if no other config is found. |
| `RETRI_SSH_SECRET` | Default sudo secret if no other config is found. |
| `RETRI_LOG_ENCODING` | Source encoding used when `log_encoding` is not configured. |

## Output Format

Logs are saved to `~/retri-logs` by default.

File: `example-host_20000101_000000.log`
```text
============================================================
 TARGET HOST : example-host
 DEVICE TYPE : linux
 START TIME  : 2000-01-01 00:00:00
============================================================
[2000-01-01 00:00:00.100]
[2000-01-01 00:00:00.101] ----------------------------------------
[2000-01-01 00:00:00.101] [EXEC] uptime
[2000-01-01 00:00:00.101] ----------------------------------------
[2000-01-01 00:00:00.102] operator@example-host:~$ uptime
[2000-01-01 00:00:00.103]  00:00:00 up 1 day,  0 user,  load average: 0.00, 0.00, 0.00
[2000-01-01 00:00:00.104]
[2000-01-01 00:00:00.104] ----------------------------------------
[2000-01-01 00:00:00.104] [EXEC] sudo whoami
[2000-01-01 00:00:00.104] ----------------------------------------
[2000-01-01 00:00:00.105] operator@example-host:~$ sudo whoami
[2000-01-01 00:00:00.105] root

============================================================
 LOG END     : 2000-01-01 00:00:00
============================================================
```

Automated Linux hosts and network devices both use a real interactive SSH PTY.
The shell/CLI prompt, command echo, and output therefore come from the same
remote terminal session; Retri does not derive a Linux prompt from user,
hostname, or directory values.

The `[EXEC]` marker is followed by the terminal transcript's
`prompt + command` line. An idle prompt is not written separately before the
marker. Retri records the terminal-rendered result of the actual PTY stream; it
does not merge a separately received prompt with a command, nor does it create
a command line when the remote terminal sends no echo:

```text
----------------------------------------
[EXEC] nv con diff
----------------------------------------
[2000-01-01 00:00:00.100] operator@example-switch:mgmt:~$ nv con diff
```

When Retri successfully sends the configured exit command and the session
closes, a non-zero remote shell status inherited from the last command does not
turn the whole session into an SSH failure. SSH transport status 255 and
signal-based termination remain failures.

## License
Distributed under the MIT License. See LICENSE for more information.
