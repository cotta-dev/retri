package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type completionKind string

const (
	completionKindItem completionKind = "item"
	completionKindHint completionKind = "hint"
	completionFieldSep                = "\x1f"
)

type completionCandidate struct {
	Kind        completionKind
	Value       string
	Description string
	Display     string
}

type optionCompletion struct {
	Short       string
	Long        string
	Description string
	ValueKind   string
	ValueHint   string
	TakesValue  bool
}

var completionOptions = []optionCompletion{
	{Short: "-c", Long: "--config", Description: "Config file path (default: ~/.config/retri/config.yaml)", ValueKind: "file", ValueHint: "<filepath>", TakesValue: true},
	{Short: "-H", Long: "--host", Description: "Target single host", ValueKind: "host", TakesValue: true},
	{Short: "-g", Long: "--group", Description: "Target group from retri config", ValueKind: "group", ValueHint: "<group name>", TakesValue: true},
	{Short: "-f", Long: "--command-file", Description: "Command file path", ValueKind: "file", ValueHint: "<filepath>", TakesValue: true},
	{Long: "--command", Description: "Single command to execute", ValueKind: "command", ValueHint: "<command>", TakesValue: true},
	{Short: "-d", Long: "--log-dir", Description: "Log directory override (default: ~/retri-logs)", ValueKind: "dir", ValueHint: "<directory path>", TakesValue: true},
	{Short: "-F", Long: "--filename-format", Description: "Log filename format override (default: {host}_{timestamp}{suffix}.log)", ValueKind: "text", ValueHint: "<filename format>", TakesValue: true},
	{Short: "-t", Long: "--timestamp-format", Description: "Timestamp format override (default: YYYYMMDD_HHmmss)", ValueKind: "text", ValueHint: "<timestamp format>", TakesValue: true},
	{Short: "-S", Long: "--suffix", Description: "Filename suffix override", ValueKind: "text", ValueHint: "<filename suffix>", TakesValue: true},
	{Long: "--log-encoding", Description: "Terminal output encoding (default: raw)", ValueKind: "text", ValueHint: "<raw|utf-8|shift_jis|euc-jp|...>", TakesValue: true},
	{Long: "--log-commands-only", Description: "Record only submitted commands and their output in session mode"},
	{Short: "-P", Long: "--parallel", Description: "Parallel execution count (default: 5 or config 'auto')", ValueKind: "int", ValueHint: "<number>", TakesValue: true},
	{Short: "-D", Long: "--debug", Description: "Enable debug output"},
	{Short: "-T", Long: "--no-timestamp", Description: "Disable timestamp logging"},
	{Short: "-p", Long: "--password", Description: "SSH Password (default: $RETRI_SSH_PASSWORD or config)", ValueKind: "password", ValueHint: "<ssh password>", TakesValue: true},
	{Short: "-s", Long: "--secret", Description: "Sudo Secret (default: $RETRI_SSH_SECRET or config)", ValueKind: "password", ValueHint: "<sudo secret>", TakesValue: true},
	{Short: "-e", Long: "--exit-command", Description: "Exit command for interactive sessions (default: exit)", ValueKind: "command", ValueHint: "<exit command>", TakesValue: true},
	{Long: "--completion", Description: "Generate shell completion script (bash, zsh, or fish)", ValueKind: "shell", ValueHint: "<bash|zsh|fish>", TakesValue: true},
	{Short: "-C", Long: "--config-help", Description: "Show config file documentation"},
	{Short: "-v", Long: "--version", Description: "Show version information"},
	{Short: "-u", Long: "--update", Description: "Update retri to the latest version"},
	{Short: "-h", Long: "--help", Description: "Show this help message"},
}

func isCompletionRequest() bool {
	return os.Getenv("RETRI_COMPLETE") != ""
}

func runCompletion(args []string) {
	cword := len(args) - 1
	if raw := os.Getenv("RETRI_COMPLETE_CWORD"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			cword = parsed
		}
	}

	for _, candidate := range completionCandidates(args, cword) {
		fmt.Printf("%s%s%s%s%s%s%s\n",
			candidate.Kind,
			completionFieldSep,
			escapeCompletionField(candidate.Value),
			completionFieldSep,
			escapeCompletionField(candidate.Description),
			completionFieldSep,
			escapeCompletionField(candidate.Display))
	}
}

func completionCandidates(args []string, cword int) []completionCandidate {
	if len(args) == 0 {
		args = []string{""}
	}
	if cword < 0 || cword >= len(args) {
		cword = len(args) - 1
	}

	current := args[cword]
	previous := ""
	if cword > 0 {
		previous = args[cword-1]
	}

	if opt, ok := findOption(previous); ok && opt.TakesValue && !strings.Contains(previous, "=") && current != previous {
		return valueCandidates(opt, current, args, cword)
	}

	if opt, valuePrefix, ok := splitLongOptionValue(current); ok {
		candidates := valueCandidates(opt, valuePrefix, args, cword)
		for i := range candidates {
			if candidates[i].Kind == completionKindItem {
				candidates[i].Value = strings.SplitN(current, "=", 2)[0] + "=" + candidates[i].Value
			}
		}
		return candidates
	}

	if strings.HasPrefix(current, "-") {
		return optionCandidates(current)
	}

	if current == "" && cword == 0 {
		return optionCandidates(current)
	}

	return hostCandidates(current)
}

func optionCandidates(prefix string) []completionCandidate {
	var candidates []completionCandidate

	for _, opt := range completionOptions {
		value, ok := optionCompletionValue(opt, prefix)
		if !ok {
			continue
		}
		candidates = append(candidates, completionCandidate{
			Kind:        completionKindItem,
			Value:       value,
			Description: opt.Description,
			Display:     optionDisplay(opt),
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Value < candidates[j].Value
	})
	return candidates
}

func optionCompletionValue(opt optionCompletion, prefix string) (string, bool) {
	if strings.HasPrefix(prefix, "--") {
		if opt.Long != "" && strings.HasPrefix(opt.Long, prefix) {
			return opt.Long, true
		}
		return "", false
	}

	if prefix != "" {
		if opt.Short != "" && strings.HasPrefix(opt.Short, prefix) {
			return opt.Short, true
		}
		if opt.Long != "" && strings.HasPrefix(opt.Long, prefix) {
			return opt.Long, true
		}
		return "", false
	}

	if opt.Long != "" {
		return opt.Long, true
	}
	return opt.Short, opt.Short != ""
}

func optionDisplay(opt optionCompletion) string {
	switch {
	case opt.Short != "" && opt.Long != "":
		return opt.Short + ", " + opt.Long
	case opt.Long != "":
		return opt.Long
	default:
		return opt.Short
	}
}

func valueCandidates(opt optionCompletion, prefix string, _ []string, _ int) []completionCandidate {
	switch opt.ValueKind {
	case "host":
		return hostCandidates(prefix)
	default:
		hint := opt.ValueHint
		if hint == "" {
			hint = "<value>"
		}
		return []completionCandidate{{Kind: completionKindHint, Description: hint + "  " + opt.Description}}
	}
}

func findOption(name string) (optionCompletion, bool) {
	if name == "" {
		return optionCompletion{}, false
	}

	base := name
	if strings.HasPrefix(name, "--") {
		base = strings.SplitN(name, "=", 2)[0]
	}

	for _, opt := range completionOptions {
		if base == opt.Short || base == opt.Long {
			return opt, true
		}
	}
	return optionCompletion{}, false
}

func splitLongOptionValue(current string) (optionCompletion, string, bool) {
	if !strings.HasPrefix(current, "--") || !strings.Contains(current, "=") {
		return optionCompletion{}, "", false
	}

	parts := strings.SplitN(current, "=", 2)
	opt, ok := findOption(parts[0])
	if !ok || !opt.TakesValue {
		return optionCompletion{}, "", false
	}
	return opt, parts[1], true
}

func hostCandidates(prefix string) []completionCandidate {
	hosts := collectSSHHosts()
	candidates := make([]completionCandidate, 0, len(hosts))
	for _, host := range hosts {
		if strings.HasPrefix(host, prefix) {
			candidates = append(candidates, completionCandidate{
				Kind:        completionKindItem,
				Value:       host,
				Description: "SSH host",
			})
		}
	}
	return candidates
}

func collectSSHHosts() []string {
	seen := map[string]struct{}{}
	for _, host := range sshConfigHosts() {
		seen[host] = struct{}{}
	}
	for _, host := range knownHosts() {
		seen[host] = struct{}{}
	}

	hosts := make([]string, 0, len(seen))
	for host := range seen {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	return hosts
}

func sshConfigHosts() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return readSSHConfigHosts(filepath.Join(home, ".ssh", "config"), map[string]bool{})
}

func readSSHConfigHosts(path string, visited map[string]bool) []string {
	if visited[path] {
		return nil
	}
	visited[path] = true

	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = file.Close() }()

	var hosts []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		switch strings.ToLower(fields[0]) {
		case "host":
			for _, host := range fields[1:] {
				if isCompletableSSHHost(host) {
					hosts = append(hosts, host)
				}
			}
		case "include":
			for _, pattern := range fields[1:] {
				for _, includePath := range expandSSHIncludePath(path, pattern) {
					hosts = append(hosts, readSSHConfigHosts(includePath, visited)...)
				}
			}
		}
	}
	return hosts
}

func expandSSHIncludePath(configPath, pattern string) []string {
	if strings.HasPrefix(pattern, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			pattern = filepath.Join(home, strings.TrimPrefix(pattern, "~"))
		}
	} else if !filepath.IsAbs(pattern) {
		pattern = filepath.Join(filepath.Dir(configPath), pattern)
	}

	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return nil
	}
	return matches
}

func knownHosts() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	var hosts []string
	for _, name := range []string{"known_hosts", "known_hosts2"} {
		file, err := os.Open(filepath.Join(home, ".ssh", name))
		if err != nil {
			continue
		}
		hosts = append(hosts, readKnownHosts(file)...)
		_ = file.Close()
	}
	return hosts
}

func readKnownHosts(r io.Reader) []string {
	var hosts []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "|") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		for _, host := range strings.Split(fields[0], ",") {
			host = strings.TrimSpace(host)
			host = strings.TrimPrefix(host, "[")
			if idx := strings.Index(host, "]:"); idx >= 0 {
				host = host[:idx]
			} else if idx := strings.LastIndex(host, ":"); idx >= 0 {
				host = host[:idx]
			}
			if isCompletableSSHHost(host) {
				hosts = append(hosts, host)
			}
		}
	}
	return hosts
}

func isCompletableSSHHost(host string) bool {
	return host != "" && !strings.ContainsAny(host, "*?![]") && !strings.HasPrefix(host, "|")
}

func escapeCompletionField(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, completionFieldSep, "")
	value = strings.ReplaceAll(value, "\t", `\t`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return value
}

func writeCompletionScript(w io.Writer, shell string) error {
	switch shell {
	case "bash":
		_, err := io.WriteString(w, bashCompletionScript)
		return err
	case "zsh":
		_, err := io.WriteString(w, zshCompletionScript)
		return err
	case "fish":
		_, err := io.WriteString(w, fishCompletionScript)
		return err
	default:
		return fmt.Errorf("unsupported shell: %s", shell)
	}
}

const bashCompletionScript = `# bash completion for retri
_retri_completion() {
    local cword=$((COMP_CWORD - 1))
    local args=("${COMP_WORDS[@]:1}")
    if (( COMP_CWORD == ${#COMP_WORDS[@]} )); then
        args+=("")
    fi
    local line kind value desc label
    local -a values=()
    local -a display_labels=()
    local -a display_descs=()
    COMPREPLY=()

    _retri_redraw() {
        local prompt="${PS1@P}"
        if [[ -z "$prompt" ]]; then
            local dir="${PWD/#$HOME/~}"
            local sigil="$"
            [[ ${EUID:-$(id -u)} -eq 0 ]] && sigil="#"
            prompt="${USER:-$(id -un)}@${HOSTNAME%%.*}:${dir}${sigil} "
        fi
        printf '\n%s%s' "$prompt" "$COMP_LINE" >&2
    }

    while IFS=$'\x1f' read -r kind value desc label; do
        case "$kind" in
            item)
                values+=("$value")
                if [[ -n "$desc" ]]; then
                    [[ -n "$label" ]] || label="$value"
                    display_labels+=("$label")
                    display_descs+=("$desc")
                fi
                ;;
            hint)
                if [[ -n "$desc" ]]; then
                    display_labels+=("$desc")
                    display_descs+=("")
                fi
                ;;
        esac
    done < <(RETRI_COMPLETE=1 RETRI_COMPLETE_CWORD="$cword" "${COMP_WORDS[0]}" "${args[@]}")

    if [[ ${#display_labels[@]} -gt 0 && ${#values[@]} -ne 1 ]]; then
        local max=0 i
        for ((i = 0; i < ${#display_labels[@]}; i++)); do
            ((${#display_labels[i]} > max)) && max=${#display_labels[i]}
        done
        printf '\n' >&2
        for ((i = 0; i < ${#display_labels[@]}; i++)); do
            if [[ -n "${display_descs[i]}" ]]; then
                printf "%-*s  %s\n" "$max" "${display_labels[i]}" "${display_descs[i]}" >&2
            else
                printf "%s\n" "${display_labels[i]}" >&2
            fi
		done
		_retri_redraw
		COMPREPLY=()
    elif [[ ${#values[@]} -eq 1 ]]; then
        COMPREPLY=("${values[0]}")
    elif [[ ${#values[@]} -gt 1 ]]; then
        COMPREPLY=("${values[@]}")
    fi
    return 0
}
complete -o nosort -F _retri_completion retri
`

const zshCompletionScript = `#compdef retri

_retri() {
    local cword=$((CURRENT - 2))
    local -a args values
    args=("${words[@]:1}")
    values=()

    local line kind value desc label
    while IFS=$'\x1f' read -r kind value desc label; do
        case "$kind" in
            item)
                [[ -n "$label" ]] || label="$value"
                values+=("${value}:${label} ${desc}")
                ;;
            hint)
                _message "$desc"
                return
                ;;
        esac
    done < <(RETRI_COMPLETE=1 RETRI_COMPLETE_CWORD="$cword" "$words[1]" "${args[@]}")

    if (( ${#values[@]} )); then
        _describe -t retri-values 'retri completions' values
    fi
}

_retri "$@"
`

const fishCompletionScript = `# fish completion for retri
function __retri_complete
    set -l tokens (commandline -opc)
    set -l cword (math (count $tokens) - 2)
    if commandline -ct | string length -q
        set tokens $tokens (commandline -ct)
    else
        set tokens $tokens ""
        set cword (math $cword + 1)
    end
    env RETRI_COMPLETE=1 RETRI_COMPLETE_CWORD=$cword $tokens[1] $tokens[2..-1] | while read -l line
        set -l sep (printf "\x1f")
        set -l fields (string split $sep $line)
        switch $fields[1]
            case item
                set -l desc "$fields[3]"
                if test (count $fields) -ge 4; and test -n "$fields[4]"
                    set desc "$fields[4] $desc"
                end
                printf "%s\t%s\n" "$fields[2]" "$desc"
        end
    end
end

complete -c retri -f -a "(__retri_complete)"
`
