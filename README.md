# Retri

[English](README.md) | [日本語](README.ja.md)

[![CI](https://github.com/cotta-dev/retri/actions/workflows/ci.yml/badge.svg)](https://github.com/cotta-dev/retri/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/cotta-dev/retri)](https://github.com/cotta-dev/retri/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Universal SSH Log Collector & Command Executor

`Retri` is a lightweight, dependency-free CLI tool written in Go designed to automate command execution and log collection across multiple servers via SSH. It bridges the gap between simple shell scripts and complex configuration management tools like Ansible.

It is specifically designed to handle interactive prompts (like sudo passwords or Cisco enable secrets) automatically using pseudo-terminals (pty), ensuring logs are captured exactly as if a human typed them.

## Key Features

* **Agentless**: Works with standard SSH. No software required on remote hosts.
* **Dependency Free**: Single binary (statically linked).
* **Smart Interactive Mode**: Automatically detects and handles password/sudo prompts even for network devices (Cisco, Juniper, etc.) without exposing passwords in logs.
* **Environment Variable Support**: Securely manage credentials using `${VAR}` expansion in configuration files.
* **Parallel Execution**: Run commands on dozens of servers concurrently with controlled concurrency.
* **Real-time Logging**: Captures output with millisecond-precision timestamps.
* **Config Aware**: Fully supports `~/.ssh/config` (aliases, proxy jumps, identity files).

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

### Basic Usage

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
