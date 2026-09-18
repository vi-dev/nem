package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vi-dev/nem/internal/spec"
)

const fmtFixture = lintFixtureGoPkg

func TestCatalogFmtRewritesToCanonical(t *testing.T) {
	nemHome := t.TempDir()
	dir := writeLintFixture(t, map[string]string{"go": fmtFixture})
	path := filepath.Join(dir, "pkgs", "go", "pkg.yaml")

	if _, _, err := runNem(t, nemHome, "catalog", "fmt", dir); err != nil {
		t.Fatalf("fmt: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	canon, err := spec.Format(data)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(canon) {
		t.Fatal("file is not canonical after fmt")
	}

	before, _ := os.ReadFile(path)
	if _, _, err := runNem(t, nemHome, "catalog", "fmt", dir); err != nil {
		t.Fatalf("second fmt: %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("second fmt changed the file")
	}
}

func TestCatalogFmtScopedToPackage(t *testing.T) {
	nemHome := t.TempDir()
	// Two packages with non-canonical manifests, same fixture content the
	// tests above use, in one catalog dir.
	dir := writeLintFixture(t, map[string]string{"alpha": fmtFixture, "beta": fmtFixture})
	alphaPath := filepath.Join(dir, "pkgs", "alpha", "pkg.yaml")
	betaPath := filepath.Join(dir, "pkgs", "beta", "pkg.yaml")

	alphaBefore, err := os.ReadFile(alphaPath)
	if err != nil {
		t.Fatal(err)
	}
	betaBefore, err := os.ReadFile(betaPath)
	if err != nil {
		t.Fatal(err)
	}

	out, errb, err := runNem(t, nemHome, "catalog", "fmt", dir, "--package", "alpha")
	if err != nil {
		t.Fatalf("scoped fmt: %v", err)
	}
	if !strings.Contains(out+errb, "Formatted alpha") {
		t.Fatalf("want a Formatted alpha line (name, not path), got:\n%s\n%s", out, errb)
	}

	alphaAfter, err := os.ReadFile(alphaPath)
	if err != nil {
		t.Fatal(err)
	}
	betaAfter, err := os.ReadFile(betaPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(alphaBefore, alphaAfter) {
		t.Fatal("alpha should have been rewritten by scoped fmt")
	}
	if !bytes.Equal(betaBefore, betaAfter) {
		t.Fatal("beta must not be touched by fmt scoped to alpha")
	}
}

func TestCatalogFmtFormatsInvalidButParseableManifest(t *testing.T) {
	nemHome := t.TempDir()
	dir := t.TempDir()
	pkgDir := filepath.Join(dir, "pkgs", "wip")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Parseable but fails spec.Validate (no artifact, no install) and
	// non-canonical so fmt has something to rewrite.
	writeFile(t, filepath.Join(pkgDir, "pkg.yaml"), "schema:   2\nname:  wip\n")

	out, errb, err := runNem(t, nemHome, "catalog", "fmt", dir, "--package", "wip")
	if err != nil {
		t.Fatalf("fmt must format WIP manifests: %v\n%s\n%s", err, out, errb)
	}
	if !strings.Contains(out+errb, "Formatted wip") {
		t.Fatalf("want a Formatted wip line, got:\n%s\n%s", out, errb)
	}
}

func TestCatalogFmtFormatsManifestPathAsFileCatalog(t *testing.T) {
	nemHome := t.TempDir()
	dir := t.TempDir()
	recipe := filepath.Join(dir, "pkg.yaml")
	writeFile(t, recipe, "schema:   2\nname:  tool\n")

	out, errb, err := runNem(t, nemHome, "catalog", "fmt", recipe)
	if err != nil {
		t.Fatalf("fmt on a pkg.yaml must format it in place: %v\n%s\n%s", err, out, errb)
	}
	if !strings.Contains(out+errb, "Formatted tool") {
		t.Fatalf("want the declared name in output, got:\n%s\n%s", out, errb)
	}
	after, err := os.ReadFile(recipe)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) == "schema:   2\nname:  tool\n" {
		t.Fatal("file must be rewritten to canonical form")
	}

	if _, _, err := runNem(t, nemHome, "catalog", "fmt", recipe, "--package", "tool"); err != nil {
		t.Fatalf("--package with the declared name must work: %v", err)
	}
	if _, _, err := runNem(t, nemHome, "catalog", "fmt", recipe, "--package", "nosuch"); err == nil {
		t.Fatal("--package with a non-matching name must error")
	}
}
