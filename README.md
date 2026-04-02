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

### SSH + Record Session (hostname as argument)

Pass a hostname to SSH into the remote host and record the entire interactive session.

```bash
retri myserver
# → SSHes to myserver and records the session to ~/retri-logs/myserver_YYYYMMDD_HHmmss.log
# → type 'exit' to disconnect and stop recording
```

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

## Output Format

Logs are saved to `~/retri-logs` by default.

File: `myserver_20251129_120000.log`
```text
============================================================
 TARGET HOST : myserver
 DEVICE TYPE : linux
 START TIME  : 2025-11-29 12:00:00
============================================================

[2025-11-29 12:00:01.123] --- EXEC: df -h ---
[2025-11-29 12:00:01.150] Filesystem      Size  Used Avail Use% Mounted on
[2025-11-29 12:00:01.150] /dev/sda1        50G   10G   40G  20% /

[2025-11-29 12:00:01.155] --- EXEC: uptime ---
[2025-11-29 12:00:01.160]  12:00:01 up 10 days,  4:20,  1 user,  load average: 0.05, 0.03, 0.01

============================================================
 LOG END     : 2025-11-29 12:00:02
============================================================
```

## Release

```bash
git tag v0.1.0
git push origin v0.1.0
```

## License
Distributed under the MIT License. See LICENSE for more information.
