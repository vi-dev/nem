package diff

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/oci"

	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/ocix/ocixtest"
	"github.com/vi-dev/nem/internal/report"
	"github.com/vi-dev/nem/internal/testx"
)

func runDiff(t *testing.T, opts Options, ref, target string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errb bytes.Buffer
	console := report.New(&out, &errb, report.Options{Color: report.ColorNever})
	err = Run(context.Background(), ref, target, opts, console)
	return out.String(), errb.String(), err
}

func writeFixture(t *testing.T, pkgs map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, y := range pkgs {
		testx.WriteFile(t, filepath.Join(dir, "pkgs", name, "pkg.yaml"), y)
	}
	return dir
}

const fixtureV1 = `schema: 2
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

const fixtureV2 = `schema: 2
name: atool
description: Test tool
artifact:
  url: "https://example.com/atool-{{.Version}}.tar.gz"
install:
  - extract: {strip: 1}
bins: ["bin"]
versions:
  - version: 1.1.0
    sha256:
      darwin/arm64: "bbb"
      darwin/amd64: "bbb"
      linux/arm64: "bbb"
      linux/amd64: "bbb"
  - version: 1.0.0
    sha256:
      darwin/arm64: "aaa"
      darwin/amd64: "aaa"
      linux/arm64: "aaa"
      linux/amd64: "aaa"
`

const sourcePkgFixture = `
schema: 2
name: ztool
description: Test tool built from source
platforms: [darwin/arm64, linux/arm64, linux/amd64]
artifact:
  oci: ":{{.Version}}"
install:
  - extract: {strip: 0}
versions:
  - version: 1.1.0
    sourceSha256: "aaa"
  - version: 1.0.0
    sourceSha256: "bbb"
build:
  source:
    url: "https://example.com/src-{{.Version}}.tar.gz"
  output: out
  steps:
    - run: make
`

func withFakeCatalog(t *testing.T, store oras.ReadOnlyTarget) {
	t.Helper()
	t.Cleanup(SetCatalogOpener(func(ref string) (oras.ReadOnlyTarget, string, error) { return store, "v2", nil }))
}

func newCatalogStore(t *testing.T, entries []ocixtest.FakeEntry) oras.Target {
	t.Helper()
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ocixtest.PushFakeCatalog(t, store, entries, ocix.SchemaVersion)
	return store
}

func decodeRows(t *testing.T, out string) []Row {
	t.Helper()
	var rows []Row
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("unmarshal diff JSON: %v\noutput: %s", err, out)
	}
	return rows
}

func TestDiffJSONUnchangedAndUpdated(t *testing.T) {
	dir := writeFixture(t, map[string]string{"atool": fixtureV2})
	store := newCatalogStore(t, []ocixtest.FakeEntry{
		{Name: "atool", Description: "Test tool", Latest: "1.0.0", YAML: []byte(fixtureV1)},
	})
	withFakeCatalog(t, store)

	out, errOut, err := runDiff(t, Options{JSON: true}, "ghcr.io/org/cat:v2", dir)
	if err != nil {
		t.Fatalf("diff: %v\nstderr: %s", err, errOut)
	}
	rows := decodeRows(t, out)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	r := rows[0]
	if r.Name != "atool" || r.Status != "updated" {
		t.Fatalf("row = %+v, want atool updated", r)
	}
	if r.Published != "1.0.0" || r.Local != "1.1.0" {
		t.Fatalf("published/local = %q/%q, want 1.0.0/1.1.0", r.Published, r.Local)
	}
	if len(r.VersionsAdded) != 1 || r.VersionsAdded[0] != "1.1.0" {
		t.Fatalf("versionsAdded = %v, want [1.1.0]", r.VersionsAdded)
	}
	if r.VersionsRemoved == nil || len(r.VersionsRemoved) != 0 {
		t.Fatalf("versionsRemoved = %v, want []", r.VersionsRemoved)
	}
	if r.Source {
		t.Fatal("source = true, want false for a prebuilt package")
	}
}

func TestDiffJSONIncludesUnchangedRows(t *testing.T) {
	dir := writeFixture(t, map[string]string{"atool": fixtureV1})
	store := newCatalogStore(t, []ocixtest.FakeEntry{
		{Name: "atool", Description: "Test tool", Latest: "1.0.0", YAML: []byte(fixtureV1)},
	})
	withFakeCatalog(t, store)

	out, errOut, err := runDiff(t, Options{JSON: true}, "ghcr.io/org/cat:v2", dir)
	if err != nil {
		t.Fatalf("diff: %v\nstderr: %s", err, errOut)
	}
	rows := decodeRows(t, out)
	if len(rows) != 1 || rows[0].Status != "unchanged" {
		t.Fatalf("rows = %+v, want one unchanged row", rows)
	}
	if rows[0].Published != "1.0.0" || rows[0].Local != "1.0.0" {
		t.Fatalf("published/local = %q/%q, want 1.0.0/1.0.0", rows[0].Published, rows[0].Local)
	}
}

func TestDiffJSONNewPackage(t *testing.T) {
	dir := writeFixture(t, map[string]string{
		"atool": fixtureV1,
		"ztool": sourcePkgFixture,
	})
	store := newCatalogStore(t, []ocixtest.FakeEntry{
		{Name: "atool", Description: "Test tool", Latest: "1.0.0", YAML: []byte(fixtureV1)},
	})
	withFakeCatalog(t, store)

	out, errOut, err := runDiff(t, Options{JSON: true}, "ghcr.io/org/cat:v2", dir)
	if err != nil {
		t.Fatalf("diff: %v\nstderr: %s", err, errOut)
	}
	rows := decodeRows(t, out)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (sorted: atool, ztool)", len(rows))
	}
	if rows[0].Name != "atool" || rows[1].Name != "ztool" {
		t.Fatalf("order = %s, %s; want atool, ztool", rows[0].Name, rows[1].Name)
	}
	z := rows[1]
	if z.Status != "new" || !z.Source {
		t.Fatalf("row = %+v, want new source row", z)
	}
	if z.Published != "" {
		t.Fatalf("published = %q, want empty for new", z.Published)
	}
	if got := strings.Count(out, "\"published\""); got != 1 {
		t.Fatalf("published keys = %d, want 1 (the key must be omitted for the new row): %s", got, out)
	}
	if len(z.VersionsAdded) != 2 || z.VersionsAdded[0] != "1.1.0" || z.VersionsAdded[1] != "1.0.0" {
		t.Fatalf("versionsAdded = %v, want all declared versions", z.VersionsAdded)
	}
}

func TestDiffRespelledVersionIsNotADelta(t *testing.T) {
	published := strings.Replace(fixtureV1, "version: 1.0.0", "version: v1.0.0", 1)
	dir := writeFixture(t, map[string]string{"atool": fixtureV1})
	store := newCatalogStore(t, []ocixtest.FakeEntry{
		{Name: "atool", Description: "Test tool", Latest: "v1.0.0", YAML: []byte(published)},
	})
	withFakeCatalog(t, store)

	out, errOut, err := runDiff(t, Options{JSON: true}, "ghcr.io/org/cat:v2", dir)
	if err != nil {
		t.Fatalf("diff: %v\nstderr: %s", err, errOut)
	}
	rows := decodeRows(t, out)
	if len(rows) != 1 || rows[0].Status != "updated" {
		t.Fatalf("rows = %+v, want one updated row (digest differs)", rows)
	}
	if len(rows[0].VersionsAdded) != 0 || len(rows[0].VersionsRemoved) != 0 {
		t.Fatalf("deltas = %v/%v, want both empty for a respelled version",
			rows[0].VersionsAdded, rows[0].VersionsRemoved)
	}
}

func TestDiffReportsRemovedPackages(t *testing.T) {
	dir := writeFixture(t, map[string]string{"atool": fixtureV1})
	store := newCatalogStore(t, []ocixtest.FakeEntry{
		{Name: "atool", Description: "Test tool", Latest: "1.0.0", YAML: []byte(fixtureV1)},
		{Name: "ztool", Description: "Gone", Latest: "1.1.0", YAML: []byte(sourcePkgFixture)},
	})
	withFakeCatalog(t, store)

	out, errOut, err := runDiff(t, Options{JSON: true}, "ghcr.io/org/cat:v2", dir)
	if err != nil {
		t.Fatalf("diff: %v\nstderr: %s", err, errOut)
	}
	rows := decodeRows(t, out)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	z := rows[1]
	if z.Name != "ztool" || z.Status != "removed" {
		t.Fatalf("row = %+v, want ztool removed", z)
	}
	if z.Local != "" || z.Path != "" {
		t.Fatalf("local/path = %q/%q, want empty for removed", z.Local, z.Path)
	}
	if z.Published != "1.1.0" {
		t.Fatalf("published = %q, want 1.1.0", z.Published)
	}
	if !z.Source {
		t.Fatal("source = false, want true (published manifest declares build)")
	}
	if len(z.VersionsRemoved) != 2 {
		t.Fatalf("versionsRemoved = %v, want both published versions", z.VersionsRemoved)
	}
	if len(z.VersionsAdded) != 0 {
		t.Fatalf("versionsAdded = %v, want []", z.VersionsAdded)
	}
}

func TestDiffSingleFileSkipsRemovedSweep(t *testing.T) {
	dir := writeFixture(t, map[string]string{"atool": fixtureV2})
	store := newCatalogStore(t, []ocixtest.FakeEntry{
		{Name: "atool", Description: "Test tool", Latest: "1.0.0", YAML: []byte(fixtureV1)},
		{Name: "ztool", Description: "Other", Latest: "1.1.0", YAML: []byte(sourcePkgFixture)},
	})
	withFakeCatalog(t, store)

	out, errOut, err := runDiff(t, Options{JSON: true}, "ghcr.io/org/cat:v2",
		filepath.Join(dir, "pkgs", "atool", "pkg.yaml"))
	if err != nil {
		t.Fatalf("diff: %v\nstderr: %s", err, errOut)
	}
	rows := decodeRows(t, out)
	if len(rows) != 1 || rows[0].Name != "atool" || rows[0].Status != "updated" {
		t.Fatalf("rows = %+v, want only atool updated (no removed sweep)", rows)
	}
}

func TestDiffHumanTableListsOnlyChanged(t *testing.T) {
	dir := writeFixture(t, map[string]string{
		"atool": fixtureV2,
		"btool": strings.Replace(fixtureV1, "name: atool", "name: btool", 1),
	})
	store := newCatalogStore(t, []ocixtest.FakeEntry{
		{Name: "atool", Description: "Test tool", Latest: "1.0.0", YAML: []byte(fixtureV1)},
		{Name: "btool", Description: "Test tool", Latest: "1.0.0",
			YAML: []byte(strings.Replace(fixtureV1, "name: atool", "name: btool", 1))},
		{Name: "ztool", Description: "Gone", Latest: "1.1.0", YAML: []byte(sourcePkgFixture)},
	})
	withFakeCatalog(t, store)

	out, errOut, err := runDiff(t, Options{}, "ghcr.io/org/cat:v2", dir)
	if err != nil {
		t.Fatalf("diff: %v\nstderr: %s", err, errOut)
	}
	if !strings.Contains(out, "atool") || !strings.Contains(out, "updated") {
		t.Fatalf("stdout = %q, want the updated atool row", out)
	}
	if !strings.Contains(out, "ztool") || !strings.Contains(out, "removed") {
		t.Fatalf("stdout = %q, want the removed ztool row", out)
	}
	if strings.Contains(out, "btool") {
		t.Fatalf("stdout = %q, unchanged btool must not be listed", out)
	}
	if !strings.Contains(out, "-") {
		t.Fatalf("stdout = %q, want '-' for the removed row's absent local side", out)
	}
}

func TestDiffFailsWholeCommandOnUnparseableManifest(t *testing.T) {
	dir := writeFixture(t, map[string]string{
		"atool":   fixtureV1,
		"badtool": "schema: [not valid yaml",
	})
	store := newCatalogStore(t, []ocixtest.FakeEntry{
		{Name: "atool", Description: "Test tool", Latest: "1.0.0", YAML: []byte(fixtureV1)},
	})
	withFakeCatalog(t, store)

	out, _, err := runDiff(t, Options{JSON: true}, "ghcr.io/org/cat:v2", dir)
	if err == nil {
		t.Fatal("diff succeeded on an unparseable manifest, want whole-command failure")
	}
	if strings.Contains(out, "\"status\"") {
		t.Fatalf("stdout = %q, want no JSON rows on failure", out)
	}
}
