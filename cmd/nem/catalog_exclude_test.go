package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vi-dev/nem/internal/catalog"
)

func TestCatalogExcludeThenUseInfoAndUnexclude(t *testing.T) {
	nemHomeDir := t.TempDir()
	catalogRoot := writeInfoDirCatalog(t, map[string]string{"tool": infoPkgYAML})
	manifest := filepath.Join(catalogRoot, "pkgs", "tool", "pkg.yaml")
	original, _ := os.ReadFile(manifest)
	chdir(t, t.TempDir())

	if _, errb, err := runNem(t, nemHomeDir, "catalog", "exclude", catalogRoot, "--package", "tool@v2.1.0",
		"--reason", "CVE-2026-1234: leaks tokens", "--use", "v1.9.0"); err != nil {
		t.Fatalf("exclude: %v\n%s", err, errb)
	}
	edited, _ := os.ReadFile(manifest)
	if !strings.HasPrefix(string(edited), "schema: 4\n") || !strings.Contains(string(edited), "reason: \"CVE-2026-1234: leaks tokens\"") {
		t.Fatalf("manifest:\n%s", edited)
	}

	if _, _, err := runNem(t, nemHomeDir, "catalog", "add", "dev", catalogRoot); err != nil {
		t.Fatal(err)
	}
	out, _, err := runNem(t, nemHomeDir, "info", "tool")
	if err != nil || !strings.Contains(out, "v2.1.0 (excluded: CVE-2026-1234: leaks tokens), v1.9.0") {
		t.Fatalf("info: %v\n%s", err, out)
	}

	_, errb, err := runNem(t, nemHomeDir, "use", "tool@v2.1.0")
	if err == nil {
		t.Fatal("using an excluded version must fail")
	}
	if !strings.Contains(err.Error(), "is excluded by catalog dev: CVE-2026-1234: leaks tokens") {
		t.Fatalf("error: %v", err)
	}
	if !strings.Contains(errb, "Use tool@v1.9.0 instead") {
		t.Fatalf("stderr should carry the replacement hint: %q", errb)
	}

	if _, errb, err := runNem(t, nemHomeDir, "catalog", "unexclude", catalogRoot, "--package", "tool@v2.1.0"); err != nil {
		t.Fatalf("unexclude: %v\n%s", err, errb)
	}
	restored, _ := os.ReadFile(manifest)
	want := strings.Replace(strings.TrimPrefix(string(original), "\n"), "schema: 2", "schema: 4", 1)
	if string(restored) != want {
		t.Fatalf("unexclude must restore the file except for the schema line:\n%s\nwant:\n%s", restored, want)
	}
}

func TestCatalogExcludeRefusals(t *testing.T) {
	catalogRoot := writeInfoDirCatalog(t, map[string]string{"tool": infoPkgYAML})
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"catalog", "exclude", catalogRoot, "--package", "tool@v2.1.0"}, "--reason is required"},
		{[]string{"catalog", "exclude", catalogRoot, "--package", "tool", "--reason", "x"}, "name the version as name@version"},
		{[]string{"catalog", "exclude", catalogRoot, "--reason", "x"}, `required flag(s) "package" not set`},
		{[]string{"catalog", "exclude", catalogRoot, "--package", "tool@v2.1.0", "--package", "tool@v1.9.0", "--reason", "x", "--use", "v1.9.0"}, "--use needs a single --package"},
		{[]string{"catalog", "exclude", catalogRoot, "--package", "tool@v9.9.9", "--reason", "x"}, "version v9.9.9 of package tool not found"},
		{[]string{"catalog", "exclude", catalogRoot, "--package", "tool@v2.1.0", "--reason", "x", "--use", "v9.9.9"}, "excluded use v9.9.9 is not a listed version"},
		{[]string{"catalog", "unexclude", catalogRoot, "--package", "tool@v2.1.0"}, "is not excluded"},
	} {
		if _, _, err := runNem(t, t.TempDir(), tc.args...); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: err = %v, want %q", tc.args, err, tc.want)
		}
	}
}

func TestHintForExcludedVersion(t *testing.T) {
	for _, tc := range []struct {
		err  *catalog.VersionExcludedError
		want string
	}{
		{&catalog.VersionExcludedError{Name: "a", Version: "1", Use: "2"}, "Use a@2 instead"},
		{&catalog.VersionExcludedError{Name: "a", Version: "1"}, "Pick another version; `nem info a` lists them"},
		{&catalog.VersionExcludedError{Name: "a", Version: "1", Use: "2", RequiredBy: "b@3"}, "Package b@3 requires an excluded version; update or unuse it"},
	} {
		if got := hintFor(tc.err); got != tc.want {
			t.Errorf("hintFor(%+v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}

func TestCatalogExcludeClearsUseThatPointsAtIt(t *testing.T) {
	catalogRoot := writeInfoDirCatalog(t, map[string]string{"tool": infoPkgYAML + "  - v1.8.0\n"})
	manifest := filepath.Join(catalogRoot, "pkgs", "tool", "pkg.yaml")
	home := t.TempDir()
	if _, errb, err := runNem(t, home, "catalog", "exclude", catalogRoot, "--package", "tool@v1.9.0", "--reason", "old", "--use", "v2.1.0"); err != nil {
		t.Fatalf("exclude v1.9.0: %v\n%s", err, errb)
	}
	_, errb, err := runNem(t, home, "catalog", "exclude", catalogRoot, "--package", "tool@v2.1.0", "--reason", "also bad")
	if err != nil {
		t.Fatalf("exclude v2.1.0: %v\n%s", err, errb)
	}
	if !strings.Contains(errb, "tool@v1.9.0 no longer suggests v2.1.0") {
		t.Fatalf("stderr should report the cleared suggestion: %q", errb)
	}
	edited, _ := os.ReadFile(manifest)
	if strings.Contains(string(edited), "use:") || !strings.Contains(string(edited), "excluded: \"old\"") {
		t.Fatalf("manifest:\n%s", edited)
	}
	if _, _, err := runNem(t, home, "catalog", "exclude", catalogRoot, "--package", "tool@v1.8.0", "--reason", "x"); err == nil || !strings.Contains(err.Error(), "every version is excluded") {
		t.Fatalf("excluding the last offered version must be refused: %v", err)
	}
}

func TestCatalogExcludeSeveralVersions(t *testing.T) {
	catalogRoot := writeInfoDirCatalog(t, map[string]string{"tool": infoPkgYAML + "  - v1.8.0\n"})
	_, errb, err := runNem(t, t.TempDir(), "catalog", "exclude", catalogRoot,
		"--package", "tool@v2.1.0", "--package", "tool@v1.9.0", "--reason", "Broken build")
	if err != nil {
		t.Fatalf("exclude: %v\n%s", err, errb)
	}
	for _, want := range []string{"Excluded tool@v2.1.0", "Excluded tool@v1.9.0"} {
		if !strings.Contains(errb, want) {
			t.Fatalf("stderr %q should contain %q", errb, want)
		}
	}
	edited, _ := os.ReadFile(filepath.Join(catalogRoot, "pkgs", "tool", "pkg.yaml"))
	if strings.Count(string(edited), "excluded: \"Broken build\"") != 2 {
		t.Fatalf("manifest:\n%s", edited)
	}
}

func TestCatalogExcludeTwiceIsIdempotent(t *testing.T) {
	catalogRoot := writeInfoDirCatalog(t, map[string]string{"tool": infoPkgYAML})
	manifest := filepath.Join(catalogRoot, "pkgs", "tool", "pkg.yaml")
	home := t.TempDir()
	args := []string{"catalog", "exclude", catalogRoot, "--package", "tool@v2.1.0", "--reason", "CVE-1", "--use", "v1.9.0"}
	if _, errb, err := runNem(t, home, args...); err != nil {
		t.Fatalf("first exclude: %v\n%s", err, errb)
	}
	before, _ := os.ReadFile(manifest)

	_, errb, err := runNem(t, home, args...)
	if err != nil {
		t.Fatalf("excluding the same version twice must succeed: %v\n%s", err, errb)
	}
	if !strings.Contains(errb, "tool@v2.1.0 is already excluded") {
		t.Fatalf("stderr = %q", errb)
	}
	if after, _ := os.ReadFile(manifest); string(after) != string(before) {
		t.Fatalf("a repeated exclusion must not rewrite the manifest:\n%s", after)
	}

	_, errb, err = runNem(t, home, "catalog", "exclude", catalogRoot, "--package", "tool@v2.1.0", "--reason", "CVE-2")
	if err != nil {
		t.Fatalf("re-exclude with a new reason: %v\n%s", err, errb)
	}
	if !strings.Contains(errb, "Updated the exclusion of tool@v2.1.0") {
		t.Fatalf("stderr = %q", errb)
	}
	edited, _ := os.ReadFile(manifest)
	if !strings.Contains(string(edited), "excluded: \"CVE-2\"") || strings.Contains(string(edited), "use:") {
		t.Fatalf("the new reason and use must replace the old ones:\n%s", edited)
	}
}
