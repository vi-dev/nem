package diff

import (
	"context"
	"fmt"
	"os"
	"sort"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"

	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/publish"
	"github.com/vi-dev/nem/internal/report"
	"github.com/vi-dev/nem/internal/spec"
)

var openCatalog = ocix.RemoteCatalog

func SetCatalogOpener(f func(ref string) (oras.ReadOnlyTarget, string, error)) (restore func()) {
	prev := openCatalog
	openCatalog = f
	return func() { openCatalog = prev }
}

const (
	statusNew       = "new"
	statusUpdated   = "updated"
	statusRemoved   = "removed"
	statusUnchanged = "unchanged"
)

type Options struct {
	JSON bool
}

type Row struct {
	Name            string   `json:"name"`
	Path            string   `json:"path,omitempty"`
	Status          string   `json:"status"`
	Source          bool     `json:"source"`
	Published       string   `json:"published,omitempty"`
	Local           string   `json:"local,omitempty"`
	VersionsAdded   []string `json:"versionsAdded"`
	VersionsRemoved []string `json:"versionsRemoved"`
}

func Run(ctx context.Context, ref, target string, opts Options, console *report.Console) error {
	if err := ocix.WithTagOrDigest(ref); err != nil {
		return err
	}
	info, err := os.Stat(target)
	if err != nil {
		return err
	}
	dirTarget := info.IsDir()

	paths, err := publish.ManifestPaths(target)
	if err != nil {
		return err
	}

	src, srcRef, err := openCatalog(ref)
	if err != nil {
		return err
	}
	idx, _, err := ocix.FetchCatalogIndex(ctx, src, srcRef)
	if err != nil {
		return err
	}
	published := make(map[string]ocispec.Descriptor, len(idx.Manifests))
	for _, m := range idx.Manifests {
		published[m.Annotations[ocix.AnnotationTitle]] = m
	}

	rows := make([]Row, 0, len(paths))
	for _, p := range paths {
		row, err := diffOne(ctx, src, published, p)
		if err != nil {
			return err
		}
		rows = append(rows, row)
	}
	if dirTarget {
		seen := make(map[string]bool, len(rows))
		for _, r := range rows {
			seen[r.Name] = true
		}
		for name, m := range published {
			if seen[name] {
				continue
			}
			row, err := removedRow(ctx, src, name, m)
			if err != nil {
				return err
			}
			rows = append(rows, row)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })

	if opts.JSON {
		if err := console.JSON(rows); err != nil {
			return err
		}
	} else {
		var tableRows [][]string
		for _, r := range rows {
			if r.Status == statusUnchanged {
				continue
			}
			pub, loc := r.Published, r.Local
			if pub == "" {
				pub = "-"
			}
			if loc == "" {
				loc = "-"
			}
			tableRows = append(tableRows, []string{r.Name, r.Status, pub, loc})
		}
		if len(tableRows) > 0 {
			console.Table([]string{"PACKAGE", "STATUS", "PUBLISHED", "LOCAL"}, tableRows)
		}
	}
	counts := map[string]int{}
	for _, r := range rows {
		counts[r.Status]++
	}
	changed := counts[statusNew] + counts[statusUpdated] + counts[statusRemoved]
	console.Success("Compared %d packages against %s: %d changed (%d new, %d updated, %d removed)",
		len(rows), ref, changed, counts[statusNew], counts[statusUpdated], counts[statusRemoved])
	return nil
}

func diffOne(ctx context.Context, src oras.ReadOnlyTarget, published map[string]ocispec.Descriptor, path string) (Row, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Row{}, err
	}
	pkg, err := spec.Parse(data)
	if err != nil {
		return Row{}, fmt.Errorf("%s: %w", path, err)
	}
	row := Row{
		Name:            pkg.Name,
		Path:            path,
		Source:          pkg.Build != nil,
		VersionsAdded:   []string{},
		VersionsRemoved: []string{},
	}
	if len(pkg.Versions) > 0 {
		row.Local = pkg.Versions[0].Version
	}

	pub, ok := published[pkg.Name]
	if !ok {
		row.Status = statusNew
		for _, v := range pkg.Versions {
			row.VersionsAdded = append(row.VersionsAdded, v.Version)
		}
		return row, nil
	}
	_, desc, err := ocix.PackageManifest(data)
	if err != nil {
		return Row{}, fmt.Errorf("%s: %w", path, err)
	}
	if pub.Digest == desc.Digest {
		row.Status = statusUnchanged
		row.Published = row.Local
		return row, nil
	}

	row.Status = statusUpdated
	row.Published = pub.Annotations[ocix.AnnotationVersion]
	pubBytes, err := ocix.FetchPkgBytes(ctx, src, pub)
	if err != nil {
		return Row{}, fmt.Errorf("published %s: %w", pkg.Name, err)
	}
	pubPkg, err := spec.Parse(pubBytes)
	if err != nil {
		return Row{}, fmt.Errorf("published %s: %w", pkg.Name, err)
	}
	if row.Published == "" && len(pubPkg.Versions) > 0 {
		row.Published = pubPkg.Versions[0].Version
	}
	row.VersionsAdded, row.VersionsRemoved = versionDelta(pkg, pubPkg)
	return row, nil
}

func versionDelta(local, published *spec.Package) (added, removed []string) {
	added, removed = []string{}, []string{}
	for _, v := range local.Versions {
		if !published.HasEqualVersion(v.Version) {
			added = append(added, v.Version)
		}
	}
	for _, v := range published.Versions {
		if !local.HasEqualVersion(v.Version) {
			removed = append(removed, v.Version)
		}
	}
	return added, removed
}

func removedRow(ctx context.Context, src oras.ReadOnlyTarget, name string, man ocispec.Descriptor) (Row, error) {
	row := Row{
		Name:            name,
		Status:          statusRemoved,
		Published:       man.Annotations[ocix.AnnotationVersion],
		VersionsAdded:   []string{},
		VersionsRemoved: []string{},
	}
	pubBytes, err := ocix.FetchPkgBytes(ctx, src, man)
	if err != nil {
		return Row{}, fmt.Errorf("published %s: %w", name, err)
	}
	pubPkg, err := spec.Parse(pubBytes)
	if err != nil {
		return Row{}, fmt.Errorf("published %s: %w", name, err)
	}
	row.Source = pubPkg.Build != nil
	if row.Published == "" && len(pubPkg.Versions) > 0 {
		row.Published = pubPkg.Versions[0].Version
	}
	for _, v := range pubPkg.Versions {
		row.VersionsRemoved = append(row.VersionsRemoved, v.Version)
	}
	return row, nil
}
