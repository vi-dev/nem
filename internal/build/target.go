package build

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"oras.land/oras-go/v2"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/spec"
)

type Target struct {
	Ref     string
	Dir     string
	File    bool
	Entry   catalog.Entry
	Overlay *ocix.ArchiveStore
}

func (t *Target) IsDir() bool { return t.Dir != "" }

func (t *Target) Packages(ctx context.Context) ([]*spec.Package, error) {
	names, err := t.Entry.Catalog.PackageNames(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*spec.Package, 0, len(names))
	for _, name := range names {
		pkg, _, err := t.Entry.Catalog.Package(ctx, name)
		if err != nil {
			return nil, err
		}
		out = append(out, pkg)
	}
	return out, nil
}

func (t *Target) Archives(name string) (oras.ReadOnlyTarget, error) {
	if !t.IsDir() {
		return readArchives(t.Ref, name)
	}
	if t.Overlay == nil || !t.Overlay.Has(name) {
		return nil, fmt.Errorf("no staged archives for %s in %s: %w", name, t.Dir, ocix.ErrArchiveNotFound)
	}
	return t.Overlay.Open(name)
}

func (t *Target) ArchivePlatforms(ctx context.Context, name, version string) ([]spec.Platform, error) {
	src, err := t.Archives(name)
	if errors.Is(err, ocix.ErrArchiveNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	have, err := ocix.ArchivePlatforms(ctx, src, version)
	if err != nil && !errors.Is(err, ocix.ErrArchiveNotFound) {
		return nil, fmt.Errorf("%s@%s: %w", name, version, err)
	}
	return have, nil
}

func (t *Target) ArchiveExists(ctx context.Context, name, version string, plat spec.Platform) (bool, error) {
	have, err := t.ArchivePlatforms(ctx, name, version)
	if err != nil {
		return false, err
	}
	return slices.Contains(have, plat), nil
}

var readArchives = ocix.RemoteArchives

func SetArchivesReader(f func(catalogRef, name string) (oras.ReadOnlyTarget, error)) (restore func()) {
	prev := readArchives
	readArchives = f
	return func() { readArchives = prev }
}

func OpenTarget(ctx context.Context, ref string) (*Target, error) {
	src, err := catalog.Open(ctx, ref)
	if err != nil {
		return nil, err
	}
	switch src.(type) {
	case *catalog.Dir:
		return &Target{
			Ref:     ref,
			Dir:     ref,
			Entry:   catalog.Entry{Name: ref, Catalog: src},
			Overlay: ocix.NewArchiveStore(ref),
		}, nil
	case *catalog.File:
		return &Target{Ref: ref, File: true, Entry: catalog.Entry{Name: ref, Catalog: src}}, nil
	default:
		return &Target{Ref: ref, Entry: catalog.Entry{Name: ref, Ref: ref, Catalog: src}}, nil
	}
}
