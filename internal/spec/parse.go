package spec

import (
	"fmt"
	"maps"
	"slices"
	"sort"

	"github.com/goccy/go-yaml"
)

type rawPackage struct {
	Schema           int                          `yaml:"schema"`
	Name             string                       `yaml:"name"`
	Description      string                       `yaml:"description"`
	Homepage         string                       `yaml:"homepage"`
	License          string                       `yaml:"license"`
	VersionDiscovery *rawDiscovery                `yaml:"versionDiscovery"`
	Versions         []yaml.RawMessage            `yaml:"versions"`
	Platforms        []string                     `yaml:"platforms"`
	Deps             []rawDep                     `yaml:"deps"`
	Artifact         rawArtifact                  `yaml:"artifact"`
	Install          []map[string]yaml.RawMessage `yaml:"install"`
	Bins             []string                     `yaml:"bins"`
	Libs             []string                     `yaml:"libs"`
	Env              []rawEnv                     `yaml:"env"`
	Build            *rawBuild                    `yaml:"build"`
	Test             []rawTestStep                `yaml:"test"`
}

type rawRecipe struct {
	Platforms []string                     `yaml:"platforms"`
	Deps      []rawDep                     `yaml:"deps"`
	Artifact  rawArtifact                  `yaml:"artifact"`
	Install   []map[string]yaml.RawMessage `yaml:"install"`
	Bins      []string                     `yaml:"bins"`
	Libs      []string                     `yaml:"libs"`
	Env       []rawEnv                     `yaml:"env"`
	Build     *rawBuild                    `yaml:"build"`
	Test      []rawTestStep                `yaml:"test"`
}

type rawDep struct {
	Name      string   `yaml:"name"`
	Version   string   `yaml:"version"`
	Platforms []string `yaml:"platforms"`
	Kind      string   `yaml:"kind"`
	Compat    string   `yaml:"compat"`
}

type rawDiscovery struct {
	GitHub *GitHubDiscovery `yaml:"github"`
	GitLab *GitLabDiscovery `yaml:"gitlab"`
	Git    *GitDiscovery    `yaml:"git"`
	HTTP   *HTTPDiscovery   `yaml:"http"`
	OCI    string           `yaml:"oci"`
}

type rawArtifact struct {
	URL    string       `yaml:"url"`
	GitHub *GitHubAsset `yaml:"github"`
	OCI    string       `yaml:"oci"`
}

type rawEnv struct {
	Name      string   `yaml:"name"`
	Value     string   `yaml:"value"`
	Platforms []string `yaml:"platforms"`
}

type rawBuild struct {
	Deps   []rawDep `yaml:"deps"`
	Source struct {
		URL string `yaml:"url"`
	} `yaml:"source"`
	Steps []struct {
		Run       string   `yaml:"run"`
		Platforms []string `yaml:"platforms"`
	} `yaml:"steps"`
	Output    string `yaml:"output"`
	Normalize *bool  `yaml:"normalize"`
}

type rawTestStep struct {
	Run       string   `yaml:"run"`
	Platforms []string `yaml:"platforms"`
}

type rawVersionEntry struct {
	Version      string            `yaml:"version"`
	Meta         map[string]string `yaml:"meta"`
	Sha256       yaml.RawMessage   `yaml:"sha256"`
	SourceSha256 string            `yaml:"sourceSha256"`
}

type Files struct {
	Pkg     []byte
	Recipes map[string][]byte
}

func (f Files) Names() []string {
	return slices.Sorted(maps.Keys(f.Recipes))
}

func Parse(data []byte) (*Package, error) {
	return ParseFiles(Files{Pkg: data})
}

func (r rawPackage) recipe() rawRecipe {
	return rawRecipe{
		Platforms: r.Platforms, Deps: r.Deps, Artifact: r.Artifact, Install: r.Install,
		Bins: r.Bins, Libs: r.Libs, Env: r.Env, Build: r.Build, Test: r.Test,
	}
}

func ParseFiles(f Files) (*Package, error) {
	var raw rawPackage
	if err := yaml.UnmarshalWithOptions(f.Pkg, &raw, yaml.Strict()); err != nil {
		return nil, fmt.Errorf("parse %s: %w", PkgFileName, err)
	}
	p := &Package{
		Schema: raw.Schema, Name: raw.Name, Description: raw.Description,
		Homepage: raw.Homepage, License: raw.License,
	}
	if raw.VersionDiscovery != nil {
		p.VersionDiscovery = &Discovery{
			GitHub: raw.VersionDiscovery.GitHub,
			GitLab: raw.VersionDiscovery.GitLab,
			Git:    raw.VersionDiscovery.Git,
			HTTP:   raw.VersionDiscovery.HTTP,
			OCI:    raw.VersionDiscovery.OCI,
		}
	}
	for i, v := range raw.Versions {
		entry, err := shapeVersion(v)
		if err != nil {
			return nil, fmt.Errorf("versions[%d]: %w", i, err)
		}
		p.Versions = append(p.Versions, entry)
	}
	base, err := shapeRecipe(raw.recipe())
	if err != nil {
		return nil, err
	}
	p.Base = base
	for _, name := range f.Names() {
		prefix, ok := ParseRecipeFileName(name)
		if !ok {
			return nil, fmt.Errorf("recipe file %q: name must be recipe@<prefix>.yaml", name)
		}
		var rr rawRecipe
		if err := yaml.UnmarshalWithOptions(f.Recipes[name], &rr, yaml.Strict()); err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		r, err := shapeRecipe(rr)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		r.Prefix = prefix
		p.Frozen = append(p.Frozen, r)
	}
	sort.SliceStable(p.Frozen, func(i, j int) bool {
		return CompareVersions(p.Frozen[i].Prefix, p.Frozen[j].Prefix) < 0
	})
	return p, nil
}

func shapeRecipe(raw rawRecipe) (Recipe, error) {
	r := Recipe{Bins: raw.Bins, Libs: raw.Libs}
	if len(r.Bins) == 0 {
		r.Bins = []string{"bin"}
	}
	var err error
	if r.Platforms, err = parsePlatforms(raw.Platforms); err != nil {
		return Recipe{}, err
	}
	for i, d := range raw.Deps {
		dep, err := shapeDep(d)
		if err != nil {
			return Recipe{}, fmt.Errorf("deps[%d]: %w", i, err)
		}
		r.Deps = append(r.Deps, dep)
	}
	r.Artifact = Artifact{URL: raw.Artifact.URL, GitHub: raw.Artifact.GitHub, OCI: raw.Artifact.OCI}
	for i, m := range raw.Install {
		act, err := shapeAction(m)
		if err != nil {
			return Recipe{}, fmt.Errorf("install[%d]: %w", i, err)
		}
		r.Install = append(r.Install, act)
	}
	for i, e := range raw.Env {
		plats, err := parsePlatforms(e.Platforms)
		if err != nil {
			return Recipe{}, fmt.Errorf("env[%d]: %w", i, err)
		}
		r.Env = append(r.Env, EnvExport{Name: e.Name, Value: e.Value, Platforms: plats})
	}
	if raw.Build != nil {
		b := &Build{Output: raw.Build.Output, Normalize: raw.Build.Normalize}
		b.Source.URL = raw.Build.Source.URL
		for i, d := range raw.Build.Deps {
			dep, err := shapeDep(d)
			if err != nil {
				return Recipe{}, fmt.Errorf("build.deps[%d]: %w", i, err)
			}
			b.Deps = append(b.Deps, dep)
		}
		for i, s := range raw.Build.Steps {
			plats, err := parsePlatforms(s.Platforms)
			if err != nil {
				return Recipe{}, fmt.Errorf("build.steps[%d]: %w", i, err)
			}
			b.Steps = append(b.Steps, BuildStep{Run: s.Run, Platforms: plats})
		}
		r.Build = b
	}
	for i, s := range raw.Test {
		plats, err := parsePlatforms(s.Platforms)
		if err != nil {
			return Recipe{}, fmt.Errorf("test[%d]: %w", i, err)
		}
		r.Test = append(r.Test, TestStep{Run: s.Run, Platforms: plats})
	}
	return r, nil
}

func parsePlatforms(ss []string) ([]Platform, error) {
	var out []Platform
	for _, s := range ss {
		p, err := ParsePlatform(s)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (d *rawDep) UnmarshalYAML(b []byte) error {
	var s string
	if err := yaml.Unmarshal(b, &s); err == nil {
		ref, err := ParseRef(s)
		if err != nil {
			return err
		}
		d.Name, d.Version = ref.Name, ref.Version
		return nil
	}
	type plain rawDep
	var p plain
	if err := yaml.UnmarshalWithOptions(b, &p, yaml.Strict()); err != nil {
		return err
	}
	*d = rawDep(p)
	return nil
}

func shapeDep(d rawDep) (Dep, error) {
	plats, err := parsePlatforms(d.Platforms)
	if err != nil {
		return Dep{}, err
	}
	kind, err := parseDepKind(d.Kind)
	if err != nil {
		return Dep{}, err
	}
	return Dep{Name: d.Name, Version: d.Version, Platforms: plats, Kind: kind, Compat: d.Compat}, nil
}

func parseDepKind(s string) (DepKind, error) {
	switch s {
	case "", "run":
		return DepKindRun, nil
	case "link":
		return DepKindLink, nil
	default:
		return "", fmt.Errorf("invalid dep kind %q (want run or link)", s)
	}
}

func shapeAction(m map[string]yaml.RawMessage) (Action, error) {
	var plats []Platform
	if raw, ok := m["platforms"]; ok {
		var ss []string
		if err := yaml.Unmarshal(raw, &ss); err != nil {
			return Action{}, fmt.Errorf("platforms: %w", err)
		}
		var err error
		if plats, err = parsePlatforms(ss); err != nil {
			return Action{}, err
		}
		delete(m, "platforms")
	}
	if len(m) != 1 {
		return Action{}, fmt.Errorf("action must have exactly one action key")
	}
	for key, val := range m {
		switch key {
		case "extract":
			var e ExtractAction
			if err := yaml.UnmarshalWithOptions(val, &e, yaml.Strict()); err != nil {
				return Action{}, fmt.Errorf("extract: %w", err)
			}
			return Action{Extract: &e, Platforms: plats}, nil
		case "copy":
			var c CopyAction
			if err := yaml.UnmarshalWithOptions(val, &c, yaml.Strict()); err != nil {
				return Action{}, fmt.Errorf("copy: %w", err)
			}
			return Action{Copy: &c, Platforms: plats}, nil
		case "move":
			var mv MoveAction
			if err := yaml.UnmarshalWithOptions(val, &mv, yaml.Strict()); err != nil {
				return Action{}, fmt.Errorf("move: %w", err)
			}
			return Action{Move: &mv, Platforms: plats}, nil
		case "mkdir":
			var dir string
			if err := yaml.Unmarshal(val, &dir); err != nil {
				return Action{}, fmt.Errorf("mkdir: %w", err)
			}
			return Action{Mkdir: dir, Platforms: plats}, nil
		default:
			return Action{}, fmt.Errorf("unknown action %q", key)
		}
	}
	return Action{}, fmt.Errorf("empty action")
}

func shapeVersion(msg yaml.RawMessage) (VersionEntry, error) {
	var s string
	if err := yaml.Unmarshal(msg, &s); err == nil {
		return VersionEntry{Version: s}, nil
	}
	var raw rawVersionEntry
	if err := yaml.UnmarshalWithOptions(msg, &raw, yaml.Strict()); err != nil {
		return VersionEntry{}, err
	}
	entry := VersionEntry{Version: raw.Version, Meta: raw.Meta, SourceSha256: raw.SourceSha256}
	if len(raw.Sha256) > 0 {
		if err := yaml.Unmarshal(raw.Sha256, &entry.Sha256); err != nil {
			return VersionEntry{}, fmt.Errorf("sha256: must be a per-platform map: %w", err)
		}
	}
	return entry, nil
}
