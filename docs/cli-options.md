# CLI Options

```
Usage:
  retri [OPTIONS]

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
  -P, --parallel=         Parallel execution count (default: 5 or config 'auto')
  -D, --debug             Enable debug output
  -T, --no-timestamp      Disable timestamp logging
  -p, --password=         SSH Password (default: $RETRI_SSH_PASSWORD or config)
  -s, --secret=           Sudo Secret (default: $RETRI_SSH_SECRET or config)
  -e, --exit-command=     Exit command for interactive sessions (default: exit)
  -C, --config-help       Show config file documentation
  -v, --version           Show version information

Help Options:
  -h, --help              Show this help message
```
