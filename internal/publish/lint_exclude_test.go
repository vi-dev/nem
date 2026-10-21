package publish

import (
	"context"
	"testing"
)

const libExcludedPkg = `schema: 4
name: lib
artifact: {oci: ":{{.Version}}"}
install: [{extract: {}}]
versions:
  - 1.4.0
  - version: 1.3.0
    excluded: "CVE-2026-1"
`

const appDependsOnExcluded = `schema: 3
name: app
artifact: {oci: ":{{.Version}}"}
install: [{extract: {}}]
deps: [lib@1.3.0]
versions: [1.0.0]
`

func TestLintExcludedCatalogIsClean(t *testing.T) {
	dir := writeCatalog(t, map[string]string{"lib": libExcludedPkg})
	found, err := Lint(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("findings: %v", found)
	}
}

func TestLintExactDepOnExcludedVersion(t *testing.T) {
	dir := writeCatalog(t, map[string]string{"lib": libExcludedPkg, "app": appDependsOnExcluded})
	for _, pkgs := range [][]string{nil, {"app"}} {
		found, err := Lint(context.Background(), dir, pkgs...)
		if err != nil {
			t.Fatal(err)
		}
		if !anyContains(found, "app: depends on lib@1.3.0, which is excluded: CVE-2026-1") {
			t.Fatalf("packages %v: findings %v", pkgs, found)
		}
	}
}

func TestLintSchema4WithoutExclusion(t *testing.T) {
	dir := writeCatalog(t, map[string]string{"lib": `schema: 4
name: lib
artifact: {oci: ":{{.Version}}"}
install: [{extract: {}}]
versions: [1.4.0]
`})
	found, err := Lint(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !anyContains(found, "lower it to 3") {
		t.Fatalf("findings: %v", found)
	}
}
