package catalog

import (
	"context"

	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/spec"
)

type Summary struct{ Name, Description, Latest string }

type Catalog interface {
	Summaries(ctx context.Context) ([]Summary, error)
	Versions(ctx context.Context, name string) ([]string, error)
	Package(ctx context.Context, name string) (*spec.Package, string, error)
	PackageNames(ctx context.Context) ([]string, error)
	ReadManifest(ctx context.Context, name string) ([]byte, error)
	ReadFiles(ctx context.Context, name string) (spec.Files, error)
	Digest(ctx context.Context, name string) (string, error)
}

func digestOfFiles(files spec.Files) (string, error) {
	_, desc, err := ocix.PackageManifest(files)
	if err != nil {
		return "", err
	}
	return desc.Digest.String(), nil
}

func versionsOf(pkg *spec.Package) []string {
	out := make([]string, len(pkg.Versions))
	for i, v := range pkg.Versions {
		out[i] = v.Version
	}
	return out
}
