package catalog

import (
	"context"

	"github.com/vi-dev/nem/internal/spec"
)

// File serves a single already-parsed manifest.
type File struct{ pkg *spec.Package }

func NewFile(pkg *spec.Package) *File { return &File{pkg: pkg} }

func (f *File) Summaries(_ context.Context) ([]Summary, error) {
	latest := ""
	if len(f.pkg.Versions) > 0 {
		latest = f.pkg.Versions[0].Version
	}
	return []Summary{{Name: f.pkg.Name, Description: f.pkg.Description, Latest: latest}}, nil
}

func (f *File) Versions(_ context.Context, name string) ([]string, error) {
	if name != f.pkg.Name {
		return nil, &PackageNotFoundError{Name: name}
	}
	return versionsOf(f.pkg), nil
}

func (f *File) Load(_ context.Context, name string) (*spec.Package, string, error) {
	if name != f.pkg.Name {
		return nil, "", &PackageNotFoundError{Name: name}
	}
	return f.pkg, "", nil
}
