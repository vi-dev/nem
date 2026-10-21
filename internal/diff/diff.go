package diff

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"

	"github.com/vi-dev/clix/console"
	"golang.org/x/sync/errgroup"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/spec"
)

const compareConcurrency = 16

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
	// Excluded lists versions base excludes that target does not; Unexcluded
	// lists versions target excludes that base offers again.
	Excluded   []string `json:"excluded,omitempty"`
	Unexcluded []string `json:"unexcluded,omitempty"`
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

type plan struct {
	name             string
	inBase, inTarget bool
	same             bool
}

func Compare(ctx context.Context, base, target string, packages []string) (*Result, error) {
	baseEntry, err := catalog.OpenIndex(ctx, base)
	if err != nil {
		return nil, err
	}
	targetEntry, err := catalog.OpenIndex(ctx, target)
	if err != nil {
		return nil, err
	}
	names, err := compareNames(ctx, packages, baseEntry, targetEntry)
	if err != nil {
		return nil, err
	}

	plans, err := planAll(ctx, baseEntry, targetEntry, names)
	if err != nil {
		return nil, err
	}
	var needBase, needTarget int
	for _, p := range plans {
		if p.same {
			continue
		}
		if p.inBase {
			needBase++
		}
		if p.inTarget {
			needTarget++
		}
	}
	pulls, pctx := errgroup.WithContext(ctx)
	pulls.Go(func() error {
		var err error
		baseEntry, err = pullIfMostNeeded(pctx, baseEntry, needBase)
		return err
	})
	pulls.Go(func() error {
		var err error
		targetEntry, err = pullIfMostNeeded(pctx, targetEntry, needTarget)
		return err
	})
	if err := pulls.Wait(); err != nil {
		return nil, err
	}

	type outcome struct {
		change  Change
		changed bool
	}
	outcomes := make([]outcome, len(names))
	subject := console.Plural(len(names), "package")
	task := console.FromContext(ctx).Start("Comparing " + subject)
	task.Phase("comparing")
	var done atomic.Int64
	total := int64(len(names))
	task.Progress(0, total, console.Items)
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(compareConcurrency)
	for i, p := range plans {
		g.Go(func() error {
			if !p.same {
				c, changed, err := loadChange(gctx, baseEntry, targetEntry, p)
				if err != nil {
					return err
				}
				outcomes[i] = outcome{change: c, changed: changed}
			}
			task.Progress(done.Add(1), total, console.Items)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		task.Fail("Failed to compare " + subject)
		return nil, err
	}
	task.Done("Compared " + subject)

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

func planAll(ctx context.Context, base, target catalog.Entry, names []string) ([]plan, error) {
	plans := make([]plan, len(names))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(compareConcurrency)
	for i, name := range names {
		g.Go(func() error {
			bd, inBase, err := digestOf(gctx, base, name)
			if err != nil {
				return err
			}
			td, inTarget, err := digestOf(gctx, target, name)
			if err != nil {
				return err
			}
			if !inBase && !inTarget {
				return &catalog.PackageNotFoundError{Name: name, Catalogs: []string{base.Name, target.Name}}
			}
			plans[i] = plan{name: name, inBase: inBase, inTarget: inTarget, same: inBase && inTarget && bd == td}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return plans, nil
}

func pullIfMostNeeded(ctx context.Context, entry catalog.Entry, need int) (catalog.Entry, error) {
	if entry.Ref == "" || need == 0 {
		return entry, nil
	}
	names, err := entry.Catalog.PackageNames(ctx)
	if err != nil {
		return catalog.Entry{}, err
	}
	if need*4 <= len(names) {
		return entry, nil
	}
	return catalog.Open(ctx, entry.Name)
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

func digestOf(ctx context.Context, entry catalog.Entry, name string) (string, bool, error) {
	d, err := entry.Catalog.Digest(ctx, name)
	if _, ok := errors.AsType[*catalog.PackageNotFoundError](err); ok {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("%s: package %s: %w", entry.Name, name, err)
	}
	return d, true, nil
}

type side struct {
	pkg    *spec.Package
	digest string
}

func loadSide(ctx context.Context, entry catalog.Entry, name string) (*side, error) {
	files, err := entry.Catalog.ReadFiles(ctx, name)
	if err != nil {
		return nil, err
	}
	pkg, err := spec.ParseFiles(files)
	if err != nil {
		return nil, fmt.Errorf("%s: package %s: %w", entry.Name, name, err)
	}
	if pkg.Name != name {
		return nil, fmt.Errorf("%s: package %s: manifest declares name %q", entry.Name, name, pkg.Name)
	}
	_, desc, err := ocix.PackageManifest(files)
	if err != nil {
		return nil, fmt.Errorf("%s: package %s: %w", entry.Name, name, err)
	}
	return &side{pkg: pkg, digest: desc.Digest.String()}, nil
}

func loadChange(ctx context.Context, base, target catalog.Entry, p plan) (Change, bool, error) {
	var b, t *side
	var err error
	if p.inBase {
		if b, err = loadSide(ctx, base, p.name); err != nil {
			return Change{}, false, err
		}
	}
	if p.inTarget {
		if t, err = loadSide(ctx, target, p.name); err != nil {
			return Change{}, false, err
		}
	}
	c := Change{Name: p.name, Diff: []string{}}
	switch {
	case t == nil:
		c.Status = StatusNew
		c.Base = latest(b.pkg)
		c.Diff = versionsOf(b.pkg)
		c.Build = anyBuilds(b.pkg, c.Diff)
		c.Excluded = newlyExcluded(b.pkg, nil)
	case b == nil:
		c.Status = StatusRemoved
		c.Build = anyBuilds(t.pkg, versionsOf(t.pkg))
		c.Target = latest(t.pkg)
	case b.digest == t.digest:
		return Change{}, false, nil
	default:
		c.Status = StatusUpdated
		c.Base = latest(b.pkg)
		c.Target = latest(t.pkg)
		c.Diff = changedVersions(b.pkg, t.pkg)
		c.Build = anyBuilds(b.pkg, c.Diff)
		c.Excluded = newlyExcluded(b.pkg, t.pkg)
		c.Unexcluded = newlyExcluded(t.pkg, b.pkg)
	}
	return c, true, nil
}

func anyBuilds(pkg *spec.Package, versions []string) bool {
	for _, v := range versions {
		if r, err := pkg.Recipe(v); err == nil && r.Build != nil {
			return true
		}
	}
	return false
}

func latest(pkg *spec.Package) string {
	return pkg.Latest()
}

func versionsOf(pkg *spec.Package) []string {
	offered := pkg.Offered()
	out := make([]string, 0, len(offered))
	for _, v := range offered {
		out = append(out, v.Version)
	}
	return out
}

func newlyExcluded(base, target *spec.Package) []string {
	var out []string
	for _, v := range base.Versions {
		if v.Excluded != nil && (target == nil || target.Exclusion(v.Version) == nil) {
			out = append(out, v.Version)
		}
	}
	return out
}

func changedVersions(base, target *spec.Package) []string {
	var out []string
	for _, v := range base.Offered() {
		if !target.HasEqualVersion(v.Version) || target.Exclusion(v.Version) != nil {
			out = append(out, v.Version)
			continue
		}
		br, berr := base.Recipe(v.Version)
		tr, terr := target.Recipe(v.Version)
		if berr != nil || terr != nil || !br.Equal(tr) {
			out = append(out, v.Version)
		}
	}
	if latest := base.Latest(); len(out) == 0 && latest != "" {
		out = append(out, latest)
	}
	return out
}
