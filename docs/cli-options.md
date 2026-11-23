# CLI Options

Run `retri --help` for the options supported by the installed binary.

```text
Usage:
  retri                  Start a new local shell and record the session
  retri <hostname>       SSH to host and record the session
  retri [OPTIONS]        Execute commands and collect logs
  retri --credential-cache-init [-H host | -g group]
                        Initialize named SSH/sudo caches only (default: defaults section)
                        Requires cache.backend: session-keyring or user-keyring

  Note: In automated mode, <hostname> is ignored when -H, -g, --command,
        or -f is specified. Cache initialization accepts one target selector.


Application Options:
  -c, --config=           Config file path (default: ~/.config/retri/config.yaml)
  -H, --host=             Target single host
  -g, --group=            Target group
  -f, --command-file=     Command file path
      --command=          Single command to execute
  -d, --log-dir=          Log directory override (default: ~/retri-logs)
  -F, --filename-format=  Log filename format override (default: {host}_{timestamp}{suffix}.log)
  -t, --timestamp-format= Timestamp format override (default: YYYYMMDD_HHmmss)
  -S, --suffix=           Filename suffix override
      --log-encoding=     Terminal output encoding (default: raw)
      --log-commands-only Record only submitted commands and their output in session mode
  -P, --parallel=         Parallel execution count (default: 5 or config 'auto')
  -D, --debug             Enable debug output
  -T, --no-timestamp      Disable timestamp logging
  -p, --password=         SSH password literal override (may be visible in process arguments)
  -s, --secret=           sudo/enable secret literal override (may be visible in process arguments)
  -e, --exit-command=     Exit command for automated SSH sessions (default: exit)
  -i, --credential-cache-init
                          Initialize selected SSH/sudo credential caches and exit; no session logs or SSH (Linux keyring required)
      --credential-cache-refresh
                          Discard selected credential caches and obtain fresh values from their configured sources
      --credential-cache-clear
                          Clear this config's current credential cache entries and exit
      --completion=[bash|zsh|fish]
                          Generate shell completion script (bash, zsh, or fish)
  -C, --config-help       Show config file documentation
  -v, --version           Show version information
  -u, --update            Update retri to the latest version

Help Options:
  -h, --help              Show this help message
```

## Execution modes

| Invocation | Behavior |
| --- | --- |
| `retri` | Start a new login shell from `$SHELL` (fallback: `/bin/sh`) and record it. |
| `retri myserver` | Open an interactive SSH session and record it. |
| `retri -H myserver --command "uptime"` | Run the combined command list on one host. |
| `retri -g web` | Run configured commands on the group's hosts. |
| `retri --command "uptime"` | Run the combined command list on all entries in `hosts`. |
| `retri -i [-H myserver \| -g web \| myserver]` | Initialize selected named credential caches and exit. |

Any of `-H`, `-g`, `--command`, or `-f` selects automated execution. Commands in
the config alone do not select that mode. Hosts without commands are skipped.
In ordinary automated execution, `-H` takes precedence over `-g`; initialization
accepts at most one of `-H`, `-g`, or a positional hostname.

Commands are added, not replaced, in this order: `defaults`, applicable groups,
device type, host, CLI command file, CLI command. Within each config layer,
`command_file`, `commands`, and `command` are all included in that order.
Command files contain one command per line; whitespace is trimmed and blank
lines and lines beginning with `#` are ignored. A relative file path is tried
from the current directory, then from `~/.config/retri/commands/` if missing.

## Settings by mode

Automated execution resolves shared settings in this order (low to high):
`defaults` → `groups` → `device_types` → `hosts` → CLI overrides.
See the [configuration reference](config-reference.yaml) for field-specific rules.

Local and interactive SSH recording use logging settings from `defaults` and
the CLI. They do not apply host/group/device profiles or Retri's automated
credential acquisition. Interactive SSH uses the system SSH client's
authentication and `~/.ssh/config`. `-p`, `-s`, `-e`, `-P`, and
`--credential-cache-refresh` do not configure these recording sessions.
`--log-commands-only` applies only to recording sessions.

`--log-encoding` overrides the source encoding in all modes. `-T` disables
timestamp prefixes on terminal output lines; session headers, footers, and
automated `[EXEC]` markers never have those prefixes. Filename timestamps
are controlled separately by `-t`.

## Credential cache operations

`--credential-cache-refresh` reacquires cached named credentials selected for
runnable automated hosts, or for `--credential-cache-init`. It does not
refresh every definition in the config.

`--credential-cache-clear` removes cache entries for all currently defined
cached credentials in the selected config, then exits without contacting
providers or opening SSH. `-H` and `-g` do not narrow this operation. Removed
definitions and entries in a previously selected backend are not cleared.
Unlike initialization, clear goes through normal initial config/directory
creation if they are missing.

For initialization examples, required cache configuration, prompt controls and
side effects, see [Credential providers](credentials.md#disabling-password-prompts-and-preparing-the-cache).

## Updates

`--update` explicitly downloads and installs a newer `.deb` using
`sudo apt-get install -y`. The normal daily update check only prints a notice;
it does not install anything. See [Operations](operations.md#update) for
requirements and configuration.
