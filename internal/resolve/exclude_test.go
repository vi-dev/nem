package resolve_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/project"
	"github.com/vi-dev/nem/internal/resolve"
)

func writeExcluded(t *testing.T, root, name, extra, versions string) {
	t.Helper()
	writePkg(t, root, "schema: 4\nname: "+name+"\n"+extra+
		"artifact: {oci: \":{{.Version}}\"}\ninstall: [{extract: {}}]\nversions: "+versions+"\n")
}

func resolveOne(t *testing.T, root, name, version string) (*resolve.Result, error) {
	t.Helper()
	tools := []resolve.Tool{{Key: project.ToolKey{Name: name}, Version: version}}
	return resolve.Resolve(context.Background(), tools, namedSources(root))
}

func TestResolveFloatingSkipsExcludedHead(t *testing.T) {
	root := t.TempDir()
	writeExcluded(t, root, "a", "", `[{version: v2.0.0, excluded: "CVE-1"}, v1.0.0]`)
	res, err := resolveOne(t, root, "a", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if e := entry(t, res, "a"); e.Version != "v1.0.0" {
		t.Fatalf("floating must skip the excluded head: %+v", e)
	}
}

func TestResolvePinnedExcludedFailsWithReason(t *testing.T) {
	root := t.TempDir()
	writeExcluded(t, root, "a", "", `[v2.1.0, {version: v2.0.0, excluded: {reason: "CVE-1: leaks", use: v2.1.0}}]`)
	_, err := resolveOne(t, root, "a", "v2.0.0")
	ee, ok := errors.AsType[*catalog.VersionExcludedError](err)
	if !ok {
		t.Fatalf("err = %v, want VersionExcludedError", err)
	}
	if ee.Use != "v2.1.0" || ee.RequiredBy != "" {
		t.Fatalf("error: %+v", ee)
	}
	if want := "version v2.0.0 of package a is excluded by catalog cat: CVE-1: leaks"; err.Error() != want {
		t.Fatalf("message %q, want %q", err.Error(), want)
	}
}

func TestResolveFloatingDepSkipsExcluded(t *testing.T) {
	root := t.TempDir()
	writeExcluded(t, root, "lib", "", `[{version: v2.0.0, excluded: "bad"}, v1.5.0]`)
	writeSpec(t, root, pkgSpec{name: "app", deps: []string{"{name: lib}"}, versions: "[v1.0.0]"})
	res, err := resolveOne(t, root, "app", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if e := entry(t, res, "lib"); e.Version != "v1.5.0" {
		t.Fatalf("lib: %+v", e)
	}
}

func TestResolveRangedDepSkipsExcluded(t *testing.T) {
	root := t.TempDir()
	writeExcluded(t, root, "lib", "libs: [lib]\n", `[v2.0.0, {version: v1.9.0, excluded: "bad"}, v1.8.0]`)
	writeSpec(t, root, pkgSpec{name: "app", deps: []string{`{name: lib, kind: link, compat: "1"}`}, versions: "[v1.0.0]"})
	res, err := resolveOne(t, root, "app", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if e := entry(t, res, "lib"); e.Version != "v1.8.0" {
		t.Fatalf("the range must take its highest non-excluded match: %+v", e)
	}
}

func TestResolveRangedDepWithOnlyExcludedMatches(t *testing.T) {
	root := t.TempDir()
	writeExcluded(t, root, "lib", "libs: [lib]\n", `[v2.0.0, {version: v1.9.0, excluded: "bad"}]`)
	writeSpec(t, root, pkgSpec{name: "app", deps: []string{`{name: lib, kind: link, compat: "1"}`}, versions: "[v1.0.0]"})
	_, err := resolveOne(t, root, "app", "")
	if _, ok := errors.AsType[*catalog.VersionNotFoundError](err); !ok {
		t.Fatalf("err = %v, want VersionNotFoundError", err)
	}
}

func TestResolveExactDepOnExcludedNamesDependent(t *testing.T) {
	root := t.TempDir()
	writeExcluded(t, root, "lib", "", `[v2.0.0, {version: v1.3.0, excluded: "bad"}]`)
	writeSpec(t, root, pkgSpec{name: "app", deps: []string{"lib@v1.3.0"}, versions: "[v1.4.0]"})
	_, err := resolveOne(t, root, "app", "")
	ee, ok := errors.AsType[*catalog.VersionExcludedError](err)
	if !ok {
		t.Fatalf("err = %v, want VersionExcludedError", err)
	}
	if ee.RequiredBy != "app@v1.4.0" || !strings.Contains(err.Error(), "(required by app@v1.4.0)") {
		t.Fatalf("error: %v", err)
	}
}
