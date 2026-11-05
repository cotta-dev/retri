# CLI Options

```
Usage:
  retri                  Start local work session recording
  retri <hostname>       SSH to host and record the session
  retri [OPTIONS]        Execute commands and collect logs
  retri --credential-cache-init [-H host | -g group]
                        Initialize named SSH/sudo caches only (default: defaults section)

  Note: <hostname> is ignored when -H, -g, --command, or -f is specified.


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
  -p, --password=         SSH Password (default: $RETRI_SSH_PASSWORD or config)
  -s, --secret=           Sudo Secret (default: $RETRI_SSH_SECRET or config)
  -e, --exit-command=     Exit command for interactive sessions (default: exit)
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

For initialization examples, required cache configuration, prompt controls and
side effects, see [Credential providers](credentials.md#disabling-password-prompts-and-preparing-the-cache).
