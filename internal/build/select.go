package build

import (
	"context"
	"errors"
	"slices"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/resolve"
	"github.com/vi-dev/nem/internal/spec"
)

func Select(ctx context.Context, t *Target, packages []string) ([]Selection, error) {
	sels := make([]Selection, 0, len(packages))
	for _, p := range packages {
		pkg, version, err := selectPackage(ctx, t, p)
		if err != nil {
			return nil, err
		}
		sels = append(sels, Selection{Pkg: pkg, Version: version, Reason: "forced", Platforms: pkg.SupportedBy()})
	}
	return sels, nil
}

func SelectDeps(ctx context.Context, t *Target, configured *catalog.Set, roots []Selection) ([]Selection, error) {
	names := map[string]bool{}
	var pkgs []*spec.Package
	for _, r := range roots {
		if names[r.Pkg.Name] {
			continue
		}
		names[r.Pkg.Name] = true
		pkgs = append(pkgs, r.Pkg)
	}
	catalogs := configured.Prepend(t.Entry)
	var sels []Selection
	for _, root := range pkgs {
		deps, err := selectDependencies(ctx, t, root, catalogs)
		if err != nil {
			return nil, err
		}
		for _, d := range deps {
			if names[d.Pkg.Name] {
				continue
			}
			names[d.Pkg.Name] = true
			sels = append(sels, d)
		}
	}
	return sels, nil
}

func SelectAll(ctx context.Context, t *Target) ([]Selection, error) {
	pkgs, err := t.Packages(ctx)
	if err != nil {
		return nil, err
	}
	var sels []Selection
	for _, pkg := range pkgs {
		if !buildable(pkg) || len(pkg.Versions) == 0 {
			continue
		}
		sels = append(sels, Selection{Pkg: pkg, Version: pkg.Versions[0].Version, Reason: "latest", Platforms: pkg.SupportedBy()})
	}
	return sels, nil
}

func buildable(pkg *spec.Package) bool {
	return pkg.Artifact.OCI != "" && pkg.Build != nil
}

func selectPackage(ctx context.Context, t *Target, selector string) (*spec.Package, string, error) {
	ref, err := spec.ParseRef(selector)
	if err != nil {
		return nil, "", err
	}
	pkg, _, err := t.Entry.Catalog.Package(ctx, ref.Name)
	if err != nil {
		if nf, ok := errors.AsType[*catalog.PackageNotFoundError](err); ok {
			nf.Catalogs = []string{t.Ref}
			return nil, "", nf
		}
		return nil, "", err
	}
	version := ref.Version
	if version == "" {
		version = pkg.Versions[0].Version
	}
	return pkg, version, nil
}

func selectDependencies(ctx context.Context, t *Target, root *spec.Package, catalogs *catalog.Set) ([]Selection, error) {
	deps := root.Deps
	if root.Build != nil {
		deps = append(append([]spec.Dep(nil), deps...), root.Build.Deps...)
	}
	if len(deps) == 0 {
		return nil, nil
	}
	resolvedDeps, err := resolve.Dependencies(ctx, root, deps, catalogs)
	if err != nil {
		return nil, err
	}

	plat := spec.Current()
	var sels []Selection
	for _, e := range resolvedDeps.Entries {
		if !slices.Contains(e.Platforms, plat.String()) {
			continue
		}

		if p := resolvedDeps.Pkgs[e.Name]; p == nil || p.Artifact.OCI == "" {
			continue
		}
		pkg, _, err := t.Entry.Catalog.Package(ctx, e.Name)
		if err != nil {
			if _, ok := errors.AsType[*catalog.PackageNotFoundError](err); ok {
				continue
			}
			return nil, err
		}
		if pkg.Build == nil {
			continue
		}
		exists, err := t.ArchiveExists(ctx, e.Name, e.Version, plat)
		if err != nil {
			return nil, err
		}
		if exists {
			continue
		}
		sels = append(sels, Selection{Pkg: pkg, Version: e.Version, Reason: "dep of " + root.Name, Platforms: pkg.SupportedBy()})
	}
	return sels, nil
}

func Merge(a, b []Selection) []Selection {
	seen := map[string]bool{}
	out := make([]Selection, 0, len(a)+len(b))
	for _, s := range slices.Concat(a, b) {
		key := s.Pkg.Name + "@" + s.Version
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
	}
	return out
}

type MissingStats struct{ Checked, Skipped int }

func SelectMissing(ctx context.Context, t *Target, scope []spec.Ref) ([]Selection, MissingStats, error) {
	type candidate struct {
		pkg      *spec.Package
		versions []string
	}
	var candidates []candidate
	var stats MissingStats
	if len(scope) == 0 {
		pkgs, err := t.Packages(ctx)
		if err != nil {
			return nil, MissingStats{}, err
		}
		for _, pkg := range pkgs {
			if !buildable(pkg) {
				stats.Skipped++
				continue
			}
			candidates = append(candidates, candidate{pkg: pkg, versions: versionsOf(pkg)})
		}
	}
	for _, ref := range scope {
		pkg, _, err := t.Entry.Catalog.Package(ctx, ref.Name)
		if err != nil {
			if nf, ok := errors.AsType[*catalog.PackageNotFoundError](err); ok {
				nf.Catalogs = []string{t.Ref}
				return nil, MissingStats{}, nf
			}
			return nil, MissingStats{}, err
		}
		if !buildable(pkg) {
			stats.Skipped++
			continue
		}
		versions := versionsOf(pkg)
		if ref.Version != "" {
			if !pkg.HasEqualVersion(ref.Version) {
				return nil, MissingStats{}, &catalog.VersionNotFoundError{Name: ref.Name, Version: ref.Version, Catalog: t.Ref}
			}
			versions = []string{ref.Version}
		}
		candidates = append(candidates, candidate{pkg: pkg, versions: versions})
	}

	seen := map[string]bool{}
	var sels []Selection
	for _, c := range candidates {
		stats.Checked++
		plats := c.pkg.SupportedBy()
		for _, v := range c.versions {
			if seen[c.pkg.Name+"@"+v] {
				continue
			}
			seen[c.pkg.Name+"@"+v] = true
			have, err := t.ArchivePlatforms(ctx, c.pkg.Name, v)
			if err != nil {
				return nil, MissingStats{}, err
			}
			var absent []spec.Platform
			for _, plat := range plats {
				if !slices.Contains(have, plat) {
					absent = append(absent, plat)
				}
			}
			if len(absent) > 0 {
				sels = append(sels, Selection{Pkg: c.pkg, Version: v, Reason: "missing archive", Platforms: absent})
			}
		}
	}
	return sels, stats, nil
}

func versionsOf(pkg *spec.Package) []string {
	out := make([]string, len(pkg.Versions))
	for i, v := range pkg.Versions {
		out[i] = v.Version
	}
	return out
}
