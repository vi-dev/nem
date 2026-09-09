package spec

import (
	"fmt"
	"regexp"
	"strings"
)

var NameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

type Ref struct{ Name, Version string }

func ParseRef(s string) (Ref, error) {
	name, version, found := strings.Cut(s, "@")
	if !NameRE.MatchString(name) {
		return Ref{}, fmt.Errorf("invalid package name %q", name)
	}
	if found && version == "" {
		return Ref{}, fmt.Errorf("invalid ref %q: empty version after @", s)
	}
	return Ref{Name: name, Version: version}, nil
}

func (r Ref) String() string {
	if r.Version == "" {
		return r.Name
	}
	return r.Name + "@" + r.Version
}
