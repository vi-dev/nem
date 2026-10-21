package resolve

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

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
	set   *catalog.Set
	cands map[string]*candidate
	picks map[string]string
	hits  map[string]catalog.Hit
	err   error
}

func (c *collector) fail(err error) {
	if c.err == nil {
		c.err = err
	}
}

func newCollector(set *catalog.Set, picks map[string]string, hits map[string]catalog.Hit) *collector {
	return &collector{set: set, cands: map[string]*candidate{}, picks: picks, hits: hits}
}

func (c *collector) lookup(ctx context.Context, name string) (catalog.Hit, error) {
	if hit, ok := c.hits[name]; ok {
		return hit, nil
	}
	hit, err := c.set.Lookup(ctx, project.ToolKey{Name: name})
	if err != nil {
		return catalog.Hit{}, err
	}
	c.hits[name] = hit
	return hit, nil
}

func (c *collector) provisionalVersion(d demand) string {
	if pick, ok := c.picks[d.pkg.Name]; ok && satisfies(pick, d) && d.pkg.HasEqualVersion(pick) {
		return pick
	}
	if d.kind == demandRanged {
		return selectHighest(d.pkg.Versions, d.compat)
	}
	return d.version
}

func (c *collector) recipe(d demand) (*spec.Recipe, error) {
	return d.pkg.Recipe(c.provisionalVersion(d))
}

func (c *collector) record(d demand, platform spec.Platform) error {
	r, err := c.recipe(d)
	if err != nil {
		return err
	}
	cand, ok := c.cands[d.pkg.Name]
	if !ok {
		cand = &candidate{platforms: map[spec.Platform]bool{}}
		c.cands[d.pkg.Name] = cand
	}
	cand.platforms[platform] = true
	cand.onPath = cand.onPath || !d.link
	cand.onLoaderPath = cand.onLoaderPath || ((d.direct || d.link) && len(r.Libs) > 0)
	for _, seen := range cand.demands {
		if seen.kind == d.kind && seen.version == d.version && seen.compat == d.compat && seen.direct == d.direct {
			return nil
		}
	}
	cand.demands = append(cand.demands, d)
	return nil
}

func (c *collector) walk(ctx context.Context, d demand, platform spec.Platform, visited map[string]bool) {
	if err := c.record(d, platform); err != nil {
		c.fail(err)
		return
	}
	if visited[d.pkg.Name] {
		return
	}
	visited[d.pkg.Name] = true
	r, err := c.recipe(d)
	if err != nil {
		c.fail(err)
		return
	}
	c.walkDeps(ctx, r.Deps, platform, visited, d.pkg.Name+"@"+c.provisionalVersion(d))
}

func (c *collector) walkDeps(ctx context.Context, deps []spec.Dep, platform spec.Platform, visited map[string]bool, from string) {
	for _, dep := range deps {
		if !spec.PlatformsInclude(dep.Platforms, platform) {
			continue
		}
		d, ok, err := c.demandFor(ctx, dep, platform, from)
		if err != nil {
			c.fail(err)
			continue
		}
		if ok {
			c.walk(ctx, d, platform, visited)
		}
	}
}

func (c *collector) demandFor(ctx context.Context, dep spec.Dep, platform spec.Platform, from string) (demand, bool, error) {
	hit, err := c.lookup(ctx, dep.Name)
	if err != nil {
		return demand{}, false, err
	}
	d, err := edgeDemand(dep, hit.Pkg, hit.Entry.Name, hit.Digest, from)
	if err != nil {
		return demand{}, false, err
	}
	r, err := c.recipe(d)
	if err != nil {
		return demand{}, false, err
	}
	if !slices.Contains(r.SupportedBy(), platform) {
		return demand{}, false, nil
	}
	return d, true, nil
}

const maxSettleRounds = 8

type UnsettledError struct {
	Rounds int
	Names  []string
	Err    error
}

func (e *UnsettledError) Error() string {
	msg := fmt.Sprintf("dependency resolution did not settle after %d rounds: %s keep changing; pin one of them",
		e.Rounds, strings.Join(e.Names, ", "))
	if e.Err != nil {
		msg += " (last error: " + e.Err.Error() + ")"
	}
	return msg
}

func (e *UnsettledError) Unwrap() error { return e.Err }

func settle(ctx context.Context, set *catalog.Set, directNames map[string]bool, seed func(*collector)) (*Result, error) {
	var picks map[string]string
	var history []map[string]string
	hits := map[string]catalog.Hit{}
	for round := 1; ; round++ {
		col := newCollector(set, picks, hits)
		seed(col)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		res, next, err := finalize(col.cands, directNames)
		if err == nil {
			err = col.err
		}
		changing := changedPicks(picks, next)
		if len(changing) == 0 {
			if err != nil {
				return nil, err
			}
			return res, nil
		}
		cycled := slices.ContainsFunc(history, func(h map[string]string) bool { return maps.Equal(h, next) })
		if cycled || round == maxSettleRounds {
			return nil, &UnsettledError{Rounds: round, Names: changing, Err: err}
		}
		history = append(history, next)
		picks = next
	}
}

func changedPicks(prev, next map[string]string) []string {
	var names []string
	for name, v := range next {
		if old, ok := prev[name]; !ok || old != v {
			names = append(names, name)
		}
	}
	for name := range prev {
		if _, ok := next[name]; !ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func edgeDemand(dep spec.Dep, pkg *spec.Package, catName, digest, from string) (demand, error) {
	d := demand{link: dep.Kind == spec.DepKindLink, catalog: catName, digest: digest, pkg: pkg}
	switch {
	case dep.Compat != "":
		if selectHighest(pkg.Versions, dep.Compat) == "" {
			return demand{}, &catalog.VersionNotFoundError{Name: dep.Name, Version: dep.Compat + ".x", Catalog: catName}
		}
		d.kind, d.compat = demandRanged, dep.Compat
	case dep.Version != "":
		version, err := resolveVersion(pkg, dep.Name, dep.Version, catName)
		if ee, ok := errors.AsType[*catalog.VersionExcludedError](err); ok {
			ee.RequiredBy = from
		}
		if err != nil {
			return demand{}, err
		}
		d.kind, d.version = demandExact, version
	default:
		version, err := resolveVersion(pkg, dep.Name, "", catName)
		if err != nil {
			return demand{}, err
		}
		d.kind, d.version = demandFloating, version
	}
	return d, nil
}

func Resolve(ctx context.Context, tools []Tool, set *catalog.Set) (*Result, error) {
	directNames := make(map[string]bool, len(tools))
	roots := make([]demand, len(tools))
	for i, t := range tools {
		hit, err := set.Lookup(ctx, t.Key)
		if err != nil {
			return nil, err
		}
		pkg, catName, dig := hit.Pkg, hit.Entry.Name, hit.Digest
		version, err := resolveVersion(pkg, t.Key.Name, t.Version, catName)
		if err != nil {
			return nil, err
		}
		r, err := pkg.Recipe(version)
		if err != nil {
			return nil, err
		}
		if len(r.SupportedBy()) == 0 {
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

	return settle(ctx, set, directNames, func(col *collector) {
		for _, platform := range spec.SupportedPlatforms {
			visited := map[string]bool{}
			for _, root := range roots {
				r, err := col.recipe(root)
				if err != nil {
					col.fail(err)
					continue
				}
				if slices.Contains(r.SupportedBy(), platform) {
					col.walk(ctx, root, platform, visited)
				}
			}
		}
	})
}

func Dependencies(ctx context.Context, name string, platforms []spec.Platform, deps []spec.Dep, set *catalog.Set) (*Result, error) {
	return settle(ctx, set, nil, func(col *collector) {
		for _, platform := range spec.SupportedPlatforms {
			if spec.PlatformsInclude(platforms, platform) {
				col.walkDeps(ctx, deps, platform, map[string]bool{name: true}, name)
			}
		}
	})
}

func finalize(cands map[string]*candidate, directNames map[string]bool) (*Result, map[string]string, error) {
	names := make([]string, 0, len(cands))
	for name := range cands {
		names = append(names, name)
	}
	sort.Strings(names)

	entries := make([]project.LockEntry, 0, len(cands))
	pkgs := make(map[string]*spec.Package, len(cands))
	picks := make(map[string]string, len(cands))
	var firstErr error
	for _, name := range names {
		cand := cands[name]
		chosen, err := choose(name, cand.demands)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			picks[name] = strongestVersion(cand.demands)
			continue
		}
		picks[name] = chosen.version
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
	return &Result{Entries: entries, Pkgs: pkgs}, picks, firstErr
}

func strongestVersion(demands []demand) string {
	for _, kind := range []demandKind{demandPinned, demandExact} {
		if i := slices.IndexFunc(demands, func(d demand) bool { return d.kind == kind }); i >= 0 {
			return demands[i].version
		}
	}
	best := ""
	for _, d := range demands {
		if d.kind != demandRanged {
			continue
		}
		if v := selectHighest(d.pkg.Versions, d.compat); best == "" || spec.CompareVersions(v, best) > 0 {
			best = v
		}
	}
	if best != "" {
		return best
	}
	return demands[0].version
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
		return spec.MatchesPrefix(v, d.compat)
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
		if v.Excluded != nil || !satisfiesAll(v.Version, demands) {
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
		if latest := pkg.Latest(); latest != "" {
			return latest, nil
		}
		return "", &catalog.VersionNotFoundError{Name: name, Version: "latest", Catalog: catName}
	}
	for _, v := range pkg.Versions {
		if v.Version != version {
			continue
		}
		if ex := v.Excluded; ex != nil {
			return "", &catalog.VersionExcludedError{Name: name, Version: version, Catalog: catName, Reason: ex.Reason, Use: ex.Use}
		}
		return version, nil
	}
	return "", &catalog.VersionNotFoundError{Name: name, Version: version, Catalog: catName}
}
