package publish

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/sync/errgroup"
	"oras.land/oras-go/v2"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/report"
	"github.com/vi-dev/nem/internal/spec"
)

type Options struct {
	Tags []string

	DryRun bool

	Force bool
}

var nowFunc = time.Now

var openTarget = func(ctx context.Context, ref string) (oras.Target, error) {
	t, _, err := ocix.RemoteCatalogRW(ref)
	return t, err
}

func SetTargetOpener(f func(context.Context, string) (oras.Target, error)) (restore func()) {
	prev := openTarget
	openTarget = f
	return func() { openTarget = prev }
}

type pkgEntry struct {
	Bytes       []byte
	Name        string
	Description string
	Version     string
}

type Package struct {
	Name    string
	Version string
	Pushed  bool
}

type Result struct {
	Tags      []string
	Packages  []Package
	Pushed    int
	Unchanged int
}

func Publish(ctx context.Context, cat, ref string, opts Options) (*Result, error) {
	if err := ocix.WithoutTagOrDigest(ref); err != nil {
		return nil, err
	}

	entries, err := enumerate(ctx, cat)
	if err != nil {
		return nil, err
	}

	res := &Result{Tags: effectiveTags(opts.Tags, nowFunc()), Packages: make([]Package, 0, len(entries))}
	for _, e := range entries {
		res.Packages = append(res.Packages, Package{Name: e.Name, Version: e.Version})
	}
	if opts.DryRun {
		return res, nil
	}

	target, err := openTarget(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", ref, err)
	}
	if err := ocix.PushEmptyConfig(ctx, target); err != nil {
		return nil, fmt.Errorf("push empty config: %w", err)
	}

	idxEntries, err := pushPackages(ctx, target, entries, opts.Force, res)
	if err != nil {
		return nil, err
	}

	idxBytes, idxDesc := ocix.AssembleCatalogIndex(idxEntries)
	if err := ocix.PushBlobAndTag(ctx, target, idxBytes, idxDesc, res.Tags); err != nil {
		return nil, fmt.Errorf("push catalog index: %w", err)
	}
	return res, nil
}

func effectiveTags(optTags []string, now time.Time) []string {
	base := optTags
	if len(base) == 0 {
		base = []string{"v2"}
	}
	release := "v2." + now.UTC().Format("20060102T150405Z")
	return append(append([]string(nil), base...), release)
}

func enumerate(ctx context.Context, cat string) ([]pkgEntry, error) {
	entry, err := catalog.Open(ctx, cat)
	if err != nil {
		return nil, err
	}
	switch entry.Catalog.(type) {
	case *catalog.Dir, *catalog.File:
	default:
		return nil, fmt.Errorf("%s: publish reads a local catalog directory or pkg.yaml", cat)
	}
	names, err := entry.Catalog.PackageNames(ctx)
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("no package manifests under %s", cat)
	}
	out := make([]pkgEntry, 0, len(names))
	for _, name := range names {
		data, err := entry.Catalog.ReadManifest(ctx, name)
		if err != nil {
			return nil, err
		}
		pkg, err := spec.Parse(data)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		var version string
		if len(pkg.Versions) > 0 {
			version = pkg.Versions[0].Version
		}
		out = append(out, pkgEntry{Bytes: data, Name: pkg.Name, Description: pkg.Description, Version: version})
	}
	return out, nil
}

func pushPackages(ctx context.Context, target oras.Target, entries []pkgEntry, force bool, res *Result) ([]ocix.CatalogIndexEntry, error) {
	idxEntries := make([]ocix.CatalogIndexEntry, len(entries))
	var pushed, skipped atomic.Int64

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(min(runtime.NumCPU(), 8))

	for i, e := range entries {
		g.Go(func() error {
			if err := gctx.Err(); err != nil {
				return err
			}
			entry, wasPushed, err := pushOne(gctx, target, e, force)
			if err != nil {
				return err
			}
			idxEntries[i] = entry
			res.Packages[i].Pushed = wasPushed
			if wasPushed {
				pushed.Add(1)
			} else {
				skipped.Add(1)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	res.Pushed = int(pushed.Load())
	res.Unchanged = int(skipped.Load())
	return idxEntries, nil
}

func pushOne(ctx context.Context, target oras.Target, e pkgEntry, force bool) (ocix.CatalogIndexEntry, bool, error) {
	r := report.FromContext(ctx)

	_, desc, err := ocix.PackageManifest(e.Bytes)
	if err != nil {
		return ocix.CatalogIndexEntry{}, false, fmt.Errorf("compute manifest descriptor for %s: %w", e.Name, err)
	}

	if !force {
		exists, err := target.Exists(ctx, desc)
		if err != nil {
			return ocix.CatalogIndexEntry{}, false, fmt.Errorf("check %s: %w", e.Name, err)
		}
		if exists {
			r.Info("Skip %s %s (unchanged)", e.Name, e.Version)
			return indexEntry(e, desc), false, nil
		}
	}

	pushed, err := ocix.PushPackageManifest(ctx, target, e.Bytes)
	if err != nil {
		return ocix.CatalogIndexEntry{}, false, fmt.Errorf("push %s: %w", e.Name, err)
	}
	r.Info("Push %s %s", e.Name, e.Version)
	return indexEntry(e, pushed), true, nil
}

func indexEntry(e pkgEntry, desc ocispec.Descriptor) ocix.CatalogIndexEntry {
	return ocix.CatalogIndexEntry{Manifest: desc, Title: e.Name, Description: e.Description, Version: e.Version}
}
