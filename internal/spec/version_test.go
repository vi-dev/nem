package spec

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{

		{"1.8.3", "1.8.2", 1},
		{"1.8.2", "1.8.2", 0},
		{"1.9", "1.10", -1},
		{"2.0", "10.0", -1},
		{"1.08", "1.8", 0},

		{"v1.2.4", "1.2.3", 1},
		{"v1.2.3", "1.2.3", 0},
		{"ver2", "1", 1},

		{"25.0.4_10", "25.0.4_7", 1},
		{"25.0.4_7", "25.0.4", 1},

		{"1.2.3.1", "1.2.3", 1},
		{"1.1.1a", "1.1.1", 1},
		{"1.1.1b", "1.1.1a", 1},

		{"1.2.3", "1.2.3-rc1", 1},
		{"1.2.3-rc10", "1.2.3-rc2", 1},
		{"1.2.3-beta", "1.2.3-alpha", 1},
		{"1.2.4-rc1", "1.2.3", 1},
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); sign(got) != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want sign %d", c.a, c.b, got, c.want)
		}
		if got := CompareVersions(c.b, c.a); sign(got) != -c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want sign %d (antisymmetry)", c.b, c.a, got, -c.want)
		}
	}
}

func sign(n int) int {
	switch {
	case n > 0:
		return 1
	case n < 0:
		return -1
	}
	return 0
}
