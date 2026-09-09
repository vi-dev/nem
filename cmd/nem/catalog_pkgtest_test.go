package main

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vi-dev/nem/internal/spec"
)

func TestCatalogTestNoTestSectionIsClean(t *testing.T) {
	nemHome := t.TempDir()
	dir := t.TempDir()
	recipe := filepath.Join(dir, "pkg.yaml")
	writeFile(t, recipe, "schema: 2\nname: tool\n"+
		"artifact: {oci: \":{{.Version}}\"}\ninstall: [{extract: {}}]\n"+
		"versions: [{version: \"1.0.0\"}]\n")

	out, errb, err := runNem(t, nemHome, "catalog", "test", recipe)
	if err != nil {
		t.Fatalf("catalog test: %v\n%s", err, errb)
	}
	if !strings.Contains(out+errb, "declares no tests") {
		t.Fatalf("want a no-tests notice, got:\n%s\n%s", out, errb)
	}
}

func TestCatalogTestInstallsAndRunsDeclaredTests(t *testing.T) {
	nemHome := t.TempDir()
	catalogRoot := downloadableDirCatalog(t, `test:
  - run: 'test -f "$NEM_PREFIX/bin/tool"'
`)

	if _, errb, err := runNem(t, nemHome, "catalog", "add", "demo", catalogRoot); err != nil {
		t.Fatalf("catalog add: %v\n%s", err, errb)
	}

	recipe := filepath.Join(catalogRoot, "pkgs", "tool", "pkg.yaml")
	out, errb, err := runNem(t, nemHome, "catalog", "test", recipe)
	if err != nil {
		t.Fatalf("catalog test: %v\nstdout: %s\nstderr: %s", err, out, errb)
	}
	if !strings.Contains(errb, "Tested tool v1.0.0 (1 step)") {
		t.Fatalf("want a tested-successfully notice, got:\n%s\n%s", out, errb)
	}
	if n := strings.Count(out+errb, "Tested"); n != 1 {
		t.Fatalf("want exactly one completion line, got %d:\n%s\n%s", n, out, errb)
	}
	if _, err := os.Stat(filepath.Join(nemHome, "packages", "tool", "v1.0.0")); !os.IsNotExist(err) {
		t.Fatalf("catalog test must not leave a real install behind, stat err = %v", err)
	}
	assertNoLeakedTestAlias(t, nemHome)
}

func TestCatalogTestResolvesDepsFromTheCatalogNotTheLocalManifest(t *testing.T) {
	nemHome := t.TempDir()
	catalogRoot := t.TempDir()

	depArchive := makeTarGz(t, map[string]string{"marker": "dep bytes"})
	depSum := sha256.Sum256(depArchive)
	depSha := hex.EncodeToString(depSum[:])
	depSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(depArchive) }))
	defer depSrv.Close()

	toolArchive := makeTarGz(t, map[string]string{"bin/tool": "tool binary bytes"})
	toolSum := sha256.Sum256(toolArchive)
	toolSha := hex.EncodeToString(toolSum[:])
	toolSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(toolArchive) }))
	defer toolSrv.Close()

	if err := os.MkdirAll(filepath.Join(catalogRoot, "pkgs", "dep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(catalogRoot, "pkgs", "tool"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(catalogRoot, "pkgs", "dep", "pkg.yaml"), `schema: 2
name: dep
artifact: {url: "`+depSrv.URL+`"}
install: [{extract: {}}]
versions:
  - version: v1.0.0
    sha256:
      darwin/arm64: "`+depSha+`"
      darwin/amd64: "`+depSha+`"
      linux/arm64: "`+depSha+`"
      linux/amd64: "`+depSha+`"
`)
	writeFile(t, filepath.Join(catalogRoot, "pkgs", "tool", "pkg.yaml"), `schema: 2
name: tool
deps:
  - name: dep
artifact: {url: "`+toolSrv.URL+`"}
install: [{extract: {}}]
versions:
  - version: v1.0.0
    sha256:
      darwin/arm64: "`+toolSha+`"
      darwin/amd64: "`+toolSha+`"
      linux/arm64: "`+toolSha+`"
      linux/amd64: "`+toolSha+`"
`)

	if _, errb, err := runNem(t, nemHome, "catalog", "add", "demo", catalogRoot); err != nil {
		t.Fatalf("catalog add: %v\n%s", err, errb)
	}

	dir := t.TempDir()
	recipe := filepath.Join(dir, "pkg.yaml")
	writeFile(t, recipe, `schema: 2
name: tool
artifact: {oci: ":{{.Version}}"}
install: [{extract: {}}]
versions: [{version: "1.0.0"}]
test:
  - run: 'test -n "$NEM_DEP_DEP_PREFIX"'
`)

	out, errb, err := runNem(t, nemHome, "catalog", "test", recipe)
	if err != nil {
		t.Fatalf("catalog test: %v\nstdout: %s\nstderr: %s", err, out, errb)
	}
	if !strings.Contains(errb, "Tested tool v1.0.0 (1 step)") {
		t.Fatalf("want a tested-successfully notice, got:\n%s\n%s", out, errb)
	}
	if _, err := os.Stat(filepath.Join(nemHome, "packages", "dep", "v1.0.0")); err != nil {
		t.Fatalf("the catalog-resolved dep must be installed under its real name: %v", err)
	}
	if _, err := os.Stat(filepath.Join(nemHome, "packages", "tool", "v1.0.0")); !os.IsNotExist(err) {
		t.Fatalf("catalog test must not leave a real install of the package under test, stat err = %v", err)
	}
	assertNoLeakedTestAlias(t, nemHome)
}

func TestCatalogTestSkipsAPackageUnsupportedHere(t *testing.T) {
	nemHome := t.TempDir()
	dir := t.TempDir()
	other := "linux/amd64"
	if spec.Current().String() == other {
		other = "darwin/arm64"
	}
	recipe := filepath.Join(dir, "pkg.yaml")
	writeFile(t, recipe, "schema: 2\nname: tool\nplatforms: ["+other+"]\n"+
		"artifact: {oci: \":{{.Version}}\"}\ninstall: [{extract: {}}]\n"+
		"versions: [{version: \"1.0.0\"}]\ntest:\n  - run: \"exit 1\"\n")

	out, errb, err := runNem(t, nemHome, "catalog", "test", recipe)
	if err != nil {
		t.Fatalf("catalog test: %v\n%s", err, errb)
	}
	if !strings.Contains(out+errb, "does not support") {
		t.Fatalf("want an unsupported-platform notice, got:\n%s\n%s", out, errb)
	}
}
