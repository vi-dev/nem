package home

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

type Home struct{ root string }

func Resolve(getenv func(string) string) Home {
	if root := getenv("NEM_HOME"); root != "" {
		return Home{root: root}
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		dir = "."
	}
	return Home{root: filepath.Join(dir, ".nem")}
}

func (h Home) Root() string           { return h.root }
func (h Home) Config() string         { return filepath.Join(h.root, "config.yaml") }
func (h Home) GlobalManifest() string { return filepath.Join(h.root, "nem.toml") }
func (h Home) GlobalLock() string     { return filepath.Join(h.root, "nem.lock") }
func (h Home) LockFile() string       { return filepath.Join(h.root, "lock") }
func (h Home) Tmp() string            { return filepath.Join(h.root, "tmp") }
func (h Home) Usage() string          { return filepath.Join(h.root, "usage.json") }

func (h Home) Packages() string { return filepath.Join(h.root, "packages") }

const TmpSuffix = ".tmp"

const BuildStagingInfix = "-build-"

const TestInstallInfix = "-NEMTEST-"

var segmentRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)

func safeSegment(s string) error {
	if !segmentRE.MatchString(s) {
		return fmt.Errorf("invalid path segment %q", s)
	}
	return nil
}

func (h Home) PackageDir(name, version string) (string, error) {
	if err := safeSegment(name); err != nil {
		return "", err
	}
	if err := safeSegment(version); err != nil {
		return "", err
	}
	return filepath.Join(h.Packages(), name, version), nil
}

func (h Home) CatalogStore(name string) (string, error) {
	dir, err := h.CatalogDir(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "store"), nil
}

func (h Home) CatalogDir(name string) (string, error) {
	if err := safeSegment(name); err != nil {
		return "", err
	}
	return filepath.Join(h.root, "catalogs", name), nil
}
