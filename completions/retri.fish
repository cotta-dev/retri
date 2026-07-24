# fish completion for retri
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
