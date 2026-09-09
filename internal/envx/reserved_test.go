package envx

import "testing"

func TestIsReserved(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{

		{"PATH uppercase", "PATH", true},
		{"path lowercase", "path", true},

		{"NEM_HOME", "NEM_HOME", true},

		{"LD_PRELOAD", "LD_PRELOAD", true},

		{"DYLD_LIBRARY_PATH", "DYLD_LIBRARY_PATH", true},

		{"FOO_SET", "FOO_SET", true},

		{"HOME", "HOME", false},
		{"FOO", "FOO", false},
		{"my_var", "my_var", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsReserved(tt.input)
			if result != tt.expected {
				t.Errorf("IsReserved(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}
