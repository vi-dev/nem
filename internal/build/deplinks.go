package build

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/vi-dev/nem/internal/install"
)

type depLink struct {
	dir    string
	target string
}

func planDepLinks(outDir string) ([]depLink, error) {
	dirs := map[string]bool{}
	err := walkBinaryRefs(outDir, func(path string, refs []string) error {
		if dir := filepath.Dir(path); dir != outDir && slices.ContainsFunc(refs, isDepsRpath) {
			dirs[dir] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	var links []depLink
	for _, dir := range slices.Sorted(maps.Keys(dirs)) {
		rel, err := filepath.Rel(dir, outDir)
		if err != nil {
			return nil, err
		}
		links = append(links, depLink{dir: dir, target: filepath.Join(rel, install.DepsDir)})
	}
	return links, nil
}

func applyDepLinks(links []depLink) error {
	for _, l := range links {
		path := filepath.Join(l.dir, install.DepsDir)
		info, err := os.Lstat(path)
		switch {
		case err == nil && info.Mode()&fs.ModeSymlink != 0:
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("replace %s: %w", path, err)
			}
		case err == nil:
			return fmt.Errorf("%s: %s is not a symlink", l.dir, install.DepsDir)
		case !errors.Is(err, fs.ErrNotExist):
			return fmt.Errorf("stat %s: %w", path, err)
		}
		if err := os.Symlink(l.target, path); err != nil {
			return fmt.Errorf("link %s: %w", path, err)
		}
	}
	return nil
}
