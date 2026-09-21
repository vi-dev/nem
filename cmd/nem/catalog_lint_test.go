package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const lintFixtureGoPkg = `
schema: 2
name: go
description: The Go programming language
homepage: https://go.dev
license: BSD-3-Clause
artifact:
  url: "https://go.dev/dl/go{{.Version}}.{{.OS}}-{{.Arch}}.tar.gz"
install:
  - extract: {strip: 1}
versions:
  - version: v1.26.5
    sha256:
      darwin/arm64: "aaa"
      darwin/amd64: "bbb"
      linux/arm64: "ccc"
      linux/amd64: "ddd"
`

const lintFixtureGoPkgReservedEnv = lintFixtureGoPkg + `env:
  - name: PATH
    value: "x"
`

func writeLintFixture(t *testing.T, pkgs map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, y := range pkgs {
		pkgDir := filepath.Join(dir, "pkgs", name)
		if err := os.MkdirAll(pkgDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pkgDir, "pkg.yaml"), []byte(y), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestCatalogLintCleanCatalog(t *testing.T) {
	nemHome := t.TempDir()
	catDir := writeLintFixture(t, map[string]string{"go": lintFixtureGoPkg})

	out, errb, err := runNem(t, nemHome, "catalog", "lint", catDir)
	if err != nil {
		t.Fatalf("lint: %v", err)
	}
	if !strings.Contains(errb, "OK Catalog is clean: no findings") {
		t.Fatalf("stderr = %q, want the clean-summary success line", errb)
	}
	if out != "" {
		t.Fatalf("stdout = %q, want empty", out)
	}
}

func TestCatalogLintCleanSummarySuppressedByQuiet(t *testing.T) {
	nemHome := t.TempDir()
	catDir := writeLintFixture(t, map[string]string{"go": lintFixtureGoPkg})

	out, errb, err := runNem(t, nemHome, "catalog", "lint", catDir, "--quiet")
	if err != nil {
		t.Fatalf("lint: %v", err)
	}
	if strings.Contains(errb, "clean") {
		t.Fatalf("stderr = %q, want no clean summary under --quiet", errb)
	}
	if out != "" {
		t.Fatalf("stdout = %q, want empty", out)
	}
}

func TestCatalogLintDefectiveCatalog(t *testing.T) {
	nemHome := t.TempDir()
	catDir := writeLintFixture(t, map[string]string{"go": lintFixtureGoPkgReservedEnv})

	out, errb, err := runNem(t, nemHome, "catalog", "lint", catDir)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("lint error = %v, want *ExitError", err)
	}
	if exitErr.Code != 1 {
		t.Fatalf("exit code = %d, want 1", exitErr.Code)
	}
	if !strings.Contains(errb, "WARN ") || !strings.Contains(errb, "reserved") {
		t.Fatalf("stderr = %q, want the reserved-env finding as a WARN line", errb)
	}
	if out != "" {
		t.Fatalf("stdout = %q, want empty", out)
	}
}

func TestCatalogLintDefaultsToCurrentDir(t *testing.T) {
	nemHome := t.TempDir()
	catDir := writeLintFixture(t, map[string]string{"go": lintFixtureGoPkg})
	chdir(t, catDir)

	_, errb, err := runNem(t, nemHome, "catalog", "lint")
	if err != nil {
		t.Fatalf("lint: %v", err)
	}
	if !strings.Contains(errb, "OK Catalog is clean: no findings") {
		t.Fatalf("stderr = %q, want the clean-summary success line", errb)
	}
}

func TestCatalogLintScopedToPackage(t *testing.T) {
	nemHome := t.TempDir()
	// Build one catalog containing a clean package and a defective package,
	// cribbing content from lintFixtureGoPkg/lintFixtureGoPkgReservedEnv above
	// (their embedded "name: go" must match the directory name "go", so it's
	// adapted to "good"/"bad" here rather than reused verbatim).
	goodPkg := `
schema: 2
name: good
description: The Go programming language
homepage: https://go.dev
license: BSD-3-Clause
artifact:
  url: "https://go.dev/dl/go{{.Version}}.{{.OS}}-{{.Arch}}.tar.gz"
install:
  - extract: {strip: 1}
versions:
  - version: v1.26.5
    sha256:
      darwin/arm64: "aaa"
      darwin/amd64: "bbb"
      linux/arm64: "ccc"
      linux/amd64: "ddd"
`
	badPkg := `
schema: 2
name: bad
description: The Go programming language
homepage: https://go.dev
license: BSD-3-Clause
artifact:
  url: "https://go.dev/dl/go{{.Version}}.{{.OS}}-{{.Arch}}.tar.gz"
install:
  - extract: {strip: 1}
versions:
  - version: v1.26.5
    sha256:
      darwin/arm64: "aaa"
      darwin/amd64: "bbb"
      linux/arm64: "ccc"
      linux/amd64: "ddd"
env:
  - name: PATH
    value: "x"
`
	catDir := writeLintFixture(t, map[string]string{
		"good": goodPkg,
		"bad":  badPkg,
	})

	// Unfiltered: defective package fails the run.
	if _, _, err := runNem(t, nemHome, "catalog", "lint", catDir); err == nil {
		t.Fatal("unfiltered lint must fail on the defective package")
	}

	// Scoped to the clean package: passes.
	out, errb, err := runNem(t, nemHome, "catalog", "lint", catDir, "--package", "good")
	if err != nil {
		t.Fatalf("scoped lint: %v\n%s\n%s", err, out, errb)
	}

	// Unknown package: hard error naming the catalog.
	_, errb, err = runNem(t, nemHome, "catalog", "lint", catDir, "--package", "nosuch")
	if err == nil || !strings.Contains(errb, "nosuch not found in catalog(s) "+catDir) {
		t.Fatalf("err = %v, stderr = %q; want PackageNotFoundError naming the catalog", err, errb)
	}
}

func TestCatalogLintAcceptsManifestPathAsFileCatalog(t *testing.T) {
	nemHome := t.TempDir()
	catDir := writeLintFixture(t, map[string]string{"go": lintFixtureGoPkg})
	recipe := filepath.Join(catDir, "pkgs", "go", "pkg.yaml")

	out, errb, err := runNem(t, nemHome, "catalog", "lint", recipe)
	if err != nil {
		t.Fatalf("lint on a pkg.yaml must lint it as a single-file catalog: %v\n%s\n%s", err, out, errb)
	}

	if _, _, err := runNem(t, nemHome, "catalog", "lint", recipe, "--package", "go"); err != nil {
		t.Fatalf("--package with the declared name must work: %v", err)
	}
	_, _, err = runNem(t, nemHome, "catalog", "lint", recipe, "--package", "nosuch")
	if err == nil || !strings.Contains(err.Error(), "package nosuch not found") {
		t.Fatalf("want a clean not-found error, got: %v", err)
	}
	if strings.Contains(err.Error(), "catalog(s)") {
		t.Fatalf("dangling catalog list in error: %v", err)
	}
}

func TestCatalogLintFileCatalogReportsBrokenManifest(t *testing.T) {
	nemHome := t.TempDir()
	recipe := filepath.Join(t.TempDir(), "pkg.yaml")
	writeFile(t, recipe, "not: [valid")

	_, errb, err := runNem(t, nemHome, "catalog", "lint", recipe)
	if err == nil {
		t.Fatalf("a broken manifest must produce findings and exit 1:\n%s", errb)
	}
}
