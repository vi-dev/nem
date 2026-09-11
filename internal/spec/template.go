package spec

import (
	"fmt"
	"strings"
	"text/template"
)

type templateCtx struct {
	Version string
	OS      string
	Arch    string
	Meta    map[string]string
}

const ArtifactToken = "{{.Artifact}}"

type actionCtx struct {
	Version  string
	OS       string
	Arch     string
	Artifact string
}

var helperFuncs = template.FuncMap{
	"trimPrefix":        func(s, prefix string) string { return strings.TrimPrefix(s, prefix) },
	"trimSuffix":        func(s, suffix string) string { return strings.TrimSuffix(s, suffix) },
	"replace":           func(s, old, new string) string { return strings.ReplaceAll(s, old, new) },
	"versionMajor":      func(v string) string { return versionSegments(v, 1) },
	"versionMajorMinor": func(v string) string { return versionSegments(v, 2) },
}

func versionSegments(v string, n int) string {
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) > n {
		parts = parts[:n]
	}
	return strings.Join(parts, ".")
}

func ExpandActionPaths(a Action, version string, plat Platform) (Action, error) {
	ctx := actionCtx{Version: version, OS: plat.OS, Arch: plat.Arch, Artifact: ArtifactToken}
	exp := func(s string) (string, error) {
		if s == "" {
			return "", nil
		}
		return expand(s, ctx)
	}
	var err error
	switch {
	case a.Copy != nil:
		c := *a.Copy
		if c.Src, err = exp(c.Src); err != nil {
			return Action{}, err
		}
		if c.Dst, err = exp(c.Dst); err != nil {
			return Action{}, err
		}
		a.Copy = &c
	case a.Move != nil:
		m := *a.Move
		if m.Src, err = exp(m.Src); err != nil {
			return Action{}, err
		}
		if m.Dst, err = exp(m.Dst); err != nil {
			return Action{}, err
		}
		a.Move = &m
	case a.Mkdir != "":
		if a.Mkdir, err = exp(a.Mkdir); err != nil {
			return Action{}, err
		}
	}
	return a, nil
}

func expand(tmpl string, ctx any) (string, error) {
	t, err := template.New("").Funcs(helperFuncs).Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse template %q: %w", tmpl, err)
	}
	var b strings.Builder
	if err := t.Execute(&b, ctx); err != nil {
		return "", fmt.Errorf("expand template %q: %w", tmpl, err)
	}
	return b.String(), nil
}

func (p *Package) ArtifactURL(version string, plat Platform) (string, error) {
	return expand(p.Artifact.URL, p.templateCtx(version, plat))
}

func (p *Package) BuildSourceURL(version string, plat Platform) (string, error) {
	if p.Build == nil {
		return "", fmt.Errorf("package %s has no build section", p.Name)
	}
	return expand(p.Build.Source.URL, p.templateCtx(version, plat))
}

func (p *Package) AssetName(version string, plat Platform) (string, error) {
	if p.Artifact.GitHub == nil {
		return "", fmt.Errorf("package %s has no github artifact", p.Name)
	}
	return expand(p.Artifact.GitHub.Asset, p.templateCtx(version, plat))
}

func (p *Package) templateCtx(version string, plat Platform) templateCtx {
	ctx := templateCtx{Version: version, OS: plat.OS, Arch: plat.Arch, Meta: map[string]string{}}
	for _, v := range p.Versions {
		if v.Version == version {
			ctx.Meta = v.Meta
			break
		}
	}
	return ctx
}

func (p *Package) Sha256(version string, plat Platform) (string, error) {
	for _, v := range p.Versions {
		if v.Version != version {
			continue
		}
		sum := v.Sha256[plat.String()]
		if sum == "" {
			return "", fmt.Errorf("no sha256 for %s@%s on %s", p.Name, version, plat)
		}
		return sum, nil
	}
	return "", fmt.Errorf("version %s not offered by %s", version, p.Name)
}
