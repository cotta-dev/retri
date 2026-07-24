#compdef retri

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
