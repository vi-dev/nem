package catalog

import (
	"context"
	"fmt"

	"github.com/vi-dev/nem/internal/spec"
)

type ExclusionChange int

const (
	ExclusionAdded ExclusionChange = iota
	ExclusionUpdated
	ExclusionUnchanged
	ExclusionLifted
)

// SetExclusion excludes version of package name in a local catalog, or lifts
// the exclusion when ex is nil. Excluding an excluded version again replaces
// its reason and use, and is a no-op when they match. Excluding raises the
// package to schema 4 and drops the use of other exclusions that pointed at
// version, returning those versions; lifting leaves the schema as it is.
func SetExclusion(ctx context.Context, cat, name, version string, ex *spec.Exclusion) (ExclusionChange, []string, error) {
	entry, err := Open(ctx, cat)
	if err != nil {
		return 0, nil, err
	}
	editor, ok := entry.Catalog.(Editor)
	if !ok {
		return 0, nil, fmt.Errorf("%s: exclusions are written to a local catalog directory or pkg.yaml", cat)
	}
	files, err := editor.ReadFiles(ctx, name)
	if err != nil {
		return 0, nil, err
	}
	pkg, err := spec.ParseFiles(files)
	if err != nil {
		return 0, nil, err
	}
	if !pkg.HasVersion(version) {
		return 0, nil, &VersionNotFoundError{Name: name, Version: version, Catalog: entry.Name}
	}
	current := pkg.Exclusion(version)
	change := ExclusionAdded
	switch {
	case ex == nil && current == nil:
		return 0, nil, fmt.Errorf("version %s of package %s is not excluded", version, name)
	case ex == nil:
		change = ExclusionLifted
	case current != nil && *current == *ex:
		return ExclusionUnchanged, nil, nil
	case current != nil:
		change = ExclusionUpdated
	}

	edited, err := spec.SetExclusion(files.Pkg, version, ex)
	if err != nil {
		return 0, nil, err
	}
	var cleared []string
	for _, e := range pkg.Versions {
		if ex == nil || e.Excluded == nil || e.Excluded.Use != version {
			continue
		}
		if edited, err = spec.SetExclusion(edited, e.Version, &spec.Exclusion{Reason: e.Excluded.Reason}); err != nil {
			return 0, nil, err
		}
		cleared = append(cleared, e.Version)
	}
	if ex != nil && pkg.Schema < spec.SchemaExcluded {
		if edited, err = spec.SetSchema(edited, spec.SchemaExcluded); err != nil {
			return 0, nil, err
		}
	}
	check, err := spec.ParseFiles(spec.Files{Pkg: edited, Recipes: files.Recipes})
	if err != nil {
		return 0, nil, fmt.Errorf("edited manifest is invalid: %w", err)
	}
	if err := check.Validate(); err != nil {
		return 0, nil, fmt.Errorf("edited manifest is invalid: %w", err)
	}
	return change, cleared, editor.UpdateManifest(ctx, name, edited)
}
