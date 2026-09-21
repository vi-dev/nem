package build

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"oras.land/oras-go/v2"

	"github.com/vi-dev/nem/internal/archive"
	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/spec"
)

type Target struct {
	Ref     string
	Dir     string
	File    bool
	Entry   catalog.Entry
	Overlay *archive.Dir
	Store   archive.ReadWriteStore
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

func (t *Target) Archives(ctx context.Context, name string) (oras.ReadOnlyTarget, error) {
	return t.Entry.Archives.Open(ctx, name)
}

func (t *Target) ArchivePlatforms(ctx context.Context, name, version string) ([]spec.Platform, error) {
	src, err := t.Archives(ctx, name)
	if errors.Is(err, archive.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	have, err := archive.ResolvePlatforms(ctx, src, version)
	if err != nil && !errors.Is(err, archive.ErrNotFound) {
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

func OpenTarget(ctx context.Context, ref string) (*Target, error) {
	entry, err := catalog.Open(ctx, ref)
	if err != nil {
		return nil, err
	}
	switch entry.Catalog.(type) {
	case *catalog.Dir:
		dir := archive.NewDir(ref)
		return &Target{Ref: ref, Dir: ref, Entry: entry, Overlay: dir, Store: dir}, nil
	case *catalog.File:
		return &Target{Ref: ref, File: true, Entry: entry}, nil
	default:
		return &Target{Ref: ref, Entry: entry, Store: archive.Remote(ref)}, nil
	}
}
