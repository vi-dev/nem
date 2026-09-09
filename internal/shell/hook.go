package shell

import "fmt"

const (
	BeginMarker = "# >>> nem >>>"

	EndMarker = "# <<< nem <<<"
)

const hookBlockBody = `export NEM_ORIGINAL_PATH="${NEM_ORIGINAL_PATH-$PATH}"

nem() {
  command nem "$@"
  local __nem_rc=$?
  case "$1" in
    use|unuse|lock|sync)
      eval "$(command nem env --shell %[1]s)"
      ;;
  esac
  return $__nem_rc
}

__nem_hook() {
  eval "$(command nem env --shell %[1]s)"
}

%[2]s

__nem_hook

source <(command nem completion %[1]s 2>/dev/null) 2>/dev/null || true
`

const bashChpwdHook = `case "$PROMPT_COMMAND" in
  *__nem_hook*) ;;
  *) PROMPT_COMMAND="${PROMPT_COMMAND:+$PROMPT_COMMAND$'\n'}__nem_hook" ;;
esac`

const zshChpwdHook = `autoload -Uz add-zsh-hook
add-zsh-hook chpwd __nem_hook`

func HookBlock(d Dialect) string {
	switch d {
	case Bash:
		return fmt.Sprintf(hookBlockBody, "bash", bashChpwdHook)
	case Zsh:
		return fmt.Sprintf(hookBlockBody, "zsh", zshChpwdHook)
	default:
		return ""
	}
}
