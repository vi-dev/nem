package install

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/vi-dev/nem/internal/home"
	"github.com/vi-dev/nem/internal/spec"
)

const DepsDir = ".nem-link-dependencies"

func Links(pkg *spec.Package, versions map[string]string) map[string]string {
	plat := spec.Current()
	links := map[string]string{}
	for _, d := range pkg.Deps {
		if d.Kind != spec.DepKindLink || !spec.PlatformsInclude(d.Platforms, plat) {
			continue
		}
		if v, ok := versions[d.Name]; ok {
			links[d.Name] = v
		}
	}
	return links
}

func linkTarget(dep, version string) string {
	return filepath.Join("..", "..", "..", dep, version)
}

func WriteLinks(dir string, links map[string]string) (map[string]string, error) {
	depsPath := filepath.Join(dir, DepsDir)
	entries, err := os.ReadDir(depsPath)
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read %s: %w", depsPath, err)
	}
	for _, e := range entries {
		if e.Type()&fs.ModeSymlink == 0 {
			return nil, fmt.Errorf("%s: %s is not a symlink", depsPath, e.Name())
		}
	}
	for _, e := range entries {
		if _, keep := links[e.Name()]; keep {
			continue
		}
		if err := os.Remove(filepath.Join(depsPath, e.Name())); err != nil {
			return nil, fmt.Errorf("remove stale link %s: %w", e.Name(), err)
		}
	}
	if len(links) == 0 {
		if exists {
			if err := os.Remove(depsPath); err != nil {
				return nil, fmt.Errorf("remove %s: %w", depsPath, err)
			}
		}
		return nil, nil
	}
	if err := os.MkdirAll(depsPath, 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", depsPath, err)
	}
	retargeted := map[string]string{}
	for dep, version := range links {
		link := filepath.Join(depsPath, dep)
		target := linkTarget(dep, version)
		cur, err := os.Readlink(link)
		if err == nil && cur == target {
			continue
		}
		if err == nil {
			retargeted[dep] = filepath.Base(cur)
		}
		tmp := link + home.TmpSuffix
		if err := os.Symlink(target, tmp); err != nil {
			return nil, fmt.Errorf("link %s: %w", dep, err)
		}
		if err := os.Rename(tmp, link); err != nil {
			os.Remove(tmp)
			return nil, fmt.Errorf("link %s: %w", dep, err)
		}
	}
	return retargeted, nil
}

func ReadLinks(dir string) (map[string]string, error) {
	depsPath := filepath.Join(dir, DepsDir)
	entries, err := os.ReadDir(depsPath)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", depsPath, err)
	}
	links := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.Type()&fs.ModeSymlink == 0 || strings.HasSuffix(e.Name(), home.TmpSuffix) {
			continue
		}
		target, err := os.Readlink(filepath.Join(depsPath, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read link %s: %w", e.Name(), err)
		}
		links[e.Name()] = filepath.Base(target)
	}
	return links, nil
}
