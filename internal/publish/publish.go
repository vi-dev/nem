package publish

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/vi-dev/clix/console"
	"golang.org/x/sync/errgroup"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/errdef"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/spec"
)

const pushConcurrency = 16

const (
	StatusNew       = "new"
	StatusUpdated   = "updated"
	StatusUnchanged = "unchanged"
	StatusRemoved   = "removed"
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
	Files       spec.Files
	Name        string
	Description string
	Version     string
	Schema      int
	manifest    ocispec.Descriptor
	status      string
}

type Package struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Status  string `json:"status"`
}

type Result struct {
	Tags      []string
	Index     string
	Packages  []Package
	Pushed    int
	Unchanged int
	Removed   int
}

type targetIndex struct {
	found    bool
	digests  map[digest.Digest]bool
	versions map[string]string
}

func Publish(ctx context.Context, cat, ref string, opts Options) (*Result, error) {
	if err := ocix.WithoutTagOrDigest(ref); err != nil {
		return nil, err
	}

	entries, err := enumerate(ctx, cat)
	if err != nil {
		return nil, err
	}

	target, err := openTarget(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", ref, err)
	}
	res := &Result{Tags: effectiveTags(opts.Tags, nowFunc())}
	removed, err := compare(ctx, target, ref, baseTags(opts.Tags), entries, opts.Force, res)
	if err != nil {
		return nil, err
	}
	if opts.DryRun {
		_, idxDesc := assembleIndex(entries)
		res.Index = idxDesc.Digest.String()
		return res, nil
	}

	if err := ocix.PushEmptyConfig(ctx, target); err != nil {
		return nil, fmt.Errorf("push empty config: %w", err)
	}
	if err := pushPackages(ctx, target, entries, opts.Force); err != nil {
		return nil, err
	}
	res.Packages = packagesOf(entries, removed)
	res.count(opts.Force)

	idxBytes, idxDesc := assembleIndex(entries)
	if err := ocix.PushBlobAndTag(ctx, target, idxBytes, idxDesc, res.Tags); err != nil {
		return nil, fmt.Errorf("push catalog index: %w", err)
	}
	res.Index = idxDesc.Digest.String()
	return res, nil
}

func assembleIndex(entries []pkgEntry) ([]byte, ocispec.Descriptor) {
	idxEntries := make([]ocix.CatalogIndexEntry, 0, len(entries))
	for _, e := range entries {
		idxEntries = append(idxEntries, indexEntry(e))
	}
	return ocix.AssembleCatalogIndex(idxEntries)
}

func (r *Result) count(force bool) {
	r.Pushed, r.Unchanged, r.Removed = 0, 0, 0
	for _, p := range r.Packages {
		switch {
		case p.Status == StatusRemoved:
			r.Removed++
		case force || p.Status != StatusUnchanged:
			r.Pushed++
		default:
			r.Unchanged++
		}
	}
}

func baseTags(optTags []string) []string {
	if len(optTags) == 0 {
		return []string{"v3"}
	}
	return optTags
}

func effectiveTags(optTags []string, now time.Time) []string {
	release := "v3." + now.UTC().Format("20060102T150405Z")
	return append(append([]string(nil), baseTags(optTags)...), release)
}

func fetchTargetIndex(ctx context.Context, target oras.ReadOnlyTarget, tags []string) (targetIndex, error) {
	index := targetIndex{digests: map[digest.Digest]bool{}, versions: map[string]string{}}
	for _, tag := range tags {
		idx, _, err := ocix.FetchCatalogIndex(ctx, target, tag)
		if errors.Is(err, errdef.ErrNotFound) {
			continue
		}
		if err != nil {
			return targetIndex{}, err
		}
		index.found = true
		for _, m := range idx.Manifests {
			index.digests[m.Digest] = true
			if title := m.Annotations[ocix.AnnotationTitle]; title != "" {
				if _, seen := index.versions[title]; !seen {
					index.versions[title] = m.Annotations[ocix.AnnotationVersion]
				}
			}
		}
		return index, nil
	}
	return index, nil
}

func classify(entries []pkgEntry, index targetIndex) []Package {
	present := make(map[string]bool, len(entries))
	for i := range entries {
		e := &entries[i]
		present[e.Name] = true
		_, known := index.versions[e.Name]
		switch {
		case index.digests[e.manifest.Digest]:
			e.status = StatusUnchanged
		case known:
			e.status = StatusUpdated
		default:
			e.status = StatusNew
		}
	}
	var removed []Package
	for name, version := range index.versions {
		if !present[name] {
			removed = append(removed, Package{Name: name, Version: version, Status: StatusRemoved})
		}
	}
	sort.Slice(removed, func(i, j int) bool { return removed[i].Name < removed[j].Name })
	return removed
}

func packagesOf(entries []pkgEntry, removed []Package) []Package {
	out := make([]Package, 0, len(entries)+len(removed))
	for _, e := range entries {
		out = append(out, Package{Name: e.Name, Version: e.Version, Status: e.status})
	}
	out = append(out, removed...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
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
		files, err := entry.Catalog.ReadFiles(ctx, name)
		if err != nil {
			return nil, err
		}
		pkg, err := spec.ParseFiles(files)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		_, desc, err := ocix.PackageManifest(files)
		if err != nil {
			return nil, fmt.Errorf("compute manifest descriptor for %s: %w", name, err)
		}
		version := pkg.Latest()
		out = append(out, pkgEntry{Files: files, Name: pkg.Name, Description: pkg.Description, Version: version, Schema: pkg.Schema, manifest: desc})
	}
	return out, nil
}

func pushPackages(ctx context.Context, target oras.Target, entries []pkgEntry, force bool) error {

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(pushConcurrency)

	for i := range entries {
		e := &entries[i]
		g.Go(func() error {
			if err := gctx.Err(); err != nil {
				return err
			}
			return pushOne(gctx, target, e, force)
		})
	}
	return g.Wait()
}

func pushOne(ctx context.Context, target oras.Target, e *pkgEntry, force bool) error {
	r := console.FromContext(ctx)
	if !force && e.status == StatusUnchanged {
		r.Debugf("Skipped %s %s (unchanged)", e.Name, e.Version)
		return nil
	}

	task := r.Start(fmt.Sprintf("Pushing %s %s", e.Name, e.Version))
	task.Phase("pushing manifest")
	if _, err := ocix.PushPackageManifest(ctx, target, e.Files); err != nil {
		task.Fail(fmt.Sprintf("Failed to push %s %s", e.Name, e.Version))
		return fmt.Errorf("push %s: %w", e.Name, err)
	}
	task.Done(fmt.Sprintf("Pushed %s %s", e.Name, e.Version))
	return nil
}

func indexEntry(e pkgEntry) ocix.CatalogIndexEntry {
	return ocix.CatalogIndexEntry{Manifest: e.manifest, Title: e.Name, Description: e.Description, Version: e.Version, Schema: e.Schema}
}

func compare(ctx context.Context, target oras.ReadOnlyTarget, ref string, tags []string, entries []pkgEntry, force bool, res *Result) ([]Package, error) {
	r := console.FromContext(ctx)
	with := ref + ":" + strings.Join(tags, ", ")
	task := r.Start("Comparing with " + with)
	index, err := fetchTargetIndex(ctx, target, tags)
	if err != nil {
		task.Fail("Failed to compare with " + with)
		return nil, err
	}
	removed := classify(entries, index)
	suffix := ""
	if !index.found {
		task.Phase("checking manifests")
		if err := markExisting(ctx, target, entries); err != nil {
			task.Fail("Failed to compare with " + with)
			return nil, err
		}
		suffix = " (tag not found)"
	}
	res.Packages = packagesOf(entries, removed)
	res.count(force)
	task.Done("Compared with " + with + ": " + compareSummary(res) + suffix)
	return removed, nil
}

func markExisting(ctx context.Context, target oras.ReadOnlyTarget, entries []pkgEntry) error {
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(pushConcurrency)
	for i := range entries {
		e := &entries[i]
		g.Go(func() error {
			exists, err := target.Exists(gctx, e.manifest)
			if err != nil {
				return fmt.Errorf("check %s: %w", e.Name, err)
			}
			if exists {
				e.status = StatusUnchanged
			}
			return nil
		})
	}
	return g.Wait()
}

func compareSummary(res *Result) string {
	var groups []string
	if res.Pushed > 0 {
		groups = append(groups, fmt.Sprintf("%d to push", res.Pushed))
	}
	if res.Removed > 0 {
		groups = append(groups, fmt.Sprintf("%d removed", res.Removed))
	}
	if res.Unchanged > 0 {
		groups = append(groups, fmt.Sprintf("%d unchanged", res.Unchanged))
	}
	return strings.Join(groups, ", ")
}
