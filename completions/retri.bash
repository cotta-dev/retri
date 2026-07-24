# bash completion for retri
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
