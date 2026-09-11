package bump

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/vi-dev/nem/internal/discover"
	"github.com/vi-dev/nem/internal/fetch"
	"github.com/vi-dev/nem/internal/fsx"
	"github.com/vi-dev/nem/internal/netx"
	"github.com/vi-dev/nem/internal/publish"
	"github.com/vi-dev/nem/internal/report"
	"github.com/vi-dev/nem/internal/spec"
)

var (
	listVersions = discover.List
	digestURL    = fetch.DigestURL
)

type Options struct {
	Version  string
	Backfill int
	JSON     bool
}

type Row struct {
	Name    string   `json:"name"`
	Path    string   `json:"path"`
	Current string   `json:"current"`
	Head    string   `json:"head,omitempty"`
	Added   []string `json:"added,omitempty"`
	Error   string   `json:"error,omitempty"`
}

func Run(ctx context.Context, target string, opts Options, console *report.Console) error {
	if opts.Version != "" && opts.Backfill > 0 {
		return fmt.Errorf("--version and --backfill cannot be combined")
	}
	info, err := os.Stat(target)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if opts.Version != "" {
			return fmt.Errorf("--version needs a single pkg.yaml target, not a directory")
		}
		return sweep(ctx, target, opts.Backfill, opts.JSON, console)
	}
	path := target
	data, pkg, current, err := load(path)
	if err != nil {
		return err
	}
	if err := spec.ValidateEditable(data); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	row := Row{Name: pkg.Name, Path: path, Current: current}
	emit := func() error {
		if !opts.JSON {
			return nil
		}
		return console.JSON([]Row{row})
	}

	var targets []string
	var metas map[string]map[string]string
	if opts.Version != "" {
		if pkg.HasEqualVersion(opts.Version) {
			console.Success("%s %s already present", pkg.Name, opts.Version)
			row.Head = current
			return emit()
		}
		targets = []string{opts.Version}
		meta, err := discoverMetaFor(ctx, pkg, opts.Version)
		if err != nil {
			return err
		}
		metas = map[string]map[string]string{opts.Version: meta}
	} else {
		if targets, metas, err = candidateVersions(ctx, pkg, opts.Backfill); err != nil {
			return err
		}
		if len(targets) == 0 {
			console.Success("%s up to date (%s)", pkg.Name, displayVersion(current))
			row.Head = current
			return emit()
		}
	}

	added, head, err := apply(ctx, path, data, pkg, targets, metas, console)
	if err != nil {
		return err
	}
	printResult(console, pkg.Name, current, head, added)
	row.Head, row.Added = head, added
	return emit()
}

func sweep(ctx context.Context, dir string, backfill int, jsonOut bool, console *report.Console) error {
	paths, err := publish.ManifestPaths(dir)
	if err != nil {
		return err
	}
	results := make([]*Row, len(paths))
	var g errgroup.Group

	g.SetLimit(4)
	for i, path := range paths {
		g.Go(func() error {
			results[i] = sweepOne(ctx, path, backfill, console)
			return nil
		})
	}
	_ = g.Wait()

	var rows []Row
	var bumped, upToDate, failed, skipped int
	for _, r := range results {
		if r == nil {
			skipped++
			continue
		}
		rows = append(rows, *r)
		switch {
		case r.Error != "":
			failed++
		case len(r.Added) > 0:
			bumped++
		default:
			upToDate++
		}
	}
	if jsonOut {
		if err := console.JSON(rows); err != nil {
			return err
		}
	}
	console.Success("Checked %d packages: %d bumped, %d up to date, %d failed, %d without discovery",
		len(paths), bumped, upToDate, failed, skipped)
	return nil
}

func sweepOne(ctx context.Context, path string, backfill int, console *report.Console) *Row {
	row := &Row{Name: filepath.Base(filepath.Dir(path)), Path: path}
	fail := func(err error) *Row {
		console.Warn("%s: %v", row.Name, err)
		row.Error = err.Error()
		return row
	}
	data, pkg, current, err := load(path)
	if err != nil {
		return fail(err)
	}
	row.Name, row.Current = pkg.Name, current
	if pkg.VersionDiscovery == nil {
		console.Debug("Skipping %s: no versionDiscovery", pkg.Name)
		return nil
	}
	if err := spec.ValidateEditable(data); err != nil {
		return fail(err)
	}
	targets, metas, err := candidateVersions(ctx, pkg, backfill)
	if err != nil {
		return fail(err)
	}
	if len(targets) == 0 {
		console.Debug("%s up to date (%s)", pkg.Name, displayVersion(current))
		row.Head = current
		return row
	}
	added, head, err := apply(ctx, path, data, pkg, targets, metas, console)
	if err != nil {
		return fail(err)
	}
	row.Head, row.Added = head, added
	printResult(console, pkg.Name, current, head, added)
	return row
}

func load(path string) ([]byte, *spec.Package, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, "", err
	}
	pkg, err := spec.Parse(data)
	if err != nil {
		return nil, nil, "", fmt.Errorf("%s: %w", path, err)
	}
	current := ""
	if len(pkg.Versions) > 0 {
		current = pkg.Versions[0].Version
	}
	return data, pkg, current, nil
}

func displayVersion(v string) string {
	if v == "" {
		return "none"
	}
	return v
}

func apply(ctx context.Context, path string, data []byte, pkg *spec.Package, targets []string, metas map[string]map[string]string, console *report.Console) ([]string, string, error) {
	edited := data
	var added []string
	var lastErr error
	notFound, sourceBackfill := false, false
	for _, target := range targets {
		entry, err := buildEntry(ctx, pkg, target, metas[target], console)
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
		console.Hint("Backfilled source versions need `nem catalog build --version <v> --push` before installs work")
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
	if err := fsx.WriteAtomic(path, edited, 0o644); err != nil {
		return nil, "", err
	}
	return added, check.Versions[0].Version, nil
}

func printResult(console *report.Console, name, current, head string, added []string) {
	switch {
	case head == current:
		word := "versions"
		if len(added) == 1 {
			word = "version"
		}
		console.Success("Backfilled %s (%d %s)", name, len(added), word)
	case len(added) > 1:
		console.Success("Bumped %s %s → %s (%d versions)", name, displayVersion(current), head, len(added))
	default:
		console.Success("Bumped %s %s → %s", name, displayVersion(current), head)
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

func buildEntry(ctx context.Context, pkg *spec.Package, target string, meta map[string]string, console *report.Console) (spec.VersionEntry, error) {
	pkg = withMeta(pkg, target, meta)
	if pkg.Artifact.OCI != "" {
		return buildOCIEntry(ctx, pkg, target, meta, console)
	}

	e := spec.VersionEntry{Version: target, Meta: meta}
	platforms := pkg.SupportedBy()

	sums := make([]string, len(platforms))
	g, gctx := errgroup.WithContext(ctx)
	for i, plat := range platforms {
		g.Go(func() error {
			url, err := fetch.UpstreamURL(pkg, target, plat)
			if err != nil {
				return err
			}
			subject := pkg.Name + " " + target + " " + plat.String()
			task := console.Task("Hashing " + subject)
			fmeta := fetch.Meta{Name: pkg.Name, Version: target, Platform: plat}
			sum, err := digestURL(gctx, netx.Client(), url, fmeta, task)
			if err != nil {
				task.Fail("Download failed for " + subject)
				return err
			}
			task.Done("Hashed " + subject)
			sums[i] = sum
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return e, err
	}
	e.Sha256 = make(map[string]string, len(platforms))
	for i, plat := range platforms {
		e.Sha256[plat.String()] = sums[i]
	}
	return e, nil
}

func buildOCIEntry(ctx context.Context, pkg *spec.Package, target string, meta map[string]string, console *report.Console) (spec.VersionEntry, error) {
	e := spec.VersionEntry{Version: target, Meta: meta}
	if pkg.Build == nil {
		return e, nil
	}
	url, err := pkg.BuildSourceURL(target, spec.Current())
	if err != nil {
		return e, err
	}
	subject := pkg.Name + " " + target + " source"
	task := console.Task("Hashing " + subject)
	fmeta := fetch.Meta{Name: pkg.Name, Version: target, Platform: spec.Current()}
	sum, err := digestURL(ctx, netx.Client(), url, fmeta, task)
	if err != nil {
		task.Fail("Download failed for " + subject)

		if _, ok := errors.AsType[*fetch.ArtifactNotFoundError](err); ok {
			return e, fmt.Errorf("source for %s@%s not found: %s", pkg.Name, target, url)
		}
		return e, err
	}
	task.Done("Hashed " + subject)
	e.SourceSha256 = sum
	return e, nil
}

func hasVersionEntry(pkg *spec.Package, v string) bool {
	return slices.ContainsFunc(pkg.Versions, func(e spec.VersionEntry) bool { return e.Version == v })
}
