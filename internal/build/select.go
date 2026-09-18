package build

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/resolve"
	"github.com/vi-dev/nem/internal/spec"
)

func Select(ctx context.Context, t *Target, configured *catalog.Set, packages []string, withDeps bool) ([]Selection, error) {
	var sels []Selection
	names := map[string]bool{}
	add := func(s Selection) {
		names[s.Pkg.Name] = true
		sels = append(sels, s)
	}

	roots := make([]*spec.Package, 0, len(packages))
	for _, p := range packages {
		pkg, version, err := selectPackage(ctx, t, p)
		if err != nil {
			return nil, err
		}

		add(Selection{Pkg: pkg, Version: version, Reason: "forced", Platforms: pkg.SupportedBy()})
		roots = append(roots, pkg)
	}

	if !withDeps {
		return sels, nil
	}
	catalogs := configured.Prepend(t.Entry)
	for _, root := range roots {
		deps, err := selectDependencies(ctx, t, root, catalogs)
		if err != nil {
			return nil, err
		}
		for _, d := range deps {
			if names[d.Pkg.Name] {
				continue
			}
			add(d)
		}
	}
	return sels, nil
}

func selectPackage(ctx context.Context, t *Target, selector string) (*spec.Package, string, error) {
	name, version, pinned := strings.Cut(selector, "@")
	if name == "" || (pinned && version == "") {
		return nil, "", fmt.Errorf("invalid package selection %q: want name or name@version", selector)
	}
	pkg, _, err := t.Entry.Catalog.Package(ctx, name)
	if err != nil {
		if nf, ok := errors.AsType[*catalog.PackageNotFoundError](err); ok {
			nf.Catalogs = []string{t.Ref}
			return nil, "", nf
		}
		return nil, "", err
	}
	if !pinned {
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

func SelectMissing(ctx context.Context, t *Target) ([]Selection, MissingStats, error) {
	pkgs, err := t.Packages(ctx)
	if err != nil {
		return nil, MissingStats{}, err
	}

	var stats MissingStats
	var sels []Selection
	for _, pkg := range pkgs {
		if pkg.Artifact.OCI == "" {
			stats.Skipped++
			continue
		}
		if pkg.Build == nil {
			stats.Skipped++
			continue
		}
		stats.Checked++

		plats := pkg.SupportedBy()
		for _, v := range pkg.Versions {
			have, err := t.ArchivePlatforms(ctx, pkg.Name, v.Version)
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
				sels = append(sels, Selection{
					Pkg:       pkg,
					Version:   v.Version,
					Reason:    "missing archive",
					Platforms: absent,
				})
			}
		}
	}
	return sels, stats, nil
}
