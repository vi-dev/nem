package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vi-dev/nem/internal/archive"
	"github.com/vi-dev/nem/internal/home"
)

func assertNoLeakedTestAlias(t *testing.T, nemHome string) {
	t.Helper()
	root := filepath.Join(nemHome, "packages")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && strings.Contains(d.Name(), home.TestInstallInfix) {
			t.Errorf("leaked test alias directory: %s", path)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("walk %s: %v", root, err)
	}
}

func assertStaged(t *testing.T, catalogDir, name, version string) {
	t.Helper()
	if !archive.NewDir(catalogDir).HasVersion(name, version) {
		t.Fatalf("%s@%s is not staged under %s", name, version,
			filepath.Join(catalogDir, "archives", name))
	}
}

func assertNothingStaged(t *testing.T, catalogDir, name string) {
	t.Helper()
	if archive.NewDir(catalogDir).Has(name) {
		t.Fatalf("a failed build staged archives under %s",
			filepath.Join(catalogDir, "archives", name))
	}
}

func seedFooLinkDep(t *testing.T, cc, nemHome string) {
	t.Helper()

	fooDir := filepath.Join(nemHome, "packages", "foo", "v1")
	includeDir := filepath.Join(fooDir, "include")
	if err := os.MkdirAll(includeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(includeDir, "foo.h"), "int foo_v(void);\n")

	libDir := filepath.Join(fooDir, "lib")
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "foo.c")
	writeFile(t, src, "int foo_v(void){return 7;}\n")
	switch runtime.GOOS {
	case "darwin":
		run(t, cc, "-dynamiclib", "-install_name", "@rpath/libfoo.dylib",
			"-o", filepath.Join(libDir, "libfoo.dylib"), src)
	case "linux":
		run(t, cc, "-shared", "-fPIC", "-Wl,-soname,libfoo.so",
			"-o", filepath.Join(libDir, "libfoo.so"), src)
	}

	binDir := filepath.Join(fooDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "foocli"), []byte("#!/bin/sh\necho foocli\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	writeMetaFile(t, fooDir, "package: foo\nversion: v1\ncatalog: seed\nbins: [bin]\nlibs: [lib]\n")

	catDir := t.TempDir()
	pkgDir := filepath.Join(catDir, "pkgs", "foo")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(pkgDir, "pkg.yaml"), "schema: 2\nname: foo\nlibs: [lib]\n"+
		"artifact: {oci: \":{{.Version}}\"}\ninstall: [{extract: {}}]\nversions: [v1]\n")

	if _, errb, err := runNem(t, nemHome, "catalog", "add", "seed", catDir); err != nil {
		t.Fatalf("catalog add seed: %v\n%s", err, errb)
	}
}

func useCTarball(t *testing.T) *httptest.Server {
	t.Helper()
	tgz := makeTarGz(t, map[string]string{
		"src/use.c": "#include <foo.h>\nint foo_v(void);\nint main(void){return foo_v();}\n",
	})
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(tgz) }))
}

func buildCatalog(t *testing.T, sourceURL, buildStep string) string {
	t.Helper()
	return buildRoleCatalog(t, sourceURL, "link", buildStep)
}

func buildRoleCatalog(t *testing.T, sourceURL, kind, buildStep string) string {
	t.Helper()
	return writeLintFixture(t, map[string]string{
		"tool": "schema: 2\nname: tool\n" +
			"artifact: {oci: \":{{.Version}}\"}\ninstall: [{extract: {}}]\n" +
			"versions: [v1.0.0]\nbuild:\n  source: {url: \"" + sourceURL + "\"}\n" +
			"  deps: [{name: foo, kind: " + kind + "}]\n  output: out\n" +
			"  steps:\n    - run: " + buildStep + "\n",
	})
}

func TestCatalogBuildLinksAgainstDepViaScaffold(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("loader semantics differ off darwin/linux")
	}
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("cc not available")
	}

	nemHome := t.TempDir()
	seedFooLinkDep(t, cc, nemHome)

	srv := useCTarball(t)
	defer srv.Close()

	cat := buildCatalog(t, srv.URL,
		`mkdir -p "$NEM_OUTPUT/bin" && cc $CPPFLAGS -o "$NEM_OUTPUT/bin/usefoo" use.c $LDFLAGS -lfoo`)

	_, errb, err := runNem(t, nemHome, "catalog", "build", cat, "--package", "tool@v1.0.0", "--push")
	if err != nil {
		t.Fatalf("catalog build: %v\n%s", err, errb)
	}
	assertStaged(t, cat, "tool", "v1.0.0")
}

func TestCatalogBuildRejectsAbsoluteRpathIntoPackages(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("loader semantics differ off darwin/linux")
	}
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("cc not available")
	}

	nemHome := t.TempDir()
	seedFooLinkDep(t, cc, nemHome)

	srv := useCTarball(t)
	defer srv.Close()

	badRpath := filepath.Join(nemHome, "packages", "foo", "v1", "lib")
	cat := buildCatalog(t, srv.URL,
		`mkdir -p "$NEM_OUTPUT/bin" && cc $CPPFLAGS -o "$NEM_OUTPUT/bin/usefoo" use.c $LDFLAGS -lfoo -Wl,-rpath,`+badRpath)

	out, errb, err := runNem(t, nemHome, "catalog", "build", cat, "--package", "tool@v1.0.0", "--push")
	if err == nil {
		t.Fatal("catalog build must fail: recipe bakes an absolute rpath into packages")
	}
	row := tableRow(out, 0, "tool")
	if row == nil || row[2] != "failed" {
		t.Fatalf("tool row = %v, want a failed row:\n%s\nstderr: %s", row, out, errb)
	}
	if !strings.Contains(strings.Join(row, " "), "conformance violation") {
		t.Fatalf("the failed row must name the conformance check, got %v", row)
	}
	if !strings.Contains(errb, badRpath) {
		t.Fatalf("stderr must name the offending rpath %s:\n%s", badRpath, errb)
	}
	assertNothingStaged(t, cat, "tool")
}

const probeStep = `mkdir -p "$NEM_OUTPUT" && command -v foocli >/dev/null && echo ON_PATH > "$NEM_OUTPUT/probe"`

func TestCatalogBuildPutsDirectLinkDepBinsOnPath(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("loader semantics differ off darwin/linux")
	}
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("cc not available")
	}

	nemHome := t.TempDir()
	seedFooLinkDep(t, cc, nemHome)

	srv := useCTarball(t)
	defer srv.Close()

	cat := buildRoleCatalog(t, srv.URL, "link", probeStep)

	_, errb, err := runNem(t, nemHome, "catalog", "build", cat, "--package", "tool@v1.0.0", "--push")
	if err != nil {
		t.Fatalf("a kind: link build dep's bins must join the build PATH: %v\n%s", err, errb)
	}
	assertStaged(t, cat, "tool", "v1.0.0")
}

func testedCatalog(t *testing.T, sourceURL, buildStep, testStep string) string {
	t.Helper()
	return writeLintFixture(t, map[string]string{
		"tool": "schema: 2\nname: tool\n" +
			"artifact: {oci: \":{{.Version}}\"}\ninstall: [{extract: {}}]\n" +
			"versions: [v1.0.0]\nbuild:\n  source: {url: \"" + sourceURL + "\"}\n" +
			"  output: out\n" +
			"  steps:\n    - run: " + buildStep + "\n" +
			"test:\n  - run: " + testStep + "\n",
	})
}

func helloTarball(t *testing.T) *httptest.Server {
	t.Helper()
	tgz := makeTarGz(t, map[string]string{"src/placeholder": "x\n"})
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(tgz) }))
}

const makeHelloBin = `mkdir -p "$NEM_OUTPUT/bin" && printf '#!/bin/sh\necho hi\n' > "$NEM_OUTPUT/bin/hello" && chmod +x "$NEM_OUTPUT/bin/hello"`

func TestCatalogBuildRunsDeclaredTests(t *testing.T) {
	nemHome := t.TempDir()
	srv := helloTarball(t)
	defer srv.Close()

	cat := testedCatalog(t, srv.URL, makeHelloBin, `hello | grep -q hi`)
	if _, errb, err := runNem(t, nemHome, "catalog", "build", cat,
		"--package", "tool@v1.0.0", "--push"); err != nil {
		t.Fatalf("catalog build: %v\n%s", err, errb)
	}
	assertStaged(t, cat, "tool", "v1.0.0")
	if _, err := os.Stat(filepath.Join(nemHome, "packages", "tool", "v1.0.0")); !os.IsNotExist(err) {
		t.Fatalf("the staged tree must not stay installed, stat err = %v", err)
	}
	assertNoLeakedTestAlias(t, nemHome)
}

func TestCatalogBuildFailsOnFailingTest(t *testing.T) {
	nemHome := t.TempDir()
	srv := helloTarball(t)
	defer srv.Close()

	cat := testedCatalog(t, srv.URL, makeHelloBin, `exit 1`)
	out, errb, err := runNem(t, nemHome, "catalog", "build", cat, "--package", "tool@v1.0.0", "--push")
	if err == nil {
		t.Fatal("want catalog build to fail when a test step fails")
	}

	row := tableRow(out, 0, "tool")
	if row == nil || row[2] != "failed" {
		t.Fatalf("tool row = %v, want a failed row:\n%s\nstderr: %s", row, out, errb)
	}
	if !strings.Contains(strings.Join(row, " "), "test step 1") {
		t.Fatalf("the failed row must name the failing step, got %v", row)
	}
	assertNothingStaged(t, cat, "tool")
	if _, err := os.Stat(filepath.Join(nemHome, "packages", "tool", "v1.0.0")); !os.IsNotExist(err) {
		t.Fatalf("a failed build must leave nothing staged, stat err = %v", err)
	}
	assertNoLeakedTestAlias(t, nemHome)
}
