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
		sel, err := selection(pkg, version, "forced")
		if err != nil {
			return nil, err
		}
		sels = append(sels, sel)
	}
	return sels, nil
}

func SelectDeps(ctx context.Context, t *Target, configured *catalog.Set, roots []Selection) ([]Selection, error) {
	names := map[string]bool{}
	var firsts []Selection
	for _, r := range roots {
		if names[r.Pkg.Name] {
			continue
		}
		names[r.Pkg.Name] = true
		firsts = append(firsts, r)
	}
	catalogs := configured.Prepend(t.Entry)
	var sels []Selection
	for _, root := range firsts {
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
		if len(pkg.Versions) == 0 || !buildable(pkg.LatestRecipe()) {
			continue
		}
		sel, err := selection(pkg, pkg.Versions[0].Version, "latest")
		if err != nil {
			return nil, err
		}
		sels = append(sels, sel)
	}
	return sels, nil
}

func buildable(r *spec.Recipe) bool { return r.Artifact.OCI != "" && r.Build != nil }

func selection(pkg *spec.Package, version, reason string) (Selection, error) {
	r, err := pkg.Recipe(version)
	if err != nil {
		return Selection{}, err
	}
	return Selection{Pkg: pkg, Version: version, Reason: reason, Platforms: r.SupportedBy()}, nil
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

func selectDependencies(ctx context.Context, t *Target, root Selection, catalogs *catalog.Set) ([]Selection, error) {
	r, err := root.Pkg.Recipe(root.Version)
	if err != nil {
		return nil, err
	}
	deps := r.Deps
	if r.Build != nil {
		deps = append(append([]spec.Dep(nil), deps...), r.Build.Deps...)
	}
	if len(deps) == 0 {
		return nil, nil
	}
	resolvedDeps, err := resolve.Dependencies(ctx, root.Pkg.Name, r.Platforms, deps, catalogs)
	if err != nil {
		return nil, err
	}

	plat := spec.Current()
	var sels []Selection
	for _, e := range resolvedDeps.Entries {
		if !slices.Contains(e.Platforms, plat.String()) {
			continue
		}

		p := resolvedDeps.Pkgs[e.Name]
		if p == nil {
			continue
		}
		pr, err := p.Recipe(e.Version)
		if err != nil {
			return nil, err
		}
		if pr.Artifact.OCI == "" {
			continue
		}
		pkg, _, err := t.Entry.Catalog.Package(ctx, e.Name)
		if err != nil {
			if _, ok := errors.AsType[*catalog.PackageNotFoundError](err); ok {
				continue
			}
			return nil, err
		}
		cr, err := pkg.Recipe(e.Version)
		if err != nil {
			return nil, err
		}
		if cr.Build == nil {
			continue
		}
		exists, err := t.ArchiveExists(ctx, e.Name, e.Version, plat)
		if err != nil {
			return nil, err
		}
		if exists {
			continue
		}
		sel, err := selection(pkg, e.Version, "dep of "+root.Pkg.Name)
		if err != nil {
			return nil, err
		}
		sels = append(sels, sel)
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
			if !buildable(pkg.LatestRecipe()) {
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
		if !buildable(pkg.LatestRecipe()) {
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
		for _, v := range c.versions {
			if seen[c.pkg.Name+"@"+v] {
				continue
			}
			seen[c.pkg.Name+"@"+v] = true
			r, err := c.pkg.Recipe(v)
			if err != nil {
				return nil, MissingStats{}, err
			}
			if !buildable(r) {
				continue
			}
			plats := r.SupportedBy()
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
