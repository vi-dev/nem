package bump

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"sort"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/discover"
	"github.com/vi-dev/nem/internal/fetch"
	"github.com/vi-dev/nem/internal/netx"
	"github.com/vi-dev/nem/internal/report"
	"github.com/vi-dev/nem/internal/spec"
)

var (
	listVersions = discover.List
	digestURL    = fetch.DigestURL
)

type Options struct {
	Packages []spec.Ref
	Backfill int
	DryRun   bool
}

type Row struct {
	Name    string   `json:"name"`
	Current string   `json:"current"`
	Head    string   `json:"head,omitempty"`
	Added   []string `json:"added,omitempty"`
	Error   string   `json:"error,omitempty"`
}

type Result struct {
	Rows    []Row
	Skipped int
}

func (r *Result) Bumped() int {
	return r.count(func(row Row) bool { return row.Error == "" && len(row.Added) > 0 })
}

func (r *Result) UpToDate() int {
	return r.count(func(row Row) bool { return row.Error == "" && len(row.Added) == 0 })
}

func (r *Result) Failed() int {
	return r.count(func(row Row) bool { return row.Error != "" })
}

func (r *Result) count(match func(Row) bool) int {
	n := 0
	for _, row := range r.Rows {
		if match(row) {
			n++
		}
	}
	return n
}

type job struct {
	name     string
	versions []string
}

func Run(ctx context.Context, cat string, opts Options) (*Result, error) {
	jobs, err := plan(opts)
	if err != nil {
		return nil, err
	}
	entry, err := catalog.Open(ctx, cat)
	if err != nil {
		return nil, err
	}
	editor, ok := entry.Catalog.(catalog.Editor)
	if !ok {
		return nil, fmt.Errorf("%s: bump writes a local catalog directory or pkg.yaml", cat)
	}
	names, err := entry.Catalog.PackageNames(ctx)
	if err != nil {
		return nil, err
	}
	for _, j := range jobs {
		if !slices.Contains(names, j.name) {
			return nil, &catalog.PackageNotFoundError{Name: j.name, Catalogs: []string{entry.Name}}
		}
	}
	_, isFile := entry.Catalog.(*catalog.File)
	explicit := len(jobs) > 0 || isFile
	if len(jobs) == 0 {
		jobs = make([]job, len(names))
		for i, name := range names {
			jobs[i] = job{name: name}
		}
	}

	results := make([]*Row, len(jobs))
	var g errgroup.Group
	g.SetLimit(min(runtime.NumCPU(), 8))
	for i, j := range jobs {
		g.Go(func() error {
			results[i] = bumpOne(ctx, editor, j, opts, explicit)
			return nil
		})
	}
	_ = g.Wait()

	res := &Result{Rows: make([]Row, 0, len(results))}
	for _, r := range results {
		if r == nil {
			res.Skipped++
			continue
		}
		res.Rows = append(res.Rows, *r)
	}
	return res, nil
}

func plan(opts Options) ([]job, error) {
	var jobs []job
	for _, ref := range opts.Packages {
		if ref.Version != "" && opts.Backfill > 0 {
			return nil, fmt.Errorf("--package %s and --backfill cannot be combined", ref)
		}
		i := slices.IndexFunc(jobs, func(j job) bool { return j.name == ref.Name })
		if i < 0 {
			jobs = append(jobs, job{name: ref.Name})
			i = len(jobs) - 1
		}
		if ref.Version != "" && !slices.Contains(jobs[i].versions, ref.Version) {
			jobs[i].versions = append(jobs[i].versions, ref.Version)
		}
	}
	return jobs, nil
}

func bumpOne(ctx context.Context, editor catalog.Editor, j job, opts Options, explicit bool) *Row {
	r := report.FromContext(ctx)
	row := &Row{Name: j.name}
	task := r.Task("Bumping " + j.name)
	fail := func(err error) *Row {
		r.Warn("%s: %v", row.Name, err)
		row.Error = err.Error()
		task.Fail("Failed " + row.Name)
		return row
	}
	data, err := editor.ReadManifest(ctx, j.name)
	if err != nil {
		return fail(err)
	}
	pkg, err := spec.Parse(data)
	if err != nil {
		return fail(err)
	}
	current := ""
	if len(pkg.Versions) > 0 {
		current = pkg.Versions[0].Version
	}
	row.Name, row.Current = pkg.Name, current
	if pkg.VersionDiscovery == nil && !explicit {
		r.Debug("Skipping %s: no versionDiscovery", pkg.Name)
		task.Discard()
		return nil
	}
	if err := spec.ValidateEditable(data); err != nil {
		return fail(err)
	}

	task.Segment("discovering")
	var targets []string
	var metas map[string]map[string]string
	var present []string
	if len(j.versions) > 0 {
		metas = map[string]map[string]string{}
		for _, v := range j.versions {
			if pkg.HasEqualVersion(v) {
				present = append(present, v)
				continue
			}
			meta, err := discoverMetaFor(ctx, pkg, v)
			if err != nil {
				return fail(err)
			}
			targets = append(targets, v)
			metas[v] = meta
		}
		sort.Slice(targets, func(a, b int) bool { return spec.CompareVersions(targets[a], targets[b]) < 0 })
	} else if targets, metas, err = candidateVersions(ctx, pkg, opts.Backfill); err != nil {
		return fail(err)
	}
	if len(targets) == 0 {
		row.Head = current
		switch {
		case len(present) > 0:
			task.Done(fmt.Sprintf("%s %s already present", pkg.Name, strings.Join(present, ", ")))
		case explicit:
			task.Done(fmt.Sprintf("%s up to date (%s)", pkg.Name, displayVersion(current)))
		default:
			task.Discard()
		}
		return row
	}

	if opts.DryRun {
		row.Head, row.Added = headAfter(current, targets), targets
		task.Done(resultLine("Would bump", "Would backfill", pkg.Name, current, row.Head, targets))
		return row
	}
	added, head, err := apply(ctx, editor, pkg.Name, data, pkg, targets, metas, task)
	if err != nil {
		return fail(err)
	}
	row.Head, row.Added = head, added
	task.Done(resultLine("Bumped", "Backfilled", pkg.Name, current, head, added))
	return row
}

func headAfter(current string, targets []string) string {
	head := current
	for _, t := range targets {
		if head == "" || spec.CompareVersions(t, head) > 0 {
			head = t
		}
	}
	return head
}

func displayVersion(v string) string {
	if v == "" {
		return "none"
	}
	return v
}

func apply(ctx context.Context, editor catalog.Editor, name string, data []byte, pkg *spec.Package, targets []string, metas map[string]map[string]string, task report.Task) ([]string, string, error) {
	console := report.FromContext(ctx)
	edited := data
	var added []string
	var lastErr error
	notFound, sourceBackfill := false, false
	for _, target := range targets {
		entry, err := buildEntry(ctx, pkg, target, metas[target], task)
		if err != nil {
			lastErr = err
			if _, ok := errors.AsType[*fetch.ArtifactNotFoundError](err); ok {
				notFound = true
			}
			console.Warn("%s %s: %v", pkg.Name, target, err)
			continue
		}
		pos, err := insertPos(edited, target)
		if err != nil {
			return nil, "", err
		}
		if edited, err = spec.InsertVersionAt(edited, entry, pos); err != nil {
			return nil, "", err
		}
		added = append(added, target)
		if pkg.Build != nil && len(pkg.Versions) > 0 && spec.CompareVersions(target, pkg.Versions[0].Version) < 0 {
			console.Warn("%s %s is source-built and its archive is not published yet", pkg.Name, target)
			sourceBackfill = true
		}
	}
	if notFound {
		console.Hint("Upstream release assets may not be uploaded yet; retry later")
	}
	if sourceBackfill {
		console.Hint("Backfilled source versions need `nem catalog build --package <name>@<version> --push` before installs work")
	}
	if len(added) == 0 {
		return nil, "", fmt.Errorf("no versions could be added: %w", lastErr)
	}

	check, err := spec.Parse(edited)
	if err != nil {
		return nil, "", fmt.Errorf("edited manifest is invalid: %w", err)
	}
	if err := check.Validate(); err != nil {
		return nil, "", fmt.Errorf("edited manifest is invalid: %w", err)
	}

	for _, v := range added {
		if !hasVersionEntry(check, v) {
			return nil, "", fmt.Errorf("edited manifest is missing %s", v)
		}
	}
	for i := 1; i < len(check.Versions); i++ {
		if spec.CompareVersions(check.Versions[i-1].Version, check.Versions[i].Version) <= 0 {
			return nil, "", fmt.Errorf("edited manifest is not newest-first at %s", check.Versions[i].Version)
		}
	}
	if err := editor.UpdateManifest(ctx, name, edited); err != nil {
		return nil, "", err
	}
	return added, check.Versions[0].Version, nil
}

func resultLine(bumped, backfilled, name, current, head string, added []string) string {
	switch {
	case head == current:
		word := "versions"
		if len(added) == 1 {
			word = "version"
		}
		return fmt.Sprintf("%s %s (%d %s)", backfilled, name, len(added), word)
	case len(added) > 1:
		return fmt.Sprintf("%s %s %s → %s (%d versions)", bumped, name, displayVersion(current), head, len(added))
	default:
		return fmt.Sprintf("%s %s %s → %s", bumped, name, displayVersion(current), head)
	}
}

func candidateVersions(ctx context.Context, pkg *spec.Package, backfill int) ([]string, map[string]map[string]string, error) {
	discovered, err := listVersions(ctx, pkg)
	if err != nil {
		return nil, nil, err
	}
	metas := map[string]map[string]string{}
	var all []string
	for _, v := range discovered {
		if _, ok := metas[v.Version]; !ok {
			metas[v.Version] = v.Meta
			all = append(all, v.Version)
		}
	}
	sort.Slice(all, func(i, j int) bool { return spec.CompareVersions(all[i], all[j]) > 0 })

	current := ""
	if len(pkg.Versions) > 0 {
		current = pkg.Versions[0].Version
	}
	var out []string
	for i, v := range all {
		newer := current != "" && spec.CompareVersions(v, current) > 0
		if !newer && (backfill <= 0 || i >= backfill) {
			continue
		}
		if pkg.HasEqualVersion(v) {
			continue
		}
		out = append(out, v)
	}
	if current == "" && backfill <= 0 && len(out) == 0 && len(all) > 0 {
		out = []string{all[0]}
	}
	sort.Slice(out, func(i, j int) bool { return spec.CompareVersions(out[i], out[j]) < 0 })
	return out, metas, nil
}

// templatesUseMeta reports whether any of the package's remote references
// expand per-version discovery metadata.
func templatesUseMeta(pkg *spec.Package) bool {
	if strings.Contains(pkg.Artifact.URL, ".Meta") {
		return true
	}
	if g := pkg.Artifact.GitHub; g != nil && strings.Contains(g.Asset, ".Meta") {
		return true
	}
	return pkg.Build != nil && strings.Contains(pkg.Build.Source.URL, ".Meta")
}

func discoverMetaFor(ctx context.Context, pkg *spec.Package, version string) (map[string]string, error) {
	if !templatesUseMeta(pkg) {
		return nil, nil
	}
	discovered, err := listVersions(ctx, pkg)
	if err != nil {
		return nil, err
	}
	for _, v := range discovered {
		if v.Version == version {
			return v.Meta, nil
		}
	}
	return nil, nil
}

// withMeta returns a package whose head version carries the given metadata.
func withMeta(pkg *spec.Package, version string, meta map[string]string) *spec.Package {
	if len(meta) == 0 {
		return pkg
	}
	p := *pkg
	p.Versions = append([]spec.VersionEntry{{Version: version, Meta: meta}}, pkg.Versions...)
	return &p
}

func insertPos(data []byte, version string) (int, error) {
	pkg, err := spec.Parse(data)
	if err != nil {
		return 0, fmt.Errorf("edited manifest is invalid: %w", err)
	}
	for i, v := range pkg.Versions {
		if spec.CompareVersions(version, v.Version) > 0 {
			return i, nil
		}
	}
	return len(pkg.Versions), nil
}

func buildEntry(ctx context.Context, pkg *spec.Package, target string, meta map[string]string, task report.Task) (spec.VersionEntry, error) {
	pkg = withMeta(pkg, target, meta)
	if pkg.Artifact.OCI != "" {
		return buildOCIEntry(ctx, pkg, target, meta, task)
	}

	e := spec.VersionEntry{Version: target, Meta: meta}
	platforms := pkg.SupportedBy()
	e.Sha256 = make(map[string]string, len(platforms))
	for _, plat := range platforms {
		url, err := fetch.UpstreamURL(pkg, target, plat)
		if err != nil {
			return e, err
		}
		task.Segment(fmt.Sprintf("hashing %s %s", target, plat))
		fmeta := fetch.Meta{Name: pkg.Name, Version: target, Platform: plat}
		sum, err := digestURL(ctx, netx.Client(), url, fmeta, task)
		if err != nil {
			return e, err
		}
		e.Sha256[plat.String()] = sum
	}
	return e, nil
}

func buildOCIEntry(ctx context.Context, pkg *spec.Package, target string, meta map[string]string, task report.Task) (spec.VersionEntry, error) {
	e := spec.VersionEntry{Version: target, Meta: meta}
	if pkg.Build == nil {
		return e, nil
	}
	url, err := pkg.BuildSourceURL(target, spec.Current())
	if err != nil {
		return e, err
	}
	task.Segment(fmt.Sprintf("hashing %s source", target))
	fmeta := fetch.Meta{Name: pkg.Name, Version: target, Platform: spec.Current()}
	sum, err := digestURL(ctx, netx.Client(), url, fmeta, task)
	if err != nil {
		if _, ok := errors.AsType[*fetch.ArtifactNotFoundError](err); ok {
			return e, fmt.Errorf("source for %s@%s not found: %s", pkg.Name, target, url)
		}
		return e, err
	}
	e.SourceSha256 = sum
	return e, nil
}

func hasVersionEntry(pkg *spec.Package, v string) bool {
	return slices.ContainsFunc(pkg.Versions, func(e spec.VersionEntry) bool { return e.Version == v })
}
