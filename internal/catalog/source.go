package catalog

import (
	"context"

	"github.com/vi-dev/nem/internal/spec"
)

type Summary struct{ Name, Description, Latest string }

type Source interface {
	Summaries(ctx context.Context) ([]Summary, error)
	Versions(ctx context.Context, name string) ([]string, error)
	Load(ctx context.Context, name string) (*spec.Package, string, error)
}

type NameLister interface {
	PackageNames(ctx context.Context) ([]string, error)
}

type Named struct {
	Name   string
	Source Source
}

func versionsOf(pkg *spec.Package) []string {
	out := make([]string, len(pkg.Versions))
	for i, v := range pkg.Versions {
		out[i] = v.Version
	}
	return out
}
