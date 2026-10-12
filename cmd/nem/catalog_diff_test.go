package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"strings"
	"sync"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/oci"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/diff"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/ocix/ocixtest"
	"github.com/vi-dev/nem/internal/spec"
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

const diffWiringFixtureV2 = `schema: 2
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

func writeDiffTarget(t *testing.T) string {
	t.Helper()
	return writeLintFixture(t, map[string]string{"atool": diffWiringFixture})
}

func TestCatalogDiffTableListsOnlyChanged(t *testing.T) {
	target := writeLintFixture(t, map[string]string{
		"atool": diffWiringFixture,
		"btool": strings.Replace(diffWiringFixture, "name: atool", "name: btool", 1),
		"ztool": strings.Replace(diffWiringFixture, "name: atool", "name: ztool", 1),
	})
	dir := writeLintFixture(t, map[string]string{
		"atool": diffWiringFixtureV2,
		"btool": strings.Replace(diffWiringFixture, "name: atool", "name: btool", 1),
	})

	out, errOut, err := runNem(t, t.TempDir(), "catalog", "diff", dir, target)
	if err != nil {
		t.Fatalf("diff: %v\nstderr: %s", err, errOut)
	}
	for _, want := range []string{"PACKAGE", "STATUS", "BASE", "TARGET", "DIFF", "atool", "updated", "1.1.0", "ztool", "removed", "-"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout = %q, want %q", out, want)
		}
	}
	if strings.Contains(out, "btool") {
		t.Fatalf("stdout = %q, unchanged btool must not be listed", out)
	}
	if !strings.Contains(errOut, "2 changed (0 new, 1 updated, 1 removed), 1 unchanged") {
		t.Fatalf("stderr = %q, want the counts summary", errOut)
	}
	if strings.Contains(errOut, dir) || strings.Contains(errOut, target) {
		t.Fatalf("stderr = %q, the summary must not repeat the catalog args", errOut)
	}
}

func TestCatalogDiffOutputJSONEmitsChanges(t *testing.T) {
	target := writeDiffTarget(t)
	dir := writeLintFixture(t, map[string]string{"atool": diffWiringFixtureV2})

	out, errOut, err := runNem(t, t.TempDir(), "catalog", "diff", dir, target, "--output", "json")
	if err != nil {
		t.Fatalf("diff: %v\nstderr: %s", err, errOut)
	}
	var changes []diff.Change
	if err := json.Unmarshal([]byte(out), &changes); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out)
	}
	if len(changes) != 1 || changes[0].Name != "atool" || changes[0].Status != "updated" {
		t.Fatalf("changes = %+v, want one updated atool c", changes)
	}
	for _, key := range []string{"\"name\"", "\"status\"", "\"build\"", "\"base\"", "\"target\"", "\"diff\""} {
		if !strings.Contains(out, key) {
			t.Fatalf("stdout = %s, want key %s", out, key)
		}
	}
}

func TestCatalogDiffOutputJSONOmitsUnchanged(t *testing.T) {
	target := writeDiffTarget(t)
	dir := writeLintFixture(t, map[string]string{"atool": diffWiringFixture})

	out, errOut, err := runNem(t, t.TempDir(), "catalog", "diff", dir, target, "--output", "json")
	if err != nil {
		t.Fatalf("diff: %v\nstderr: %s", err, errOut)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Fatalf("stdout = %q, want an empty JSON array", out)
	}
}

func TestCatalogDiffRejectsUnknownOutput(t *testing.T) {
	target := writeDiffTarget(t)
	dir := writeLintFixture(t, map[string]string{"atool": diffWiringFixture})

	_, errOut, err := runNem(t, t.TempDir(), "catalog", "diff", dir, target, "--output", "yaml")
	if err == nil || !strings.Contains(errOut, "yaml") {
		t.Fatalf("err = %v, stderr = %q; want an error naming the bad format", err, errOut)
	}
}

func TestCatalogDiffRequiresBothCatalogs(t *testing.T) {
	target := writeDiffTarget(t)

	_, _, err := runNem(t, t.TempDir(), "catalog", "diff", target)
	if err == nil || !strings.Contains(err.Error(), "accepts 2 arg(s)") {
		t.Fatalf("err = %v, want cobra's exact-args error", err)
	}
}

func TestCatalogDiffPackageFlagScopes(t *testing.T) {
	target := writeDiffTarget(t)
	dir := writeLintFixture(t, map[string]string{
		"atool": diffWiringFixtureV2,
		"btool": strings.Replace(diffWiringFixture, "name: atool", "name: btool", 1),
	})

	_, errOut, err := runNem(t, t.TempDir(), "catalog", "diff", dir, target, "--package", "btool")
	if err != nil {
		t.Fatalf("diff: %v\nstderr: %s", err, errOut)
	}
	if !strings.Contains(errOut, "1 changed (1 new, 0 updated, 0 removed), 0 unchanged") {
		t.Fatalf("stderr = %q, want only btool compared", errOut)
	}
}

func TestCatalogDiffLintsBaseBeforeComparing(t *testing.T) {
	target := writeDiffTarget(t)
	dir := writeLintFixture(t, map[string]string{
		"atool": strings.Replace(diffWiringFixtureV2, "name: atool", "name: btool", 1),
	})

	out, errOut, err := runNem(t, t.TempDir(), "catalog", "diff", dir, target)
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != 1 {
		t.Fatalf("err = %v, want exit code 1 from lint findings", err)
	}
	if !strings.Contains(errOut, `name "btool" does not match its directory "atool"`) {
		t.Fatalf("stderr = %q, want the lint finding", errOut)
	}
	if strings.Contains(errOut, "changed (") || out != "" {
		t.Fatalf("stdout = %q, stderr = %q; want no diff output after lint failure", out, errOut)
	}
}

func TestCatalogDiffCompletesOutputValues(t *testing.T) {
	out := runNemComplete(t, t.TempDir(), "catalog", "diff", "ghcr.io/org/cat:v2", "--output", "")
	if !strings.Contains(out, "text") || !strings.Contains(out, "json") {
		t.Fatalf("completion = %q, want text and json", out)
	}
}

func TestCatalogDiffCompletesPackagesFromBaseCatalog(t *testing.T) {
	dir := writeLintFixture(t, map[string]string{
		"atool": diffWiringFixture,
		"btool": strings.Replace(diffWiringFixture, "name: atool", "name: btool", 1),
	})
	out := runNemComplete(t, t.TempDir(), "catalog", "diff", dir, "ghcr.io/org/cat:v2", "--package", "b")
	if !strings.Contains(out, "btool") || strings.Contains(out, "atool") {
		t.Fatalf("completion = %q, want btool only", out)
	}
}

func TestCatalogDiffAgainstOCIRefReadsTheIndex(t *testing.T) {
	src, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	entries := []ocixtest.FakeEntry{
		{Name: "atool", Latest: "1.0.0", YAML: []byte(diffWiringFixture)},
		{Name: "ztool", Latest: "1.0.0", YAML: []byte(strings.Replace(diffWiringFixture, "name: atool", "name: ztool", 1))},
	}
	fillers, fillerYAML := diffFillers(7)
	ocixtest.PushFakeCatalog(t, src, append(entries, fillers...), "2")
	defer catalog.SetRemoteTarget(func(string) (oras.ReadOnlyTarget, string, error) { return src, "v2", nil })()
	dirPkgs := map[string]string{"atool": diffWiringFixtureV2}
	maps.Copy(dirPkgs, fillerYAML)
	dir := writeLintFixture(t, dirPkgs)

	out, errOut, err := runNem(t, t.TempDir(), "catalog", "diff", dir, "example.com/cat:v2")
	if err != nil {
		t.Fatalf("diff: %v\nstderr: %s", err, errOut)
	}
	for _, want := range []string{"atool", "updated", "ztool", "removed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout = %q, want %q", out, want)
		}
	}
	if !strings.Contains(errOut, "Read catalog example.com/cat:v2") || strings.Contains(errOut, "Pulled catalog") {
		t.Fatalf("stderr = %q, want the index read narrated and no full pull", errOut)
	}
}

type fetchCountingTarget struct {
	oras.ReadOnlyTarget
	mu      sync.Mutex
	fetches map[string]int
}

func (c *fetchCountingTarget) Fetch(ctx context.Context, d ocispec.Descriptor) (io.ReadCloser, error) {
	c.mu.Lock()
	c.fetches[d.Digest.String()]++
	c.mu.Unlock()
	return c.ReadOnlyTarget.Fetch(ctx, d)
}

func TestCatalogDiffBetweenOCIRefsPullsNeither(t *testing.T) {
	ztool := strings.Replace(diffWiringFixture, "name: atool", "name: ztool", 1)
	stores := map[string]*fetchCountingTarget{}
	fillers, _ := diffFillers(3)
	for ref, entries := range map[string][]ocixtest.FakeEntry{
		"example.com/a:v2": {{Name: "atool", Latest: "1.1.0", YAML: []byte(diffWiringFixtureV2)}, {Name: "ztool", Latest: "1.0.0", YAML: []byte(ztool)}},
		"example.com/b:v2": {{Name: "atool", Latest: "1.0.0", YAML: []byte(diffWiringFixture)}, {Name: "ztool", Latest: "1.0.0", YAML: []byte(ztool)}},
	} {
		src, err := oci.New(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		ocixtest.PushFakeCatalog(t, src, append(entries, fillers...), "2")
		stores[ref] = &fetchCountingTarget{ReadOnlyTarget: src, fetches: map[string]int{}}
	}
	defer catalog.SetRemoteTarget(func(ref string) (oras.ReadOnlyTarget, string, error) { return stores[ref], "v2", nil })()

	out, errOut, err := runNem(t, t.TempDir(), "catalog", "diff", "example.com/a:v2", "example.com/b:v2")
	if err != nil {
		t.Fatalf("diff: %v\nstderr: %s", err, errOut)
	}
	if !strings.Contains(out, "atool") || !strings.Contains(out, "updated") || strings.Contains(out, "ztool") {
		t.Fatalf("stdout = %q, want atool updated and ztool absent", out)
	}
	if strings.Contains(errOut, "Pulled catalog") || !strings.Contains(errOut, "Read catalog example.com/a:v2") {
		t.Fatalf("stderr = %q, want both sides read index-only", errOut)
	}
	_, ztoolDesc, err := ocix.PackageManifest(spec.Files{Pkg: []byte(ztool)})
	if err != nil {
		t.Fatal(err)
	}
	for ref, store := range stores {
		if n := store.fetches[ztoolDesc.Digest.String()]; n != 0 {
			t.Fatalf("%s fetched the unchanged ztool manifest %d times", ref, n)
		}
	}
}

func diffFillers(n int) ([]ocixtest.FakeEntry, map[string]string) {
	entries := make([]ocixtest.FakeEntry, 0, n)
	yamls := make(map[string]string, n)
	for i := range n {
		name := fmt.Sprintf("filler%d", i)
		yaml := strings.Replace(diffWiringFixture, "name: atool", "name: "+name, 1)
		entries = append(entries, ocixtest.FakeEntry{Name: name, Latest: "1.0.0", YAML: []byte(yaml)})
		yamls[name] = yaml
	}
	return entries, yamls
}
