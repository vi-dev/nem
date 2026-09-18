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

const toolTestStep = "test:\n  - run: 'test -f \"$NEM_PREFIX/bin/tool\"'\n"

func TestCatalogTestNoTestSectionIsClean(t *testing.T) {
	nemHome := t.TempDir()
	root := t.TempDir()
	dir := filepath.Join(root, "pkgs", "tool")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "pkg.yaml"), "schema: 2\nname: tool\n"+
		"artifact: {oci: \":{{.Version}}\"}\ninstall: [{extract: {}}]\n"+
		"versions: [{version: \"1.0.0\"}]\n")

	out, errb, err := runNem(t, nemHome, "catalog", "test", root, "--package", "tool")
	if err != nil {
		t.Fatalf("catalog test: %v\n%s", err, errb)
	}
	if !strings.Contains(out+errb, "declares no tests") {
		t.Fatalf("want a no-tests notice, got:\n%s\n%s", out, errb)
	}
}

func TestCatalogTestInstallsAndRunsDeclaredTests(t *testing.T) {
	nemHome := t.TempDir()
	catalogRoot := downloadableDirCatalog(t, toolTestStep)

	if _, errb, err := runNem(t, nemHome, "catalog", "add", "demo", catalogRoot); err != nil {
		t.Fatalf("catalog add: %v\n%s", err, errb)
	}

	out, errb, err := runNem(t, nemHome, "catalog", "test", catalogRoot, "--package", "tool")
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

func TestCatalogTestInstallsResolvedDeps(t *testing.T) {
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

	for _, d := range []string{"dep", "tool"} {
		if err := os.MkdirAll(filepath.Join(catalogRoot, "pkgs", d), 0o755); err != nil {
			t.Fatal(err)
		}
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
test:
  - run: 'test -n "$NEM_DEP_DEP_PREFIX"'
`)

	out, errb, err := runNem(t, nemHome, "catalog", "test", catalogRoot, "--package", "tool")
	if err != nil {
		t.Fatalf("catalog test: %v\nstdout: %s\nstderr: %s", err, out, errb)
	}
	if !strings.Contains(errb, "Tested tool v1.0.0 (1 step)") {
		t.Fatalf("want a tested-successfully notice, got:\n%s\n%s", out, errb)
	}
	if _, err := os.Stat(filepath.Join(nemHome, "packages", "dep", "v1.0.0")); err != nil {
		t.Fatalf("the resolved dep must be installed under its real name: %v", err)
	}
	if _, err := os.Stat(filepath.Join(nemHome, "packages", "tool", "v1.0.0")); !os.IsNotExist(err) {
		t.Fatalf("catalog test must not leave a real install of the package under test, stat err = %v", err)
	}
	assertNoLeakedTestAlias(t, nemHome)
}

func TestCatalogTestSkipsAPackageUnsupportedHere(t *testing.T) {
	nemHome := t.TempDir()
	root := t.TempDir()
	other := "linux/amd64"
	if spec.Current().String() == other {
		other = "darwin/arm64"
	}
	dir := filepath.Join(root, "pkgs", "tool")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "pkg.yaml"), "schema: 2\nname: tool\nplatforms: ["+other+"]\n"+
		"artifact: {oci: \":{{.Version}}\"}\ninstall: [{extract: {}}]\n"+
		"versions: [{version: \"1.0.0\"}]\ntest:\n  - run: \"exit 1\"\n")

	out, errb, err := runNem(t, nemHome, "catalog", "test", root, "--package", "tool")
	if err != nil {
		t.Fatalf("catalog test: %v\n%s", err, errb)
	}
	if !strings.Contains(out+errb, "does not support") {
		t.Fatalf("want an unsupported-platform notice, got:\n%s\n%s", out, errb)
	}
}

func TestCatalogTestResolvesUnconfiguredCheckout(t *testing.T) {
	nemHome := t.TempDir()
	catalogRoot := downloadableDirCatalog(t, toolTestStep)

	out, errb, err := runNem(t, nemHome, "catalog", "test", catalogRoot, "--package", "tool")
	if err != nil {
		t.Fatalf("catalog test: %v\nstdout: %s\nstderr: %s", err, out, errb)
	}
	if !strings.Contains(errb, "Tested tool v1.0.0 (1 step)") {
		t.Fatalf("want a tested-successfully notice, got:\n%s\n%s", out, errb)
	}
}

func TestCatalogTestDefaultsToCurrentDirectory(t *testing.T) {
	nemHome := t.TempDir()
	catalogRoot := downloadableDirCatalog(t, toolTestStep)
	chdir(t, catalogRoot)

	out, errb, err := runNem(t, nemHome, "catalog", "test", "--package", "tool")
	if err != nil {
		t.Fatalf("catalog test: %v\nstdout: %s\nstderr: %s", err, out, errb)
	}
	if !strings.Contains(errb, "Tested tool v1.0.0 (1 step)") {
		t.Fatalf("want a tested-successfully notice, got:\n%s\n%s", out, errb)
	}
}

func TestCatalogTestRejectsUnknownPackage(t *testing.T) {
	nemHome := t.TempDir()
	catalogRoot := downloadableDirCatalog(t, toolTestStep)

	_, _, err := runNem(t, nemHome, "catalog", "test", catalogRoot, "--package", "nosuch")
	if err == nil || !strings.Contains(err.Error(), "nosuch") {
		t.Fatalf("an unknown --package name must be a hard error, got: %v", err)
	}
}

func TestCatalogTestAcceptsRecipePathAsFileCatalog(t *testing.T) {
	nemHome := t.TempDir()
	catalogRoot := downloadableDirCatalog(t, toolTestStep)
	recipe := filepath.Join(catalogRoot, "pkgs", "tool", "pkg.yaml")

	out, errb, err := runNem(t, nemHome, "catalog", "test", recipe)
	if err != nil {
		t.Fatalf("catalog test on a pkg.yaml must run it as a single-file catalog: %v\nstdout: %s\nstderr: %s", err, out, errb)
	}
	if !strings.Contains(errb, "Tested tool v1.0.0 (1 step)") {
		t.Fatalf("want a tested-successfully notice, got:\n%s\n%s", out, errb)
	}

	if _, _, err := runNem(t, nemHome, "catalog", "test", recipe, "--package", "nosuch"); err == nil {
		t.Fatal("--package with a non-matching name must error")
	}
}

func TestCatalogTestWholeCatalogRunsEveryTestedPackage(t *testing.T) {
	nemHome := t.TempDir()
	catalogRoot := downloadableDirCatalog(t, toolTestStep)
	nakedDir := filepath.Join(catalogRoot, "pkgs", "naked")
	if err := os.MkdirAll(nakedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(nakedDir, "pkg.yaml"), "schema: 2\nname: naked\n"+
		"artifact: {oci: \":{{.Version}}\"}\ninstall: [{extract: {}}]\n"+
		"versions: [{version: \"1.0.0\"}]\n")

	out, errb, err := runNem(t, nemHome, "catalog", "test", catalogRoot)
	if err != nil {
		t.Fatalf("catalog test: %v\nstdout: %s\nstderr: %s", err, out, errb)
	}
	if !strings.Contains(errb, "Tested tool v1.0.0 (1 step)") {
		t.Fatalf("want the tested package's completion line, got:\n%s\n%s", out, errb)
	}
	if !strings.Contains(out+errb, "naked declares no tests") {
		t.Fatalf("want a skip notice for the untested package, got:\n%s\n%s", out, errb)
	}
}

func TestCatalogTestEmptyCatalogReportsNothingToTest(t *testing.T) {
	nemHome := t.TempDir()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkgs"), 0o755); err != nil {
		t.Fatal(err)
	}

	out, errb, err := runNem(t, nemHome, "catalog", "test", root)
	if err != nil {
		t.Fatalf("catalog test: %v\n%s", err, errb)
	}
	if !strings.Contains(out+errb, "Nothing to test") {
		t.Fatalf("want a nothing-to-test notice, got:\n%s\n%s", out, errb)
	}
}

func twoVersionDirCatalog(t *testing.T) string {
	t.Helper()
	archive := makeTarGz(t, map[string]string{"bin/tool": "tool binary bytes"})
	sum := sha256.Sum256(archive)
	sha := hex.EncodeToString(sum[:])
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(archive)
	}))
	t.Cleanup(srv.Close)

	root := t.TempDir()
	dir := filepath.Join(root, "pkgs", "tool")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	sums := `
    sha256:
      darwin/arm64: "` + sha + `"
      darwin/amd64: "` + sha + `"
      linux/arm64: "` + sha + `"
      linux/amd64: "` + sha + `"`
	writeFile(t, filepath.Join(dir, "pkg.yaml"), `schema: 2
name: tool
artifact:
  url: "`+srv.URL+`"
install:
  - extract: {}
versions:
  - version: v1.1.0`+sums+`
  - version: v1.0.0`+sums+`
test:
  - run: 'test -f "$NEM_PREFIX/bin/tool"'
`)
	return root
}

func TestCatalogTestPinsVersionFromSelector(t *testing.T) {
	nemHome := t.TempDir()
	root := twoVersionDirCatalog(t)

	out, errb, err := runNem(t, nemHome, "catalog", "test", root, "--package", "tool@v1.0.0")
	if err != nil {
		t.Fatalf("catalog test: %v\nstdout: %s\nstderr: %s", err, out, errb)
	}
	if !strings.Contains(errb, "Tested tool v1.0.0 (1 step)") {
		t.Fatalf("want the pinned version tested, got:\n%s\n%s", out, errb)
	}
}

func TestCatalogTestDefaultsToLatestVersion(t *testing.T) {
	nemHome := t.TempDir()
	root := twoVersionDirCatalog(t)

	out, errb, err := runNem(t, nemHome, "catalog", "test", root, "--package", "tool")
	if err != nil {
		t.Fatalf("catalog test: %v\nstdout: %s\nstderr: %s", err, out, errb)
	}
	if !strings.Contains(errb, "Tested tool v1.1.0 (1 step)") {
		t.Fatalf("want the latest version tested, got:\n%s\n%s", out, errb)
	}
}

func TestCatalogTestDuplicateSelectorRunsOnce(t *testing.T) {
	nemHome := t.TempDir()
	catalogRoot := downloadableDirCatalog(t, toolTestStep)

	out, errb, err := runNem(t, nemHome, "catalog", "test", catalogRoot, "--package", "tool", "--package", "tool")
	if err != nil {
		t.Fatalf("catalog test: %v\nstdout: %s\nstderr: %s", err, out, errb)
	}
	if !strings.Contains(errb, "Tested tool v1.0.0 (1 step)") {
		t.Fatalf("want a tested-successfully notice, got:\n%s\n%s", out, errb)
	}
	if n := strings.Count(out+errb, "Tested"); n != 1 {
		t.Fatalf("duplicate --package selectors must run the test exactly once, got %d:\n%s\n%s", n, out, errb)
	}
}
