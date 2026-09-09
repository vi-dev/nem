package clean

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

var ageRE = regexp.MustCompile(`^([0-9]+)([dh])$`)

const maxAge = 36500 * 24 * time.Hour

type Age struct {
	raw string
	d   time.Duration
}

func (a *Age) Duration() time.Duration { return a.d }

func (a *Age) String() string { return a.raw }

func (a *Age) Type() string { return "days|hours" }

func (a *Age) Set(s string) error {
	m := ageRE.FindStringSubmatch(s)
	if m == nil {
		return fmt.Errorf("invalid age %q: want whole days or hours, like 30d or 12h", s)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return fmt.Errorf("invalid age %q: want whole days or hours, like 30d or 12h", s)
	}
	if n == 0 {
		return fmt.Errorf("invalid age %q: must be greater than zero, like 30d or 12h", s)
	}
	unit := time.Hour
	if m[2] == "d" {
		unit = 24 * time.Hour
	}
	if n > int(maxAge/unit) {
		return fmt.Errorf("invalid age %q: must be no more than %dd, like 30d or 12h",
			s, int(maxAge/(24*time.Hour)))
	}
	a.raw, a.d = s, time.Duration(n)*unit
	return nil
}
