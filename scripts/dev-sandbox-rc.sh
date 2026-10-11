# Dev Sandbox Shell Configuration
# Sets a [SANDBOX] badge in the prompt and the terminal title.

# Skip if not in a container
[ -f /.dockerenv ] || return 0

[ -n "$__sandbox_configured" ] && return 0
__sandbox_configured=1

if [ -n "$ZSH_VERSION" ]; then
    __sandbox_set_title() {
        printf '\033]0;[SANDBOX] distributed-task-queue — %s\007' "${PWD:t}"
    }
    if [[ $- == *i* ]]; then
        autoload -U colors && colors
        PROMPT='%F{red}[SANDBOX]%f %F{blue}%~%f $ '
        (( ${precmd_functions[(Ie)__sandbox_set_title]} )) || precmd_functions+=(__sandbox_set_title)
    else
        __sandbox_set_title
    fi
    return
fi

__sandbox_set_title() {
    printf '\033]0;[SANDBOX] distributed-task-queue — %s\007' "${PWD##*/}"
}

if [[ $- == *i* ]]; then
    export PS1='\[\e[1;31m\][SANDBOX]\[\e[0m\] \[\e[34m\]\w\[\e[0m\] \$ '
    PROMPT_COMMAND="__sandbox_set_title${PROMPT_COMMAND:+;$PROMPT_COMMAND}"
else
    __sandbox_set_title
fi
