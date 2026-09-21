package diff

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"slices"

	"github.com/opencontainers/go-digest"
	"golang.org/x/sync/errgroup"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/spec"
)

const (
	StatusNew     = "new"
	StatusUpdated = "updated"
	StatusRemoved = "removed"
)

type Change struct {
	Name   string   `json:"name"`
	Status string   `json:"status"`
	Build  bool     `json:"build"`
	Base   string   `json:"base,omitempty"`
	Target string   `json:"target,omitempty"`
	Diff   []string `json:"diff"`
}

type Result struct {
	Changes   []Change
	Unchanged int
}

func (r *Result) Count(status string) int {
	n := 0
	for _, c := range r.Changes {
		if c.Status == status {
			n++
		}
	}
	return n
}

func Compare(ctx context.Context, base, target string, packages []string) (*Result, error) {
	baseEntry, err := catalog.Open(ctx, base)
	if err != nil {
		return nil, err
	}
	targetEntry, err := catalog.Open(ctx, target)
	if err != nil {
		return nil, err
	}
	names, err := compareNames(ctx, packages, baseEntry, targetEntry)
	if err != nil {
		return nil, err
	}

	type outcome struct {
		change  Change
		changed bool
	}
	outcomes := make([]outcome, len(names))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(min(runtime.NumCPU(), 8))
	for i, name := range names {
		g.Go(func() error {
			c, changed, err := diffOne(gctx, baseEntry, targetEntry, name)
			if err != nil {
				return err
			}
			outcomes[i] = outcome{change: c, changed: changed}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	res := &Result{Changes: make([]Change, 0, len(names))}
	for _, o := range outcomes {
		if !o.changed {
			res.Unchanged++
			continue
		}
		res.Changes = append(res.Changes, o.change)
	}
	return res, nil
}

func compareNames(ctx context.Context, packages []string, base, target catalog.Entry) ([]string, error) {
	var names, fileNames []string
	for _, e := range []catalog.Entry{base, target} {
		found, err := e.Catalog.PackageNames(ctx)
		if err != nil {
			return nil, err
		}
		if len(found) == 0 {
			return nil, fmt.Errorf("no package manifests under %s", e.Name)
		}
		if _, ok := e.Catalog.(*catalog.File); ok {
			fileNames = append(fileNames, found...)
		}
		names = append(names, found...)
	}
	switch {
	case len(packages) > 0:
		names = slices.Clone(packages)
	case len(fileNames) > 0:
		names = fileNames
	}
	slices.Sort(names)
	return slices.Compact(names), nil
}

type side struct {
	pkg    *spec.Package
	digest digest.Digest
}

func loadSide(ctx context.Context, entry catalog.Entry, name string) (*side, error) {
	data, err := entry.Catalog.ReadManifest(ctx, name)
	if _, ok := errors.AsType[*catalog.PackageNotFoundError](err); ok {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	pkg, err := spec.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: package %s: %w", entry.Name, name, err)
	}
	if pkg.Name != name {
		return nil, fmt.Errorf("%s: package %s: manifest declares name %q", entry.Name, name, pkg.Name)
	}
	_, desc, err := ocix.PackageManifest(data)
	if err != nil {
		return nil, fmt.Errorf("%s: package %s: %w", entry.Name, name, err)
	}
	return &side{pkg: pkg, digest: desc.Digest}, nil
}

func diffOne(ctx context.Context, base, target catalog.Entry, name string) (Change, bool, error) {
	b, err := loadSide(ctx, base, name)
	if err != nil {
		return Change{}, false, err
	}
	t, err := loadSide(ctx, target, name)
	if err != nil {
		return Change{}, false, err
	}
	c := Change{Name: name, Diff: []string{}}
	switch {
	case b == nil && t == nil:
		return Change{}, false, &catalog.PackageNotFoundError{Name: name, Catalogs: []string{base.Name, target.Name}}
	case b != nil && t != nil && b.digest == t.digest:
		return Change{}, false, nil
	case t == nil:
		c.Status = StatusNew
		c.Build = b.pkg.Build != nil
		c.Base = latest(b.pkg)
		c.Diff = versionsOf(b.pkg)
	case b == nil:
		c.Status = StatusRemoved
		c.Build = t.pkg.Build != nil
		c.Target = latest(t.pkg)
	default:
		c.Status = StatusUpdated
		c.Build = b.pkg.Build != nil
		c.Base = latest(b.pkg)
		c.Target = latest(t.pkg)
		c.Diff = addedOrLatest(b.pkg, t.pkg)
	}
	return c, true, nil
}

func latest(pkg *spec.Package) string {
	if len(pkg.Versions) == 0 {
		return ""
	}
	return pkg.Versions[0].Version
}

func versionsOf(pkg *spec.Package) []string {
	out := make([]string, 0, len(pkg.Versions))
	for _, v := range pkg.Versions {
		out = append(out, v.Version)
	}
	return out
}

func addedOrLatest(base, target *spec.Package) []string {
	var added []string
	for _, v := range base.Versions {
		if !target.HasEqualVersion(v.Version) {
			added = append(added, v.Version)
		}
	}
	if len(added) == 0 && len(base.Versions) > 0 {
		added = append(added, base.Versions[0].Version)
	}
	return added
}
