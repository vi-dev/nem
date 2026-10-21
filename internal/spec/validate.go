package spec

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var EnvNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var CompatRE = regexp.MustCompile(`^\d+(\.\d+)*$`)

var TagRE = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}$`)

var metaKeyRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

func validateHTTPURL(s string) error {
	u, err := url.Parse(s)
	if err != nil {
		return err
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("must be an http(s) URL, got %q", s)
	}
	return nil
}

func (p *Package) Validate() error {
	if p.Schema < MinSchema || p.Schema > MaxSchema {
		return fmt.Errorf("schema must be %d to %d, got %d", MinSchema, MaxSchema, p.Schema)
	}
	if !NameRE.MatchString(p.Name) {
		return fmt.Errorf("invalid name %q", p.Name)
	}
	if len(p.Versions) == 0 {
		return errors.New("versions is required and must be non-empty")
	}
	for i, v := range p.Versions {
		if v.Version == "" {
			return fmt.Errorf("versions[%d]: missing version", i)
		}
		if !TagRE.MatchString(v.Version) {
			return fmt.Errorf("versions[%d] (%s): version is not a valid oci tag (it names the package's archive tag)", i, v.Version)
		}
		if err := p.validateExclusion(v); err != nil {
			return fmt.Errorf("versions[%d] (%s): %w", i, v.Version, err)
		}
		for k, val := range v.Meta {
			if !metaKeyRE.MatchString(k) {
				return fmt.Errorf("versions[%d] (%s): invalid meta key %q", i, v.Version, k)
			}
			if val == "" {
				return fmt.Errorf("versions[%d] (%s): meta %s must not be empty", i, v.Version, k)
			}
		}
	}
	if p.Latest() == "" {
		return errors.New("every version is excluded; remove the package instead")
	}
	if err := p.Base.validate(p.coveredBy(&p.Base)); err != nil {
		return err
	}
	for i := range p.Frozen {
		f := &p.Frozen[i]
		if err := f.validate(p.coveredBy(f)); err != nil {
			return fmt.Errorf("%s: %w", RecipeFileName(f.Prefix), err)
		}
	}
	return p.validateDiscovery()
}

func (p *Package) coveredBy(r *Recipe) []VersionEntry {
	var out []VersionEntry
	for _, v := range p.Offered() {
		if p.selectRecipe(v.Version) == r {
			out = append(out, v)
		}
	}
	return out
}

const maxExclusionReason = 300

func (p *Package) validateExclusion(v VersionEntry) error {
	ex := v.Excluded
	if ex == nil {
		return nil
	}
	if p.Schema < SchemaExcluded {
		return fmt.Errorf("excluded requires schema %d", SchemaExcluded)
	}
	reason := strings.TrimSpace(ex.Reason)
	switch {
	case reason == "":
		return errors.New("excluded requires a reason")
	case strings.ContainsAny(reason, "\r\n"):
		return errors.New("excluded reason must be a single line")
	case len(reason) > maxExclusionReason:
		return fmt.Errorf("excluded reason is %d characters; the limit is %d", len(reason), maxExclusionReason)
	}
	if ex.Use == "" {
		return nil
	}
	if ex.Use == v.Version {
		return errors.New("excluded use must name another version")
	}
	for _, e := range p.Versions {
		if e.Version != ex.Use {
			continue
		}
		if e.Excluded != nil {
			return fmt.Errorf("excluded use %s is itself excluded", ex.Use)
		}
		return nil
	}
	return fmt.Errorf("excluded use %s is not a listed version", ex.Use)
}

func (p *Package) validateDiscovery() error {
	if d := p.VersionDiscovery; d != nil {
		sources, filter := 0, ""
		if d.GitHub != nil {
			sources++
			filter = d.GitHub.Filter
			if d.GitHub.Repo == "" {
				return errors.New("versionDiscovery github requires repo")
			}
		}
		if d.GitLab != nil {
			sources++
			filter = d.GitLab.Filter
			if d.GitLab.Repo == "" {
				return errors.New("versionDiscovery gitlab requires repo")
			}
		}
		if d.Git != nil {
			sources++
			filter = d.Git.Filter
			if err := validateHTTPURL(d.Git.URL); err != nil {
				return fmt.Errorf("versionDiscovery git url: %w", err)
			}
		}
		if d.HTTP != nil {
			sources++
			filter = d.HTTP.Filter
			if err := validateHTTPURL(d.HTTP.URL); err != nil {
				return fmt.Errorf("versionDiscovery http url: %w", err)
			}
			if d.HTTP.Filter == "" {
				return errors.New("versionDiscovery http requires filter")
			}
		}
		if d.OCI != "" {
			sources++
		}
		if sources != 1 {
			return errors.New("versionDiscovery must set exactly one of github, gitlab, git, http, oci")
		}
		if filter != "" {
			if _, err := regexp.Compile(filter); err != nil {
				return fmt.Errorf("versionDiscovery filter: %w", err)
			}
		}
	}
	return nil
}

func (r *Recipe) validate(covered []VersionEntry) error {
	fetchers := 0
	if r.Artifact.URL != "" {
		fetchers++
	}
	if r.Artifact.GitHub != nil {
		fetchers++
	}
	if r.Artifact.OCI != "" {
		fetchers++
	}
	if fetchers != 1 {
		return errors.New("artifact must set exactly one of url, github, oci")
	}
	if len(r.Install) == 0 {
		return errors.New("install is required and must be non-empty")
	}
	for i, a := range r.Install {
		switch {
		case a.Copy != nil:
			if a.Copy.Src == "" {
				return fmt.Errorf("install[%d]: copy requires src", i)
			}
			if a.Copy.Dst == "" {
				return fmt.Errorf("install[%d]: copy requires dst", i)
			}
		case a.Move != nil:
			if a.Move.Src == "" {
				return fmt.Errorf("install[%d]: move requires src", i)
			}
			if a.Move.Dst == "" {
				return fmt.Errorf("install[%d]: move requires dst", i)
			}
		case a.Extract == nil && a.Mkdir == "":
			return fmt.Errorf("install[%d]: empty action", i)
		}
		if _, err := ExpandActionPaths(a, "v0", Platform{OS: "darwin", Arch: "arm64"}); err != nil {
			return fmt.Errorf("install[%d]: %w", i, err)
		}
	}
	for _, v := range covered {
		if v.Sha256 == nil {
			if r.Artifact.OCI == "" {
				return fmt.Errorf("version %s: sha256 required for url/github artifacts", v.Version)
			}
			continue
		}
		for _, plat := range r.SupportedBy() {
			if v.Sha256[plat.String()] == "" {
				return fmt.Errorf("version %s: sha256 missing platform %s", v.Version, plat)
			}
		}
	}
	for _, e := range r.Env {
		if !EnvNameRE.MatchString(e.Name) {
			return fmt.Errorf("env export %q: invalid name", e.Name)
		}
	}
	for _, d := range r.Deps {
		if !NameRE.MatchString(d.Name) {
			return fmt.Errorf("dep %q: invalid name", d.Name)
		}
		if d.Compat != "" && d.Kind != DepKindLink {
			return fmt.Errorf("dep %q: compat requires kind: link", d.Name)
		}
		if d.Compat != "" && !CompatRE.MatchString(d.Compat) {
			return fmt.Errorf("dep %q: invalid compat %q", d.Name, d.Compat)
		}
	}
	for i, s := range r.Test {
		if strings.TrimSpace(s.Run) == "" {
			return fmt.Errorf("test[%d]: run is required", i)
		}
	}
	if b := r.Build; b != nil {
		if b.Source.URL == "" {
			return errors.New("build.source.url is required")
		}
		if _, err := expand(b.Source.URL, templateCtx{Version: "v0", OS: "darwin", Arch: "arm64"}); err != nil {
			return fmt.Errorf("build.source.url: %w", err)
		}
		if b.Output == "" {
			return errors.New("build.output is required")
		}
		if len(b.Steps) == 0 {
			return errors.New("build.steps is required and must be non-empty")
		}
		for i, s := range b.Steps {
			if strings.TrimSpace(s.Run) == "" {
				return fmt.Errorf("build.steps[%d]: run is required", i)
			}
		}
		for i, d := range b.Deps {
			if !NameRE.MatchString(d.Name) {
				return fmt.Errorf("build.deps[%d]: invalid name %q", i, d.Name)
			}
			if d.Compat != "" && d.Kind != DepKindLink {
				return fmt.Errorf("build.deps[%d] (%s): compat requires kind: link", i, d.Name)
			}
			if d.Compat != "" && !CompatRE.MatchString(d.Compat) {
				return fmt.Errorf("build.deps[%d] (%s): invalid compat %q", i, d.Name, d.Compat)
			}
		}
	}
	return nil
}
