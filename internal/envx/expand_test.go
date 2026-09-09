package envx

import "testing"

func TestExpand(t *testing.T) {

	lookup := func(name string) (string, bool) {
		vars := map[string]string{
			"HOME":  "/home/user",
			"USER":  "alice",
			"A":     "hello",
			"B":     "world",
			"EMPTY": "",
		}
		val, ok := vars[name]
		return val, ok
	}

	tests := []struct {
		name   string
		input  string
		lookup func(name string) (string, bool)
		want   string
	}{

		{"dollar_name", "$HOME/x", lookup, "/home/user/x"},

		{"brace_name", "${HOME}", lookup, "/home/user"},

		{"double_dollar", "$$", lookup, "$"},

		{"dollar_paren", "$(foo)", lookup, "$(foo)"},

		{"trailing_dollar", "text$", lookup, "text$"},

		{"adjacent_brace", "${A}${B}", lookup, "helloworld"},

		{"unterminated_brace", "${A", lookup, "${A"},

		{"empty_var", "$EMPTY", lookup, ""},
		{"empty_brace", "${EMPTY}", lookup, ""},

		{"mixed_1", "$HOME/$USER", lookup, "/home/user/alice"},
		{"mixed_2", "prefix_${A}_suffix", lookup, "prefix_hello_suffix"},
		{"no_vars", "no variables here", lookup, "no variables here"},

		{"unset_dollar", "$STALE", func(name string) (string, bool) {
			if name == "STALE" {
				return "stale_value", false
			}
			return "", false
		}, ""},
		{"unset_brace", "${STALE}", func(name string) (string, bool) {
			if name == "STALE" {
				return "stale_value", false
			}
			return "", false
		}, ""},

		{"invalid_braced_digit_first", "${1A}", lookup, "${1A}"},
		{"invalid_braced_space", "${A B}", lookup, "${A B}"},
		{"invalid_braced_empty", "${}", lookup, "${}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Expand(tt.input, tt.lookup)
			if result != tt.want {
				t.Errorf("Expand(%q, lookup) = %q, want %q", tt.input, result, tt.want)
			}
		})
	}
}
