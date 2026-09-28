package report

import "testing"

func TestPlural(t *testing.T) {
	cases := []struct {
		n    int
		noun string
		want string
	}{
		{0, "package", "0 packages"},
		{1, "package", "1 package"},
		{2, "tag", "2 tags"},
		{1, "project package", "1 project package"},
		{3, "project package", "3 project packages"},
	}
	for _, tc := range cases {
		if got := Plural(tc.n, tc.noun); got != tc.want {
			t.Errorf("Plural(%d, %q) = %q, want %q", tc.n, tc.noun, got, tc.want)
		}
	}
}
