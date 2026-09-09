package shell

import (
	"strings"
	"testing"
)

func TestHookBlockBashRegistersPromptCommandHookWithoutClobbering(t *testing.T) {
	out := HookBlock(Bash)
	for _, want := range []string{"PROMPT_COMMAND", "__nem_hook", "${PROMPT_COMMAND:+", `eval "$(command nem env --shell bash)"`} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in PROMPT_COMMAND chaining, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "add-zsh-hook") {
		t.Errorf("bash block must not reference add-zsh-hook, got:\n%s", out)
	}
}

func TestHookBlockZshRegistersChpwdHook(t *testing.T) {
	out := HookBlock(Zsh)
	for _, want := range []string{"autoload -Uz add-zsh-hook", "add-zsh-hook chpwd __nem_hook", `eval "$(command nem env --shell zsh)"`} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in chpwd hook registration, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "PROMPT_COMMAND") {
		t.Errorf("zsh block must not reference PROMPT_COMMAND, got:\n%s", out)
	}
}

func TestHookBlockDoesNotContainMarkers(t *testing.T) {
	for _, d := range []Dialect{Bash, Zsh} {
		out := HookBlock(d)
		if strings.Contains(out, BeginMarker) || strings.Contains(out, EndMarker) {
			t.Errorf("HookBlock must not embed its own markers, got:\n%s", out)
		}
	}
}

func TestHookBlockUnknownDialectReturnsEmpty(t *testing.T) {
	if out := HookBlock(Fish); out != "" {
		t.Errorf("expected empty block for Fish, got:\n%s", out)
	}
	if out := HookBlock(Dialect(99)); out != "" {
		t.Errorf("expected empty block for an unrecognized dialect, got:\n%s", out)
	}
}
