package catalog

import (
	"context"
	"fmt"
	"os"

	"github.com/vi-dev/nem/internal/fsx"
	"github.com/vi-dev/nem/internal/spec"
)

type File struct{ path string }

var (
	_ Catalog = (*File)(nil)
	_ Editor  = (*File)(nil)
)

func NewFile(path string) *File { return &File{path: path} }

func (f *File) read() (*spec.Package, error) {
	files, err := readPackageFiles(f.path)
	if err != nil {
		return nil, err
	}
	pkg, err := spec.ParseFiles(files)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", f.path, err)
	}
	if err := pkg.Validate(); err != nil {
		return nil, fmt.Errorf("load %s: %w", f.path, err)
	}
	return pkg, nil
}

func (f *File) ReadFiles(_ context.Context, name string) (spec.Files, error) {
	files, err := readPackageFiles(f.path)
	if os.IsNotExist(err) {
		return spec.Files{}, &PackageNotFoundError{Name: name}
	}
	if err != nil {
		return spec.Files{}, err
	}
	return files, nil
}

func (f *File) WriteFiles(_ context.Context, name string, files spec.Files) error {
	if _, err := os.Stat(f.path); os.IsNotExist(err) {
		return &PackageNotFoundError{Name: name}
	} else if err != nil {
		return err
	}
	return writePackageFiles(f.path, files)
}

func (f *File) Package(_ context.Context, name string) (*spec.Package, string, error) {
	pkg, err := f.read()
	if os.IsNotExist(err) {
		return nil, "", &PackageNotFoundError{Name: name}
	}
	if err != nil {
		return nil, "", err
	}
	if pkg.Name != name {
		return nil, "", &PackageNotFoundError{Name: name}
	}
	return pkg, "", nil
}

func (f *File) Versions(ctx context.Context, name string) ([]string, error) {
	pkg, _, err := f.Package(ctx, name)
	if err != nil {
		return nil, err
	}
	return versionsOf(pkg), nil
}

func (f *File) PackageNames(_ context.Context) ([]string, error) {
	data, err := os.ReadFile(f.path)
	if err != nil {
		return nil, err
	}
	pkg, err := spec.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", f.path, err)
	}
	return []string{pkg.Name}, nil
}

func (f *File) Summaries(_ context.Context) ([]Summary, error) {
	pkg, err := f.read()
	if err != nil {
		return nil, err
	}
	latest := ""
	if len(pkg.Versions) > 0 {
		latest = pkg.Versions[0].Version
	}
	return []Summary{{Name: pkg.Name, Description: pkg.Description, Latest: latest}}, nil
}

func (f *File) ReadManifest(_ context.Context, name string) ([]byte, error) {
	data, err := os.ReadFile(f.path)
	if os.IsNotExist(err) {
		return nil, &PackageNotFoundError{Name: name}
	}
	if err != nil {
		return nil, err
	}
	pkg, err := spec.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", f.path, err)
	}
	if pkg.Name != name {
		return nil, &PackageNotFoundError{Name: name}
	}
	return data, nil
}

func (f *File) CreateManifest(_ context.Context, name string, data []byte) error {
	if err := validateManifest(name, data); err != nil {
		return err
	}
	if _, err := os.Stat(f.path); err == nil {
		return &ManifestExistsError{Name: name}
	} else if !os.IsNotExist(err) {
		return err
	}
	return fsx.WriteAtomic(f.path, data, 0o644)
}

func (f *File) UpdateManifest(_ context.Context, name string, data []byte) error {
	info, err := os.Stat(f.path)
	if os.IsNotExist(err) {
		return &PackageNotFoundError{Name: name}
	}
	if err != nil {
		return err
	}
	return fsx.WriteAtomic(f.path, data, info.Mode().Perm())
}

func (f *File) Digest(ctx context.Context, name string) (string, error) {
	if _, _, err := f.Package(ctx, name); err != nil {
		return "", err
	}
	files, err := f.ReadFiles(ctx, name)
	if err != nil {
		return "", err
	}
	return digestOfFiles(files)
}
