package clean

import (
	"strings"
	"testing"
	"time"
)

func TestAgeSetValid(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"30d", 30 * 24 * time.Hour},
		{"12h", 12 * time.Hour},
		{"36500d", 36500 * 24 * time.Hour},
		{"876000h", 876000 * time.Hour},
	}
	for _, c := range cases {
		var a Age
		if err := a.Set(c.in); err != nil {
			t.Errorf("Set(%q): unexpected error %v", c.in, err)
			continue
		}
		if a.Duration() != c.want {
			t.Errorf("Set(%q).Duration() = %v, want %v", c.in, a.Duration(), c.want)
		}
	}
}

func TestAgeSetInvalid(t *testing.T) {
	for _, in := range []string{"30", "30m", "1d6h", "0d"} {
		var a Age
		err := a.Set(in)
		if err == nil {
			t.Errorf("Set(%q) should have failed", in)
			continue
		}
		if !strings.Contains(err.Error(), "30d") {
			t.Errorf("Set(%q) error must name the accepted shape, got %q", in, err)
		}
	}
}

func TestAgeSetRejectsOverflow(t *testing.T) {
	for _, in := range []string{"36501d", "876001h"} {
		var a Age
		err := a.Set(in)
		if err == nil {
			t.Errorf("Set(%q) = %v, want an error", in, a.Duration())
			continue
		}
		if !strings.Contains(err.Error(), "30d") {
			t.Errorf("Set(%q) error must name the accepted shape, got %q", in, err)
		}
		if a.Duration() != 0 {
			t.Errorf("Set(%q) left Duration() = %v; a rejected value must set nothing", in, a.Duration())
		}
	}
}

func TestAgeStringRoundTrips(t *testing.T) {
	var a Age
	if got := a.String(); got != "" {
		t.Fatalf("zero Age String() = %q, want empty", got)
	}
	if err := a.Set("30d"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := a.String(); got != "30d" {
		t.Fatalf("String() = %q, want %q", got, "30d")
	}
}
