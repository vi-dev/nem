package publish

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/sync/errgroup"
	"oras.land/oras-go/v2"

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

func Publish(ctx context.Context, dir, ref string, opts Options, r report.Reporter) error {
	if err := ocix.WithoutTagOrDigest(ref); err != nil {
		return err
	}

	findings, err := Lint(dir)
	if err != nil {
		return err
	}
	if len(findings) > 0 {
		return lintError(findings)
	}

	entries, err := enumerate(dir)
	if err != nil {
		return err
	}

	tags := effectiveTags(opts.Tags, nowFunc())

	if opts.DryRun {
		reportPlan(r, ref, entries, tags)
		return nil
	}

	target, err := openTarget(ctx, ref)
	if err != nil {
		return fmt.Errorf("open %s: %w", ref, err)
	}
	if err := ocix.PushEmptyConfig(ctx, target); err != nil {
		return fmt.Errorf("push empty config: %w", err)
	}

	idxEntries, pushed, skipped, err := pushPackages(ctx, target, entries, opts.Force, r)
	if err != nil {
		return err
	}

	idxBytes, idxDesc := ocix.AssembleCatalogIndex(idxEntries)
	if err := ocix.PushBlobAndTag(ctx, target, idxBytes, idxDesc, tags); err != nil {
		return fmt.Errorf("push catalog index: %w", err)
	}

	r.Info("Published %s: %d pushed, %d unchanged, tags %s", ref, pushed, skipped, strings.Join(tags, ", "))
	return nil
}

func effectiveTags(optTags []string, now time.Time) []string {
	base := optTags
	if len(base) == 0 {
		base = []string{"v2"}
	}
	release := "v2." + now.UTC().Format("20060102T150405Z")
	return append(append([]string(nil), base...), release)
}

func lintError(findings []Finding) error {
	msgs := make([]string, len(findings))
	for i, f := range findings {
		msgs[i] = f.String()
	}
	return fmt.Errorf("catalog lint failed:\n%s", strings.Join(msgs, "\n"))
}

func reportPlan(r report.Reporter, ref string, entries []pkgEntry, tags []string) {
	r.Info("Dry run: publish %s (%d packages)", ref, len(entries))
	for _, e := range entries {
		r.Info("  %s %s", e.Name, e.Version)
	}
	r.Info("Tags: %s", strings.Join(tags, ", "))
}

func enumerate(dir string) ([]pkgEntry, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		e, err := readEntry(dir)
		if err != nil {
			return nil, err
		}
		return []pkgEntry{e}, nil
	}

	manifests, err := Manifests(dir)
	if err != nil {
		return nil, err
	}

	var out []pkgEntry
	for _, m := range manifests {
		e, err := readEntry(m.Path)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func readEntry(path string) (pkgEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return pkgEntry{}, fmt.Errorf("read %s: %w", path, err)
	}
	pkg, err := spec.Parse(data)
	if err != nil {
		return pkgEntry{}, fmt.Errorf("parse %s: %w", path, err)
	}
	var version string
	if len(pkg.Versions) > 0 {
		version = pkg.Versions[0].Version
	}
	return pkgEntry{Bytes: data, Name: pkg.Name, Description: pkg.Description, Version: version}, nil
}

func pushPackages(ctx context.Context, target oras.Target, entries []pkgEntry, force bool, r report.Reporter) ([]ocix.CatalogIndexEntry, int, int, error) {
	idxEntries := make([]ocix.CatalogIndexEntry, len(entries))
	var pushed, skipped atomic.Int64

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(min(runtime.NumCPU(), 8))

	for i, e := range entries {
		g.Go(func() error {
			if err := gctx.Err(); err != nil {
				return err
			}
			entry, wasPushed, err := pushOne(gctx, target, e, force, r)
			if err != nil {
				return err
			}
			idxEntries[i] = entry
			if wasPushed {
				pushed.Add(1)
			} else {
				skipped.Add(1)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, 0, 0, err
	}
	return idxEntries, int(pushed.Load()), int(skipped.Load()), nil
}

func pushOne(ctx context.Context, target oras.Target, e pkgEntry, force bool, r report.Reporter) (ocix.CatalogIndexEntry, bool, error) {
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
