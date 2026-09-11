package discover

import (
	"context"
	"fmt"
	"regexp"

	"github.com/vi-dev/nem/internal/netx"
	"github.com/vi-dev/nem/internal/spec"
)

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

type Discovered struct {
	Version string
	Meta    map[string]string
}

func List(ctx context.Context, pkg *spec.Package) ([]Discovered, error) {
	d := pkg.VersionDiscovery
	switch {
	case d == nil:
		return nil, fmt.Errorf("%s has no versionDiscovery", pkg.Name)
	case d.GitHub != nil:
		return githubVersions(ctx, netx.Client(), d.GitHub)
	case d.GitLab != nil:
		return gitlabVersions(ctx, netx.Client(), d.GitLab)
	case d.Git != nil:
		return gitVersions(ctx, netx.Client(), d.Git.URL, d.Git.Filter, d.Git.Prefix, d.Git.Suffix)
	case d.HTTP != nil:
		return httpVersions(ctx, netx.Client(), d.HTTP)
	case d.OCI != "":
		tags, err := ociTags(ctx, d.OCI)
		if err != nil {
			return nil, err
		}
		out := make([]Discovered, len(tags))
		for i, t := range tags {
			out[i] = Discovered{Version: t}
		}
		return out, nil
	}
	return nil, fmt.Errorf("%s has no versionDiscovery source", pkg.Name)
}

func Latest(ctx context.Context, pkg *spec.Package) (string, error) {
	vs, err := List(ctx, pkg)
	if err != nil {
		return "", err
	}
	if len(vs) == 0 {
		return "", fmt.Errorf("%s: no versions discovered", pkg.Name)
	}
	best := vs[0].Version
	for _, v := range vs[1:] {
		if spec.CompareVersions(v.Version, best) > 0 {
			best = v.Version
		}
	}
	return best, nil
}

// versionGroupIndex reports the index of the named capture group "version",
// or -1 when the filter does not use named-capture semantics.
func versionGroupIndex(re *regexp.Regexp) int {
	for i, n := range re.SubexpNames() {
		if n == "version" {
			return i
		}
	}
	return -1
}

func metaFromMatch(re *regexp.Regexp, m []string) map[string]string {
	var meta map[string]string
	for i, n := range re.SubexpNames() {
		if i == 0 || i >= len(m) || n == "" || n == "version" || m[i] == "" {
			continue
		}
		if meta == nil {
			meta = map[string]string{}
		}
		meta[n] = m[i]
	}
	return meta
}
