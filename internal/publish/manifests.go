package publish

import (
	"fmt"
	"os"
	"path/filepath"
)

type Manifest struct {
	Pkg, Path string
}

func Manifests(dir string) ([]Manifest, error) {
	pkgsDir := filepath.Join(dir, "pkgs")
	entries, err := os.ReadDir(pkgsDir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", pkgsDir, err)
	}
	var out []Manifest
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		out = append(out, Manifest{Pkg: e.Name(), Path: filepath.Join(pkgsDir, e.Name(), "pkg.yaml")})
	}
	return out, nil
}

func ManifestPaths(target string) ([]string, error) {
	info, err := os.Stat(target)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{target}, nil
	}
	manifests, err := Manifests(target)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, m := range manifests {
		if _, err := os.Stat(m.Path); err == nil {
			out = append(out, m.Path)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no package manifests under %s", target)
	}
	return out, nil
}
