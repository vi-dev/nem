package resolve

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/project"
	"github.com/vi-dev/nem/internal/spec"
)

type Tool struct {
	Key     project.ToolKey
	Version string
}

type Result struct {
	Entries []project.LockEntry
	Pkgs    map[string]*spec.Package
}

type UnsupportedPlatformError struct{ Name, Version string }

func (e *UnsupportedPlatformError) Error() string {
	return fmt.Sprintf("package %s@%s supports none of nem's platforms", e.Name, e.Version)
}

type PinConflictError struct{ Name, Pinned, Required string }

func (e *PinConflictError) Error() string {
	return fmt.Sprintf("package %s: pinned %s but a dependency requires %s", e.Name, e.Pinned, e.Required)
}

type demandKind int

const (
	demandFloating demandKind = iota
	demandRanged
	demandExact
	demandPinned
)

type demand struct {
	kind    demandKind
	version string
	compat  string
	direct  bool
	link    bool
	catalog string
	digest  string
	pkg     *spec.Package
}

func (d demand) isExact() bool { return d.kind == demandExact || d.kind == demandPinned }

type candidate struct {
	platforms    map[spec.Platform]bool
	onPath       bool
	onLoaderPath bool
	demands      []demand
}

type collector struct {
	sources []catalog.Named
	cands   map[string]*candidate
}

func newCollector(sources []catalog.Named) *collector {
	return &collector{sources: sources, cands: map[string]*candidate{}}
}

func (c *collector) record(d demand, platform spec.Platform) {
	cand, ok := c.cands[d.pkg.Name]
	if !ok {
		cand = &candidate{platforms: map[spec.Platform]bool{}}
		c.cands[d.pkg.Name] = cand
	}
	cand.platforms[platform] = true
	cand.onPath = cand.onPath || !d.link
	cand.onLoaderPath = cand.onLoaderPath || ((d.direct || d.link) && len(d.pkg.Libs) > 0)
	for _, seen := range cand.demands {
		if seen.kind == d.kind && seen.version == d.version && seen.compat == d.compat && seen.direct == d.direct {
			return
		}
	}
	cand.demands = append(cand.demands, d)
}

func (c *collector) walk(ctx context.Context, d demand, platform spec.Platform, visited map[string]bool) error {
	c.record(d, platform)
	if visited[d.pkg.Name] {
		return nil
	}
	visited[d.pkg.Name] = true
	return c.walkDeps(ctx, d.pkg, platform, visited)
}

func (c *collector) walkDeps(ctx context.Context, pkg *spec.Package, platform spec.Platform, visited map[string]bool) error {
	for _, dep := range pkg.Deps {
		if !spec.PlatformsInclude(dep.Platforms, platform) {
			continue
		}
		depPkg, depCat, depDig, err := catalog.Lookup(ctx, c.sources, project.ToolKey{Name: dep.Name})
		if err != nil {
			return err
		}
		if !slices.Contains(depPkg.SupportedBy(), platform) {
			continue
		}
		d, err := edgeDemand(dep, depPkg, depCat, depDig)
		if err != nil {
			return err
		}
		if err := c.walk(ctx, d, platform, visited); err != nil {
			return err
		}
	}
	return nil
}

func edgeDemand(dep spec.Dep, pkg *spec.Package, catName, digest string) (demand, error) {
	d := demand{link: dep.Kind == spec.DepKindLink, catalog: catName, digest: digest, pkg: pkg}
	switch {
	case dep.Compat != "":
		if selectHighest(pkg.Versions, dep.Compat) == "" {
			return demand{}, &catalog.VersionNotFoundError{Name: dep.Name, Version: dep.Compat + ".x", Catalog: catName}
		}
		d.kind, d.compat = demandRanged, dep.Compat
	case dep.Version != "":
		version, err := resolveVersion(pkg, dep.Name, dep.Version, catName)
		if err != nil {
			return demand{}, err
		}
		d.kind, d.version = demandExact, version
	default:
		d.kind, d.version = demandFloating, pkg.Versions[0].Version
	}
	return d, nil
}

func Resolve(ctx context.Context, tools []Tool, sources []catalog.Named) (*Result, error) {
	directNames := make(map[string]bool, len(tools))
	roots := make([]demand, len(tools))
	for i, t := range tools {
		pkg, catName, dig, err := catalog.Lookup(ctx, sources, t.Key)
		if err != nil {
			return nil, err
		}
		version, err := resolveVersion(pkg, t.Key.Name, t.Version, catName)
		if err != nil {
			return nil, err
		}
		if len(pkg.SupportedBy()) == 0 {
			return nil, &UnsupportedPlatformError{Name: t.Key.Name, Version: version}
		}
		kind := demandFloating
		if t.Version != "" {
			kind = demandPinned
		}
		directNames[t.Key.Name] = true
		roots[i] = demand{
			kind: kind, version: version, direct: true, catalog: catName, digest: dig, pkg: pkg,
		}
	}

	col := newCollector(sources)
	for _, platform := range spec.SupportedPlatforms {
		visited := map[string]bool{}
		for _, r := range roots {
			if !slices.Contains(r.pkg.SupportedBy(), platform) {
				continue
			}
			if err := col.walk(ctx, r, platform, visited); err != nil {
				return nil, err
			}
		}
	}
	return finalize(col.cands, directNames)
}

func Dependencies(ctx context.Context, pkg *spec.Package, deps []spec.Dep, sources []catalog.Named) (*Result, error) {

	rootPkg := &spec.Package{Name: pkg.Name, Platforms: pkg.Platforms, Deps: deps}
	col := newCollector(sources)
	for _, platform := range spec.SupportedPlatforms {
		if !slices.Contains(rootPkg.SupportedBy(), platform) {
			continue
		}
		visited := map[string]bool{}
		if err := col.walkDeps(ctx, rootPkg, platform, visited); err != nil {
			return nil, err
		}
	}
	return finalize(col.cands, nil)
}

func finalize(cands map[string]*candidate, directNames map[string]bool) (*Result, error) {
	names := make([]string, 0, len(cands))
	for name := range cands {
		names = append(names, name)
	}
	sort.Strings(names)

	entries := make([]project.LockEntry, 0, len(cands))
	pkgs := make(map[string]*spec.Package, len(cands))
	for _, name := range names {
		cand := cands[name]
		chosen, err := choose(name, cand.demands)
		if err != nil {
			return nil, err
		}
		platforms := make([]string, 0, len(cand.platforms))
		for _, p := range spec.SupportedPlatforms {
			if cand.platforms[p] {
				platforms = append(platforms, p.String())
			}
		}
		entries = append(entries, project.LockEntry{
			Name: name, Version: chosen.version, Catalog: chosen.catalog,
			Direct: directNames[name], Platforms: platforms, Digest: chosen.digest,
			OnPath: cand.onPath, OnLoaderPath: cand.onLoaderPath,
		})
		pkgs[name] = chosen.pkg
	}
	return &Result{Entries: entries, Pkgs: pkgs}, nil
}

func choose(name string, demands []demand) (demand, error) {
	owner := &demands[0]
	for i := range demands {
		if demands[i].direct {
			owner = &demands[i]
		}
	}

	if v := pickVersion(owner.pkg.Versions, demands); v != "" {
		return withVersion(owner, v), nil
	}

	if pin := pinnedOf(demands); pin != nil {
		rest := slices.DeleteFunc(slices.Clone(demands), func(d demand) bool { return d.kind == demandPinned })
		if v := pickVersion(owner.pkg.Versions, rest); v != "" {
			return demand{}, &PinConflictError{Name: name, Pinned: pin.version, Required: v}
		}
	}

	for _, d := range demands {
		if d.isExact() && satisfiesAll(d.version, demands) {
			return demand{}, &catalog.VersionNotFoundError{Name: name, Version: d.version, Catalog: owner.catalog}
		}
		if d.kind == demandRanged && pickVersion(d.pkg.Versions, demands) != "" {
			return demand{}, &catalog.VersionNotFoundError{Name: name, Version: d.compat + ".x", Catalog: owner.catalog}
		}
	}

	return demand{}, newCompatConflictError(name, requirements(demands)...)
}

func satisfies(v string, d demand) bool {
	switch d.kind {
	case demandRanged:
		return matchesCompat(v, d.compat)
	case demandExact, demandPinned:
		return v == d.version
	case demandFloating:
		return true
	}
	return true
}

func satisfiesAll(v string, demands []demand) bool {
	for _, d := range demands {
		if !satisfies(v, d) {
			return false
		}
	}
	return true
}

func pickVersion(versions []spec.VersionEntry, demands []demand) string {
	ranged := slices.ContainsFunc(demands, func(d demand) bool { return d.kind == demandRanged })
	best := ""
	for _, v := range versions {
		if !satisfiesAll(v.Version, demands) {
			continue
		}
		if !ranged {
			return v.Version
		}
		if best == "" || spec.CompareVersions(v.Version, best) > 0 {
			best = v.Version
		}
	}
	return best
}

func requirements(demands []demand) []string {
	var out []string
	for _, d := range demands {
		switch d.kind {
		case demandRanged:
			out = append(out, d.compat)
		case demandExact, demandPinned:
			out = append(out, d.version)
		}
	}
	return out
}

func withVersion(d *demand, v string) demand {
	c := *d
	c.version = v
	return c
}

func pinnedOf(demands []demand) *demand {
	for i := range demands {
		if demands[i].kind == demandPinned {
			return &demands[i]
		}
	}
	return nil
}

func resolveVersion(pkg *spec.Package, name, version, catName string) (string, error) {
	if version == "" {
		return pkg.Versions[0].Version, nil
	}
	for _, v := range pkg.Versions {
		if v.Version == version {
			return version, nil
		}
	}
	return "", &catalog.VersionNotFoundError{Name: name, Version: version, Catalog: catName}
}
