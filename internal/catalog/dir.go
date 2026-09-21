package catalog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/vi-dev/nem/internal/fsx"
	"github.com/vi-dev/nem/internal/spec"
)

type Dir struct{ root string }

var _ Catalog = (*Dir)(nil)
var _ Editor = (*Dir)(nil)

func NewDir(root string) *Dir { return &Dir{root: root} }

const manifestDir = "pkgs"

func (d *Dir) manifestPath(name string) string {
	return filepath.Join(d.root, manifestDir, name, "pkg.yaml")
}

func (d *Dir) Package(_ context.Context, name string) (*spec.Package, string, error) {
	data, err := os.ReadFile(d.manifestPath(name))
	if os.IsNotExist(err) {
		return nil, "", &PackageNotFoundError{Name: name}
	}
	if err != nil {
		return nil, "", fmt.Errorf("read %s: %w", d.manifestPath(name), err)
	}
	pkg, err := spec.Parse(data)
	if err != nil {
		return nil, "", fmt.Errorf("load %s: %w", d.manifestPath(name), err)
	}
	if err := pkg.Validate(); err != nil {
		return nil, "", fmt.Errorf("load %s: %w", d.manifestPath(name), err)
	}
	if pkg.Name != name {
		return nil, "", fmt.Errorf("load %s: manifest declares name %q, want %q", d.manifestPath(name), pkg.Name, name)
	}
	return pkg, "", nil
}

func (d *Dir) Versions(ctx context.Context, name string) ([]string, error) {
	pkg, _, err := d.Package(ctx, name)
	if err != nil {
		return nil, err
	}
	return versionsOf(pkg), nil
}

func (d *Dir) isPackage(e os.DirEntry) bool {
	if !e.IsDir() || !spec.NameRE.MatchString(e.Name()) {
		return false
	}
	_, err := os.Stat(d.manifestPath(e.Name()))
	return err == nil
}

func (d *Dir) PackageNames(_ context.Context) ([]string, error) {
	dirEntries, err := os.ReadDir(filepath.Join(d.root, manifestDir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read catalog dir %s: %w", d.root, err)
	}
	var names []string
	for _, e := range dirEntries {
		if !d.isPackage(e) {
			continue
		}
		names = append(names, e.Name())
	}
	return names, nil
}

func (d *Dir) Summaries(ctx context.Context) ([]Summary, error) {
	dirEntries, err := os.ReadDir(filepath.Join(d.root, manifestDir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read catalog dir %s: %w", d.root, err)
	}
	var summaries []Summary
	for _, e := range dirEntries {
		if !d.isPackage(e) {
			continue
		}
		pkg, _, err := d.Package(ctx, e.Name())
		if err != nil {
			continue
		}
		latest := ""
		if len(pkg.Versions) > 0 {
			latest = pkg.Versions[0].Version
		}
		summaries = append(summaries, Summary{Name: pkg.Name, Description: pkg.Description, Latest: latest})
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Name < summaries[j].Name })
	return summaries, nil
}

func (d *Dir) ReadManifest(_ context.Context, name string) ([]byte, error) {
	if !spec.NameRE.MatchString(name) {
		return nil, fmt.Errorf("invalid package name %q", name)
	}
	data, err := os.ReadFile(d.manifestPath(name))
	if os.IsNotExist(err) {
		return nil, &PackageNotFoundError{Name: name}
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", d.manifestPath(name), err)
	}
	return data, nil
}

func (d *Dir) CreateManifest(_ context.Context, name string, data []byte) error {
	if !spec.NameRE.MatchString(name) {
		return fmt.Errorf("invalid package name %q", name)
	}
	if err := validateManifest(name, data); err != nil {
		return err
	}
	path := d.manifestPath(name)
	if _, err := os.Stat(path); err == nil {
		return &ManifestExistsError{Name: name}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return fsx.WriteAtomic(path, data, 0o644)
}

func (d *Dir) UpdateManifest(_ context.Context, name string, data []byte) error {
	if !spec.NameRE.MatchString(name) {
		return fmt.Errorf("invalid package name %q", name)
	}
	path := d.manifestPath(name)
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return &PackageNotFoundError{Name: name}
	}
	if err != nil {
		return err
	}
	return fsx.WriteAtomic(path, data, info.Mode().Perm())
}
