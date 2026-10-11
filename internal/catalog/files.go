package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vi-dev/nem/internal/fsx"
	"github.com/vi-dev/nem/internal/spec"
)

func readPackageFiles(pkgPath string) (spec.Files, error) {
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return spec.Files{}, err
	}
	files := spec.Files{Pkg: data}
	dir := filepath.Dir(pkgPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return spec.Files{}, err
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "recipe@") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		info, err := os.Stat(path)
		if err != nil {
			return spec.Files{}, fmt.Errorf("read %s: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return spec.Files{}, fmt.Errorf("read %s: %w", path, err)
		}
		if files.Recipes == nil {
			files.Recipes = map[string][]byte{}
		}
		files.Recipes[e.Name()] = b
	}
	return files, nil
}

func writePackageFiles(pkgPath string, files spec.Files) error {
	for name := range files.Recipes {
		if _, ok := spec.ParseRecipeFileName(name); !ok {
			return fmt.Errorf("recipe file %q: name must be recipe@<prefix>.yaml", name)
		}
	}
	if files.Pkg != nil {
		info, err := os.Stat(pkgPath)
		if err != nil {
			return err
		}
		if err := fsx.WriteAtomic(pkgPath, files.Pkg, info.Mode().Perm()); err != nil {
			return err
		}
	}
	dir := filepath.Dir(pkgPath)
	for _, name := range files.Names() {
		if err := fsx.WriteAtomic(filepath.Join(dir, name), files.Recipes[name], 0o644); err != nil {
			return err
		}
	}
	return nil
}

func loadPackageFiles(files spec.Files, name, where string) (*spec.Package, error) {
	pkg, err := spec.ParseFiles(files)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", where, err)
	}
	if err := pkg.Validate(); err != nil {
		return nil, fmt.Errorf("load %s: %w", where, err)
	}
	if pkg.Name != name {
		return nil, fmt.Errorf("load %s: manifest declares name %q, want %q", where, pkg.Name, name)
	}
	return pkg, nil
}
