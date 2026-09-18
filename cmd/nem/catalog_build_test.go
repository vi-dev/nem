package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vi-dev/nem/internal/build"
	"github.com/vi-dev/nem/internal/home"
	"github.com/vi-dev/nem/internal/spec"
)

func TestCatalogBuildStreamsBuildStepOutputToTheCommandStreams(t *testing.T) {
	nemHomeDir := t.TempDir()
	tgz := makeTarGz(t, map[string]string{"src/README": "hi"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(tgz) }))
	defer srv.Close()

	dir := writeLintFixture(t, map[string]string{
		"tool": "schema: 2\nname: tool\n" +
			"artifact: {oci: \":{{.Version}}\"}\ninstall: [{extract: {}}]\n" +
			"versions: [v1.0.0]\nbuild:\n  source: {url: \"" + srv.URL + "\"}\n  output: out\n" +
			"  steps:\n    - run: mkdir -p \"$NEM_OUTPUT\" && echo step-stdout-marker && echo step-stderr-marker >&2\n",
	})

	out, errb, err := runNem(t, nemHomeDir, "catalog", "build", dir, "--package", "tool@v1.0.0")
	if err != nil {
		t.Fatalf("catalog build: %v\n%s", err, errb)
	}
	if !strings.Contains(out, "step-stdout-marker") {
		t.Errorf("build step stdout missing from the command's stdout:\n%s", out)
	}
	if !strings.Contains(errb, "step-stderr-marker") {
		t.Errorf("build step stderr missing from the command's stderr:\n%s", errb)
	}
}

func TestCatalogBuildSkipsTestHookWhenManifestDeclaresNoTests(t *testing.T) {
	nemHomeDir := t.TempDir()
	tgz := makeTarGz(t, map[string]string{"src/README": "hi"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(tgz) }))
	defer srv.Close()

	dir := writeLintFixture(t, map[string]string{
		"tool": "schema: 2\nname: tool\n" +
			"artifact: {oci: \":{{.Version}}\"}\ninstall: [{extract: {}}]\n" +
			"versions: [v1.0.0]\nbuild:\n  source: {url: \"" + srv.URL + "\"}\n  output: out\n" +
			"  steps:\n    - run: mkdir -p \"$NEM_OUTPUT\" && echo built > \"$NEM_OUTPUT/marker\"\n",
	})

	called := false
	orig := runPkgTest
	runPkgTest = func(ctx context.Context, h home.Home, deps []build.ResolvedDep,
		pkg *spec.Package, version, catalogName, artifactPath string) error {
		called = true
		return nil
	}
	defer func() { runPkgTest = orig }()

	_, errb, err := runNem(t, nemHomeDir, "catalog", "build", dir, "--package", "tool@v1.0.0")
	if err != nil {
		t.Fatalf("catalog build: %v\n%s", err, errb)
	}
	if called {
		t.Fatal("build must not invoke the test hook for a manifest with no test: steps")
	}
}

func TestCatalogBuildPushFlagDryRunNamesTheTarget(t *testing.T) {
	nemHomeDir := t.TempDir()
	specs := writeLintFixture(t, map[string]string{
		"tool": "schema: 2\nname: tool\n" +
			"artifact: {oci: \":{{.Version}}\"}\ninstall: [{extract: {}}]\nversions: [v1.0.0]\n" +
			"build:\n  source: {url: \"https://example.com/tool.tar.gz\"}\n  output: out\n" +
			"  steps:\n    - run: make\n",
	})

	t.Run("directory target", func(t *testing.T) {
		_, errb, err := runNem(t, nemHomeDir, "catalog", "build", specs,
			"--package", "tool@v1.0.0", "--push", "--dry-run")
		if err != nil {
			t.Fatalf("catalog build --push --dry-run: %v\n%s", err, errb)
		}
		if !strings.Contains(errb, "Dry-run") || !strings.Contains(errb, specs) {
			t.Fatalf("dry-run must name where the archive would land:\n%s", errb)
		}
	})
}
