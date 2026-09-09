package spec

import (
	"fmt"
	"runtime"
	"slices"
	"strings"
)

type Platform struct{ OS, Arch string }

var SupportedPlatforms = []Platform{
	{"darwin", "arm64"}, {"darwin", "amd64"},
	{"linux", "arm64"}, {"linux", "amd64"},
}

func (p Platform) String() string {
	if p.Arch == "" {
		return p.OS
	}
	return p.OS + "/" + p.Arch
}

func (p Platform) Matches(full Platform) bool {
	return p.OS == full.OS && (p.Arch == "" || p.Arch == full.Arch)
}

func Current() Platform { return Platform{OS: runtime.GOOS, Arch: runtime.GOARCH} }

func PlatformsInclude(list []Platform, p Platform) bool {
	if len(list) == 0 {
		return true
	}
	for _, c := range list {
		if c.Matches(p) {
			return true
		}
	}
	return false
}

func ParsePlatform(s string) (Platform, error) {
	osPart, arch, _ := strings.Cut(s, "/")
	p := Platform{OS: osPart, Arch: arch}
	if slices.ContainsFunc(SupportedPlatforms, p.Matches) {
		return p, nil
	}
	return Platform{}, fmt.Errorf("unsupported platform %q", s)
}
