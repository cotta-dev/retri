# Retri

[English](README.md) | [日本語](README.ja.md)

[![CI](https://github.com/cotta-dev/retri/actions/workflows/ci.yml/badge.svg)](https://github.com/cotta-dev/retri/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/cotta-dev/retri)](https://github.com/cotta-dev/retri/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

SSH Log Collector & Command Executor

The name **Retri** comes from **Retriever** — the dog breed famous for fetching things back. Just like a retriever, this tool goes out to your servers and brings back logs.

It was born from a desire to replicate the **TeraTerm log + macro workflow** in WSL (Windows Subsystem for Linux): define your commands in a config file, and Retri handles SSH connections, execution, and log saving automatically.

## Key Features

* **Local Session Recording**: Run without any arguments to start a new shell and record its terminal output — just like TeraTerm's log function.
* **SSH Session Recording**: Pass a hostname as an argument to SSH into a remote host and record the entire session automatically.
* **Automated Command Execution**: Execute commands across multiple hosts and save timestamped logs — equivalent to a TeraTerm macro.
* **Agentless**: Works with standard SSH. No software required on remote hosts.
* **Single Binary**: Release binaries are statically linked; SSH connections use the system's `ssh` client.
* **Network Device Support**: Use configurable prompts and setup commands for Cisco IOS, Arista EOS, Juniper, Huawei, and other interactive CLIs.
* **Output Encoding Control**: Preserve rendered terminal text bytes by default, or convert a known source encoding to UTF-8.
* **Parallel Execution**: Run commands on multiple servers concurrently (default: 5 hosts).
* **SSH Config Aware**: Uses `~/.ssh/config` for aliases, ProxyJump, identity files, and other SSH settings.

## Installation

Release packages target Linux (`amd64` and `arm64`), including WSL.
SSH modes require an OpenSSH client (`ssh` on `PATH`). Interactive local and
SSH recording require a terminal; local recording also requires a shell.

### Ubuntu/Debian (recommended)

In Bash, use `curl` to install the latest `.deb` for your architecture.

```bash
(
  set -e
  base=https://github.com/cotta-dev/retri/releases
  tag=$(curl -fsS -o /dev/null -w '%{redirect_url}' "$base/latest")
  tag=${tag##*/}
  arch=$(dpkg --print-architecture)
  deb=$(mktemp /tmp/retri.XXXXXX.deb)
  curl -gfsSLo "$deb" --proto =https "$base/download/$tag/retri_${tag#v}_$arch.deb"
  chmod 644 "$deb"
  sudo apt-get install -y "$deb"
)
```

Alternatively, download the `.deb` matching `dpkg --print-architecture` from the
[Releases page](https://github.com/cotta-dev/retri/releases/latest).
Replace `VERSION` and the architecture below with the downloaded filename:

```bash
sudo apt-get install ./retri_VERSION_amd64.deb
```

### Build from Source

Use a Go toolchain that meets the version requirement in [go.mod](go.mod).

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

Running `retri` without any arguments starts a new login shell using `$SHELL`
(or `/bin/sh` when unset) and records its terminal output. Exit that shell to
return to the shell that launched Retri.

```bash
retri
# → starts logging to ~/retri-logs/hostname_YYYYMMDD_HHmmss.log
# → type 'exit' or press Ctrl-D to stop recording
```

### SSH + Record Session (hostname as argument)

Pass a hostname to SSH into the remote host and record the entire interactive session.

```bash
retri myserver
# → SSHes to myserver and records the session to ~/retri-logs/myserver_YYYYMMDD_HHmmss.log
# → type 'exit' to disconnect and stop recording
```

This mode uses the SSH client's authentication and connection settings.
Retri applies `defaults` and logging flags to the log; per-host, group, and
device-type settings and Retri's automated credential handling are not applied.

### Automate Commands and Collect Logs

Run a command on a single host (using `~/.ssh/config` alias):

```bash
retri --host myserver --command "df -h"
```

Run commands on a group of servers defined in config:

```bash
retri --group web_servers
```

Read commands from a file (one command per line; blank lines and lines starting
with `#` are ignored):

```bash
retri --host myserver --command-file commands.txt
```

Command lists from the configuration and CLI are combined in order; `--command`
and `--command-file` add commands rather than replacing configured commands.
`--host`, `--group`, `--command`, or `--command-file` selects automated mode;
a positional hostname is ignored when any of these is supplied. Hosts with no
commands are skipped.

### Record Only Commands and Their Output

For local or interactive SSH recording, enable commands-only logging:

```bash
retri --log-commands-only
retri --log-commands-only myserver
```

Login banners, idle prompts, input redraws, and credential prompts are omitted;
an empty submission is retained as a prompt-only line. The terminal display is
unchanged. This also works with network device CLIs.

To enable it by default for recording sessions:

```yaml
defaults:
  log_commands_only: true
```

### Command Line Options

Run `retri --help` for the current options, or see the
[option reference](docs/cli-options.md).

### Shell Completion

Installing the `.deb` package automatically places the completion files for
bash, zsh, and fish. No manual generation or placement is needed. When the
shell's completion system is enabled, they are loaded in new shells.

For source builds or `go install`, generate and load a completion script manually:

```bash
# bash
source <(retri --completion bash)
```

For zsh, save the generated function on `fpath` and let
[`compinit`](https://zsh.sourceforge.io/Doc/Release/Completion-System.html)
register it:

```zsh
# zsh
mkdir -p ~/.zsh/completions
retri --completion zsh > ~/.zsh/completions/_retri
fpath=(~/.zsh/completions $fpath)
autoload -Uz compinit
compinit
```

Keep the `fpath` line before your existing `compinit` call in `~/.zshrc` to
enable it in future shells.

```fish
# fish
retri --completion fish | source
```

Completion includes option descriptions, value hints for options, and SSH host
aliases from `~/.ssh/config` / `known_hosts` for `retri <hostname>`.

## Configuration

On the first normal run, Retri creates `~/.config/retri/config.yaml` and
`~/.config/retri/commands/` if missing. Use `--config PATH` to load another
configuration file. `--help`, `--version`, `--config-help`, and completion
generation do not create these files.

### Example `config.yaml`

This minimal configuration defines the group used above. Replace `myserver`
with your SSH hostname or alias:

```yaml
defaults:
  log_dir: "~/retri-logs"

groups:
  - name: web_servers
    hosts: [myserver]
    commands:
      - "df -h"
      - "uptime"
```

For automated execution, shared settings are resolved in this order (low to
high): `defaults` → `groups` → `device_types` → `hosts` → CLI flags.
Commands are aggregated across those layers. See `retri --config-help` or
the [complete configuration reference](docs/config-reference.yaml) for
field-specific rules, device profiles, and host ranges such as `switch-[01-04]`.

### Environment Variables & Security

Optional [named credential sources and caching](docs/credentials.md)
support shared prompts, environment variables, Linux keyrings, and Bitwarden.
Bitwarden and Linux keyrings are not required for ordinary use.
See the [cache backend comparison](docs/credentials.md#cache-backend-comparison)
to choose between `user-keyring`, `session-keyring`, and `none`.

For automated execution, prefer SSH keys or named credentials over plaintext
passwords. Legacy `password` / `secret` fields also support `${VAR}` expansion
(set the variable before running Retri):

```yaml
defaults:
  password: "${COMMON_SSH_PASSWORD}"
```

Fallback environment variables (lowest priority):

| Variable | Description |
| :--- | :--- |
| `RETRI_SSH_PASSWORD` | SSH password fallback for automated execution when no credential is configured. |
| `RETRI_SSH_SECRET` | sudo / enable secret fallback for automated execution when no credential is configured. |
| `RETRI_LOG_ENCODING` | Source encoding used when `log_encoding` is not configured. |

Automated execution masks literal matches for the SSH password and sudo /
enable secret in received terminal output before rendering it. Keep secrets
out of commands and their output. See the limits in
[Credential providers](docs/credentials.md#security-boundaries).

## Output Format

Logs are saved to `~/retri-logs` by default; use `--log-dir` to change it.
New log directories are created with mode `0700` and log files with mode `0600`.
Existing log files are never overwritten. Generated log names must be single
local filenames without path separators or control characters.

Terminal output lines have millisecond timestamps by default. Retri's session
header, footer, `[EXEC]` markers, and marker separator lines have no timestamp
prefix. Use `--no-timestamp` or `defaults.timestamp: false` to disable output
line timestamps.

Example automated Linux log (the prompt, echo, and output depend on the host):

File: `example-host_20000101_000000.log`

```text
============================================================
 TARGET HOST : example-host
 DEVICE TYPE : linux
 START TIME  : 2000-01-01 00:00:00
============================================================

----------------------------------------
[EXEC] unset HISTFILE
----------------------------------------
[2000-01-01 00:00:00.100] operator@example-host:~$ unset HISTFILE

----------------------------------------
[EXEC] uptime
----------------------------------------
[2000-01-01 00:00:00.200] operator@example-host:~$ uptime
[2000-01-01 00:00:00.201]  00:00:00 up 1 day,  1 user,  load average: 0.00, 0.00, 0.00

----------------------------------------
[EXEC] whoami
----------------------------------------
[2000-01-01 00:00:00.300] operator@example-host:~$ whoami
[2000-01-01 00:00:00.301] operator
[2000-01-01 00:00:00.802] operator@example-host:~$ exit
[2000-01-01 00:00:00.803] logout

============================================================
 LOG END     : 2000-01-01 00:00:00
============================================================
```

Automated Linux hosts and network devices both use a real interactive SSH PTY.
The shell/CLI prompt, command echo, and output therefore come from the same
remote terminal session; Retri does not derive a Linux prompt from user,
hostname, or directory values.

Before an automated batch with `device_type: linux` (the default), Retri sends
and records `unset HISTFILE` to suppress persistent history in shells such as
Bash. History behavior depends on the remote shell and its configuration.
Interactive recording modes and network-device CLI history are not changed.

`[EXEC]` marks commands sent during automated execution, including
`setup_commands` and `unset HISTFILE`. The final exit command has no `[EXEC]`
marker. When a prompt is detected, it stays buffered for the terminal's command
echo after the marker. Retri does not synthesize a prompt or command echo that
the remote terminal did not display.

Local and interactive SSH recordings use `SESSION LOG` in the header instead
of `TARGET HOST` / `DEVICE TYPE` and do not add `[EXEC]` markers.

When Retri successfully sends the configured exit command and the session
closes, a non-zero remote shell status inherited from the last command does not
turn the whole session into an SSH failure. SSH transport status 255 and
signal-based termination remain failures.

### Terminal Rendering and Character Encoding

Retri renders ANSI controls, carriage-return redraws, and cursor edits before
writing lines. Tabs are expanded, trailing whitespace is trimmed, and repeated
blank lines are collapsed. Logs are terminal-rendered text, not byte-for-byte
captures of the PTY stream.

The default `log_encoding: raw` preserves the surviving source bytes without
character conversion. To convert a known source encoding to UTF-8, set:

```yaml
defaults:
  log_encoding: shift_jis
```

For automated execution, this can also be set on a group, device type, or host.
Local and interactive SSH recording use `defaults.log_encoding`.
`--log-encoding` overrides the setting in all modes.

Supported values are `raw`, `utf-8`, `shift_jis` (including CP932 /
Windows-31J), `euc-jp`, `iso-8859-1`, `windows-1252`, `gb18030`, `gbk`, `big5`,
and `euc-kr`. If a rendered line is malformed for the configured encoding,
Retri warns once per session and preserves that line's source bytes in the
same `.log`. It does not create a `.raw` sidecar; fallback lines can make the
log contain mixed encodings.

## License

Distributed under the MIT License. See LICENSE for more information.
