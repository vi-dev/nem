package main

import (
	"encoding/json"
	"strings"
	"testing"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/oci"

	"github.com/vi-dev/nem/internal/diff"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/ocix/ocixtest"
)

const diffWiringFixture = `schema: 2
name: atool
description: Test tool
artifact:
  url: "https://example.com/atool-{{.Version}}.tar.gz"
install:
  - extract: {strip: 1}
bins: ["bin"]
versions:
  - version: 1.0.0
    sha256:
      darwin/arm64: "aaa"
      darwin/amd64: "aaa"
      linux/arm64: "aaa"
      linux/amd64: "aaa"
`

func stubDiffCatalog(t *testing.T) {
	t.Helper()
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ocixtest.PushFakeCatalog(t, store, []ocixtest.FakeEntry{
		{Name: "atool", Description: "Test tool", Latest: "1.0.0", YAML: []byte(diffWiringFixture)},
	}, ocix.SchemaVersion)
	t.Cleanup(diff.SetCatalogOpener(func(ref string) (oras.ReadOnlyTarget, string, error) { return store, "v2", nil }))
}

func TestCatalogDiffFlagWiring(t *testing.T) {
	stubDiffCatalog(t)
	dir := writeLintFixture(t, map[string]string{"atool": diffWiringFixture})

	out, errOut, err := runNem(t, t.TempDir(), "catalog", "diff", "ghcr.io/org/cat:v2", dir, "--json")
	if err != nil {
		t.Fatalf("diff: %v\nstderr: %s", err, errOut)
	}
	var rows []diff.Row
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out)
	}
	if len(rows) != 1 || rows[0].Name != "atool" || rows[0].Status != "unchanged" {
		t.Fatalf("rows = %+v, want one unchanged atool row", rows)
	}
}

func TestCatalogDiffDefaultsToCurrentDir(t *testing.T) {
	stubDiffCatalog(t)
	dir := writeLintFixture(t, map[string]string{"atool": diffWiringFixture})
	t.Chdir(dir)

	_, errOut, err := runNem(t, t.TempDir(), "catalog", "diff", "ghcr.io/org/cat:v2")
	if err != nil {
		t.Fatalf("diff: %v\nstderr: %s", err, errOut)
	}
	if !strings.Contains(errOut, "Compared 1 packages against ghcr.io/org/cat:v2: 0 changed") {
		t.Fatalf("stderr = %q, want summary for the current directory", errOut)
	}
}
