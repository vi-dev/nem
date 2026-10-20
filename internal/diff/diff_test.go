package diff

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/vi-dev/clix/console"
	"github.com/vi-dev/clix/console/consoletest"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/oci"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/ocix/ocixtest"
	"github.com/vi-dev/nem/internal/spec"
	"github.com/vi-dev/nem/internal/testx"
)

func compare(t *testing.T, base, target string, packages ...string) *Result {
	t.Helper()
	res, err := Compare(context.Background(), base, target, packages)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	return res
}

func statuses(changes []Change) string {
	parts := make([]string, 0, len(changes))
	for _, r := range changes {
		parts = append(parts, r.Name+"="+r.Status)
	}
	return strings.Join(parts, " ")
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

func renamed(fixture, name string) string {
	return strings.Replace(fixture, "name: atool", "name: "+name, 1)
}

func TestCompareUpdatedRow(t *testing.T) {
	local := writeFixture(t, map[string]string{"atool": fixtureV2})
	remote := writeFixture(t, map[string]string{"atool": fixtureV1})

	res := compare(t, local, remote)
	if len(res.Changes) != 1 {
		t.Fatalf("changes = %d, want 1", len(res.Changes))
	}
	r := res.Changes[0]
	if r.Name != "atool" || r.Status != StatusUpdated {
		t.Fatalf("c = %+v, want atool updated", r)
	}
	if r.Base != "1.1.0" || r.Target != "1.0.0" {
		t.Fatalf("base/target = %q/%q, want 1.1.0/1.0.0", r.Base, r.Target)
	}
	if len(r.Diff) != 1 || r.Diff[0] != "1.1.0" {
		t.Fatalf("diff = %v, want [1.1.0]", r.Diff)
	}
	if r.Build {
		t.Fatal("build = true, want false for a prebuilt package")
	}
}

func TestCompareNewPackage(t *testing.T) {
	local := writeFixture(t, map[string]string{
		"atool": fixtureV1,
		"ztool": sourcePkgFixture,
	})
	remote := writeFixture(t, map[string]string{"atool": fixtureV1})

	res := compare(t, local, remote)
	if len(res.Changes) != 1 {
		t.Fatalf("changes = %+v, want only the new ztool c", res.Changes)
	}
	z := res.Changes[0]
	if z.Name != "ztool" || z.Status != StatusNew || !z.Build {
		t.Fatalf("c = %+v, want new build c", z)
	}
	if z.Base != "1.1.0" || z.Target != "" {
		t.Fatalf("base/target = %q/%q, want 1.1.0/empty", z.Base, z.Target)
	}
	if len(z.Diff) != 2 || z.Diff[0] != "1.1.0" || z.Diff[1] != "1.0.0" {
		t.Fatalf("diff = %v, want all declared versions", z.Diff)
	}
}

func TestCompareManifestChangeWithoutNewVersionActsOnLatest(t *testing.T) {
	local := writeFixture(t, map[string]string{
		"atool": strings.Replace(fixtureV2, "description: Test tool", "description: Edited", 1),
	})
	remote := writeFixture(t, map[string]string{"atool": fixtureV2})

	res := compare(t, local, remote)
	if len(res.Changes) != 1 || res.Changes[0].Status != StatusUpdated {
		t.Fatalf("changes = %+v, want one updated change", res.Changes)
	}
	if d := res.Changes[0].Diff; len(d) != 1 || d[0] != "1.1.0" {
		t.Fatalf("diff = %v, want the base's latest as the fallback", d)
	}
}

func TestCompareRespelledVersionIsNotADelta(t *testing.T) {
	published := strings.Replace(fixtureV2, "version: 1.0.0", "version: v1.0.0", 1)
	local := writeFixture(t, map[string]string{"atool": fixtureV2})
	remote := writeFixture(t, map[string]string{"atool": published})

	res := compare(t, local, remote)
	if len(res.Changes) != 1 || res.Changes[0].Status != StatusUpdated {
		t.Fatalf("changes = %+v, want one updated change (digest differs)", res.Changes)
	}
	if d := res.Changes[0].Diff; len(d) != 1 || d[0] != "1.1.0" {
		t.Fatalf("diff = %v, want only the latest fallback: 1.0.0 vs v1.0.0 is not an addition", d)
	}
}

func TestCompareRejectsSideWithoutPackages(t *testing.T) {
	local := writeFixture(t, map[string]string{"atool": fixtureV1})
	empty := t.TempDir()

	for _, args := range [][2]string{{empty, local}, {local, empty}} {
		_, err := Compare(context.Background(), args[0], args[1], nil)
		if err == nil || !strings.Contains(err.Error(), "no package manifests under "+empty) {
			t.Fatalf("Compare(%q, %q): err = %v, want a no-manifests error", args[0], args[1], err)
		}
	}
}

func TestCompareRejectsDeclaredNameMismatch(t *testing.T) {
	local := writeFixture(t, map[string]string{"atool": renamed(fixtureV1, "btool")})
	remote := writeFixture(t, map[string]string{"btool": renamed(fixtureV1, "btool")})

	_, err := Compare(context.Background(), local, remote, nil)
	if err == nil || !strings.Contains(err.Error(), "declares name \"btool\"") {
		t.Fatalf("err = %v, want a declared-name mismatch error", err)
	}
}

func TestCompareReportsRemovedPackages(t *testing.T) {
	local := writeFixture(t, map[string]string{"atool": fixtureV1})
	remote := writeFixture(t, map[string]string{
		"atool": fixtureV1,
		"ztool": sourcePkgFixture,
	})

	res := compare(t, local, remote)
	if len(res.Changes) != 1 {
		t.Fatalf("changes = %+v, want only the removed ztool c", res.Changes)
	}
	z := res.Changes[0]
	if z.Name != "ztool" || z.Status != StatusRemoved {
		t.Fatalf("c = %+v, want ztool removed", z)
	}
	if z.Base != "" || z.Target != "1.1.0" {
		t.Fatalf("base/target = %q/%q, want empty/1.1.0", z.Base, z.Target)
	}
	if !z.Build {
		t.Fatal("build = false, want true (target manifest declares build)")
	}
	if z.Diff == nil || len(z.Diff) != 0 {
		t.Fatalf("diff = %v, want [] for a removed package", z.Diff)
	}
}

func TestCompareSingleFileBaseNarrowsComparison(t *testing.T) {
	local := writeFixture(t, map[string]string{"atool": fixtureV2})
	remote := writeFixture(t, map[string]string{
		"atool": fixtureV1,
		"ztool": sourcePkgFixture,
	})

	res := compare(t, filepath.Join(local, "pkgs", "atool", "pkg.yaml"), remote)
	if got := statuses(res.Changes); got != "atool=updated" || res.Unchanged != 0 {
		t.Fatalf("changes = %q, unchanged = %d; want only atool updated (no removed sweep)", got, res.Unchanged)
	}
}

func TestCompareDirAgainstDir(t *testing.T) {
	local := writeFixture(t, map[string]string{
		"atool": fixtureV2,
		"btool": renamed(fixtureV1, "btool"),
		"ctool": renamed(fixtureV1, "ctool"),
	})
	remote := writeFixture(t, map[string]string{
		"atool": fixtureV1,
		"btool": renamed(fixtureV1, "btool"),
		"ztool": sourcePkgFixture,
	})

	res := compare(t, local, remote)
	if got := statuses(res.Changes); got != "atool=updated ctool=new ztool=removed" {
		t.Fatalf("changes = %q, want sorted union statuses", got)
	}
	if res.Unchanged != 1 {
		t.Fatalf("unchanged = %d, want 1", res.Unchanged)
	}
}

func TestCompareKeepsSortedOrderAcrossManyPackages(t *testing.T) {
	localPkgs := map[string]string{}
	remotePkgs := map[string]string{}
	var want []string
	for i := range 24 {
		name := fmt.Sprintf("tool%02d", i)
		localPkgs[name] = renamed(fixtureV2, name)
		remotePkgs[name] = renamed(fixtureV1, name)
		want = append(want, name+"=updated")
	}
	local := writeFixture(t, localPkgs)
	remote := writeFixture(t, remotePkgs)

	res := compare(t, local, remote)
	if got := statuses(res.Changes); got != strings.Join(want, " ") {
		t.Fatalf("changes = %q, want every package updated in sorted order", got)
	}
}

func TestComparePackageFilterSelectsNamedPackages(t *testing.T) {
	local := writeFixture(t, map[string]string{
		"atool": fixtureV2,
		"btool": renamed(fixtureV2, "btool"),
		"ctool": renamed(fixtureV1, "ctool"),
	})
	remote := writeFixture(t, map[string]string{
		"atool": fixtureV1,
		"btool": renamed(fixtureV1, "btool"),
	})

	res := compare(t, local, remote, "btool", "ctool", "btool")
	if got := statuses(res.Changes); got != "btool=updated ctool=new" || res.Unchanged != 0 {
		t.Fatalf("changes = %q, unchanged = %d; want btool updated and ctool new only", got, res.Unchanged)
	}
}

func TestComparePackageFilterUnknownNameFails(t *testing.T) {
	local := writeFixture(t, map[string]string{"atool": fixtureV2})
	remote := writeFixture(t, map[string]string{"atool": fixtureV1})

	_, err := Compare(context.Background(), local, remote, []string{"nope"})
	var nf *catalog.PackageNotFoundError
	if !errors.As(err, &nf) || nf.Name != "nope" {
		t.Fatalf("err = %v, want PackageNotFoundError for nope", err)
	}
}

func TestCompareFailsWholeCommandOnUnparseableManifest(t *testing.T) {
	local := writeFixture(t, map[string]string{
		"atool":   fixtureV1,
		"badtool": "schema: [not valid yaml",
	})
	remote := writeFixture(t, map[string]string{"atool": fixtureV1})

	if _, err := Compare(context.Background(), local, remote, nil); err == nil {
		t.Fatal("Compare succeeded on an unparseable manifest, want failure")
	}
}

const diffFrozenBase = `schema: 3
name: gpgme
artifact: {oci: ":{{.Version}}"}
install: [{extract: {}}]
versions: [2.2.0, 2.1.2, 1.24.3, 1.24.2]
build: {source: {url: "https://x/{{.Version}}.tar"}, output: out, steps: [{run: "new"}]}
`

const diffFrozenRecipe = `artifact: {oci: ":{{.Version}}"}
install: [{extract: {}}]
build: {source: {url: "https://x/{{.Version}}.tar"}, output: out, steps: [{run: "old"}]}
`

func writeFrozenFixture(t *testing.T, pkg string, recipes map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	testx.WriteFile(t, filepath.Join(dir, "pkgs", "gpgme", "pkg.yaml"), pkg)
	for name, body := range recipes {
		testx.WriteFile(t, filepath.Join(dir, "pkgs", "gpgme", name), body)
	}
	return dir
}

func TestDiffAttributesRecipeEditsToCoveredVersions(t *testing.T) {
	target := writeFrozenFixture(t, strings.Replace(diffFrozenBase, `run: "new"`, `run: "old"`, 1), nil)
	base := writeFrozenFixture(t, diffFrozenBase, map[string]string{"recipe@1.yaml": diffFrozenRecipe})
	res := compare(t, base, target)
	if len(res.Changes) != 1 || strings.Join(res.Changes[0].Diff, ",") != "2.2.0,2.1.2" {
		t.Fatalf("freeze + edit base must attribute to 2.x only: %+v", res.Changes)
	}

	edited := writeFrozenFixture(t, strings.Replace(diffFrozenBase, `run: "new"`, `run: "old"`, 1),
		map[string]string{"recipe@1.yaml": strings.Replace(diffFrozenRecipe, `run: "old"`, `run: "older"`, 1)})
	res = compare(t, edited, target)
	if strings.Join(res.Changes[0].Diff, ",") != "1.24.3,1.24.2" {
		t.Fatalf("frozen-only edit must attribute to 1.x: %+v", res.Changes)
	}
}

func TestDiffByteIdenticalRenameIsIdentityOnly(t *testing.T) {
	old := strings.Replace(diffFrozenBase, `run: "new"`, `run: "old"`, 1)
	target := writeFrozenFixture(t, old, map[string]string{"recipe@1.yaml": diffFrozenRecipe})
	base := writeFrozenFixture(t, old, map[string]string{"recipe@1.24.yaml": diffFrozenRecipe})
	res := compare(t, base, target)
	if len(res.Changes) != 1 || res.Changes[0].Status != StatusUpdated || strings.Join(res.Changes[0].Diff, ",") != "2.2.0" {
		t.Fatalf("rename must fall back to base latest: %+v", res.Changes)
	}
}

func TestDiffNewVersionUnderBase(t *testing.T) {
	target := writeFrozenFixture(t, diffFrozenBase, map[string]string{"recipe@1.yaml": diffFrozenRecipe})
	base := writeFrozenFixture(t, strings.Replace(diffFrozenBase, "[2.2.0,", "[2.3.0, 2.2.0,", 1), map[string]string{"recipe@1.yaml": diffFrozenRecipe})
	res := compare(t, base, target)
	if strings.Join(res.Changes[0].Diff, ",") != "2.3.0" || !res.Changes[0].Build {
		t.Fatalf("added version only: %+v", res.Changes)
	}
}

func TestDiffBuildFollowsFrozenSourceRecipeUnderPrebuiltBase(t *testing.T) {
	prebuilt := `schema: 3
name: gpgme
artifact: {oci: ":{{.Version}}"}
install: [{extract: {}}]
versions: [2.2.0, 2.1.2, 1.24.3, 1.24.2]
`
	target := writeFrozenFixture(t, prebuilt, map[string]string{"recipe@1.yaml": diffFrozenRecipe})
	base := writeFrozenFixture(t, prebuilt,
		map[string]string{"recipe@1.yaml": strings.Replace(diffFrozenRecipe, `run: "old"`, `run: "older"`, 1)})
	res := compare(t, base, target)
	if len(res.Changes) != 1 || strings.Join(res.Changes[0].Diff, ",") != "1.24.3,1.24.2" || !res.Changes[0].Build {
		t.Fatalf("frozen source-built edit must build the 1.x versions: %+v", res.Changes)
	}
}

type fetchCounter struct {
	oras.ReadOnlyTarget
	mu      sync.Mutex
	fetches map[string]int
}

func (c *fetchCounter) Fetch(ctx context.Context, d ocispec.Descriptor) (io.ReadCloser, error) {
	c.mu.Lock()
	c.fetches[d.Digest.String()]++
	c.mu.Unlock()
	return c.ReadOnlyTarget.Fetch(ctx, d)
}

func (c *fetchCounter) count(digest string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.fetches[digest]
}

func manifestDigest(t *testing.T, yaml string) string {
	t.Helper()
	_, desc, err := ocix.PackageManifest(spec.Files{Pkg: []byte(yaml)})
	if err != nil {
		t.Fatal(err)
	}
	return desc.Digest.String()
}

func TestCompareAgainstOCIRefFetchesOnlyChangedPackages(t *testing.T) {
	src, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fillers, fillerYAML := fillerPackages(6)
	ocixtest.PushFakeCatalog(t, src, append([]ocixtest.FakeEntry{
		{Name: "atool", Latest: "1.0.0", YAML: []byte(fixtureV1)},
		{Name: "btool", Latest: "1.0.0", YAML: []byte(renamed(fixtureV1, "btool"))},
		{Name: "ctool", Latest: "1.0.0", YAML: []byte(renamed(fixtureV1, "ctool"))},
	}, fillers...), "2")
	counter := &fetchCounter{ReadOnlyTarget: src, fetches: map[string]int{}}
	defer catalog.SetRemoteTarget(func(string) (oras.ReadOnlyTarget, string, error) { return counter, "v2", nil })()

	basePkgs := map[string]string{"atool": fixtureV1, "btool": renamed(fixtureV2, "btool")}
	maps.Copy(basePkgs, fillerYAML)
	base := writeFixture(t, basePkgs)
	res := compare(t, base, "example.com/cat:v2")

	if got := statuses(res.Changes); got != "btool=updated ctool=removed" || res.Unchanged != 7 {
		t.Fatalf("changes = %q, unchanged %d", got, res.Unchanged)
	}
	if n := counter.count(manifestDigest(t, fixtureV1)); n != 0 {
		t.Fatalf("unchanged atool was fetched %d times; its digest in the index already settles it", n)
	}
	if n := counter.count(manifestDigest(t, renamed(fixtureV1, "btool"))); n == 0 {
		t.Fatal("updated btool must be fetched for its versions")
	}
	if n := counter.count(manifestDigest(t, renamed(fixtureV1, "ctool"))); n == 0 {
		t.Fatal("removed ctool must be fetched for its versions and build flag")
	}
	if res.Changes[0].Target != "1.0.0" || res.Changes[0].Base != "1.1.0" {
		t.Fatalf("btool row = %+v", res.Changes[0])
	}
}

func pushLegacyEncodedCatalog(t *testing.T, store oras.Target, name, yaml string) {
	t.Helper()
	ctx := context.Background()
	layer := content.NewDescriptorFromBytes(ocix.MediaTypePkgV2, []byte(yaml))
	layer.Annotations = map[string]string{ocix.AnnotationTitle: spec.PkgFileName}
	if err := ocix.PushBlobIfAbsent(ctx, store, layer, bytes.NewReader([]byte(yaml))); err != nil {
		t.Fatal(err)
	}
	if err := ocix.PushEmptyConfig(ctx, store); err != nil {
		t.Fatal(err)
	}
	manBytes, err := json.Marshal(ocispec.Manifest{
		SchemaVersion: ocix.OCISchemaVersion,
		MediaType:     ocispec.MediaTypeImageManifest,
		ArtifactType:  ocix.MediaTypePkgV2,
		Config:        ocispec.DescriptorEmptyJSON,
		Layers:        []ocispec.Descriptor{layer},
	})
	if err != nil {
		t.Fatal(err)
	}
	manDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, manBytes)
	if err := ocix.PushBlobIfAbsent(ctx, store, manDesc, bytes.NewReader(manBytes)); err != nil {
		t.Fatal(err)
	}
	if manDesc.Digest.String() == manifestDigest(t, yaml) {
		t.Fatal("fixture must differ from the current manifest encoding")
	}
	manDesc.Annotations = map[string]string{ocix.AnnotationTitle: name, ocix.AnnotationVersion: "1.0.0"}
	idxBytes, err := json.Marshal(ocispec.Index{
		SchemaVersion: ocix.OCISchemaVersion,
		MediaType:     ocispec.MediaTypeImageIndex,
		Manifests:     []ocispec.Descriptor{manDesc},
		Annotations:   map[string]string{ocix.AnnotationSchemaVersion: ocix.CatalogSchemaV2},
	})
	if err != nil {
		t.Fatal(err)
	}
	idxDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, idxBytes)
	if err := ocix.PushBlobAndTag(ctx, store, idxBytes, idxDesc, []string{"v2"}); err != nil {
		t.Fatal(err)
	}
}

func TestCompareTreatsIndexDigestAsShortcutOnly(t *testing.T) {
	src, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pushLegacyEncodedCatalog(t, src, "atool", fixtureV1)
	defer catalog.SetRemoteTarget(func(string) (oras.ReadOnlyTarget, string, error) { return src, "v2", nil })()

	base := writeFixture(t, map[string]string{"atool": fixtureV1})
	res := compare(t, base, "example.com/cat:v2")
	if len(res.Changes) != 0 || res.Unchanged != 1 {
		t.Fatalf("changes = %q, unchanged %d; identical content under another manifest encoding must be unchanged", statuses(res.Changes), res.Unchanged)
	}
}

func TestCompareNarratesProgressAsOneTask(t *testing.T) {
	src, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ocixtest.PushFakeCatalog(t, src, []ocixtest.FakeEntry{
		{Name: "atool", Latest: "1.0.0", YAML: []byte(fixtureV1)},
		{Name: "btool", Latest: "1.0.0", YAML: []byte(renamed(fixtureV1, "btool"))},
	}, "2")
	defer catalog.SetRemoteTarget(func(string) (oras.ReadOnlyTarget, string, error) { return src, "v2", nil })()
	base := writeFixture(t, map[string]string{"atool": fixtureV1, "btool": renamed(fixtureV2, "btool")})

	ctx, rep := consoletest.Context(context.Background())
	if _, err := Compare(ctx, base, "example.com/cat:v2", nil); err != nil {
		t.Fatalf("Compare: %v", err)
	}
	task := rep.TaskFor("Comparing 2 packages")
	if task == nil {
		var labels []string
		for _, tk := range rep.Tasks() {
			labels = append(labels, tk.Label)
		}
		t.Fatalf("tasks = %v, want the compare task", labels)
	}
	if done, _, outcome := task.Snapshot(); !done || outcome != "Compared 2 packages" {
		t.Fatalf("outcome = %q (done %v)", outcome, done)
	}
	calls := task.ProgressCalls()
	if len(calls) == 0 || calls[len(calls)-1].Done != 2 || calls[len(calls)-1].Total != 2 || calls[len(calls)-1].Unit != console.Items {
		t.Fatalf("progress = %+v, want it to reach 2/2 items", calls)
	}
	if got := task.Phases(); !slices.Contains(got, "comparing") {
		t.Fatalf("segments = %v, want \"comparing\" set before progress so the live line shows the count", got)
	}
}

func taskLabels(rep *consoletest.Narrator) []string {
	var labels []string
	for _, tk := range rep.Tasks() {
		labels = append(labels, tk.Label)
	}
	return labels
}

func TestComparePullsASideWhenMostOfItMustBeRead(t *testing.T) {
	src, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ocixtest.PushFakeCatalog(t, src, []ocixtest.FakeEntry{
		{Name: "atool", Latest: "1.0.0", YAML: []byte(fixtureV1)},
		{Name: "btool", Latest: "1.0.0", YAML: []byte(renamed(fixtureV1, "btool"))},
	}, "2")
	defer catalog.SetRemoteTarget(func(string) (oras.ReadOnlyTarget, string, error) { return src, "v2", nil })()
	base := writeFixture(t, map[string]string{"atool": fixtureV2, "btool": renamed(fixtureV2, "btool")})

	ctx, rep := consoletest.Context(context.Background())
	res, err := Compare(ctx, base, "example.com/cat:v2", nil)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if got := statuses(res.Changes); got != "atool=updated btool=updated" {
		t.Fatalf("changes = %q", got)
	}
	pull := rep.TaskFor("Pulling catalog example.com/cat:v2")
	if pull == nil {
		t.Fatalf("tasks = %v, want the whole target pulled since every package must be read", taskLabels(rep))
	}
	if done, _, outcome := pull.Snapshot(); !done || outcome != "Pulled catalog example.com/cat:v2" {
		t.Fatalf("pull outcome = %q (done %v)", outcome, done)
	}
}

func TestCompareReadsLazilyWhenFewPackagesChanged(t *testing.T) {
	src, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var entries []ocixtest.FakeEntry
	pkgs := map[string]string{}
	for _, name := range []string{"atool", "btool", "ctool", "dtool", "etool"} {
		yaml := renamed(fixtureV1, name)
		entries = append(entries, ocixtest.FakeEntry{Name: name, Latest: "1.0.0", YAML: []byte(yaml)})
		pkgs[name] = yaml
	}
	ocixtest.PushFakeCatalog(t, src, entries, "2")
	defer catalog.SetRemoteTarget(func(string) (oras.ReadOnlyTarget, string, error) { return src, "v2", nil })()
	pkgs["atool"] = fixtureV2
	base := writeFixture(t, pkgs)

	ctx, rep := consoletest.Context(context.Background())
	res, err := Compare(ctx, base, "example.com/cat:v2", nil)
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if got := statuses(res.Changes); got != "atool=updated" || res.Unchanged != 4 {
		t.Fatalf("changes = %q, unchanged %d", got, res.Unchanged)
	}
	if rep.TaskFor("Pulling catalog example.com/cat:v2") != nil {
		t.Fatalf("tasks = %v; one changed package out of five must be read lazily, not pulled", taskLabels(rep))
	}
}

func fillerPackages(n int) ([]ocixtest.FakeEntry, map[string]string) {
	entries := make([]ocixtest.FakeEntry, 0, n)
	yamls := make(map[string]string, n)
	for i := range n {
		name := fmt.Sprintf("filler%d", i)
		yaml := renamed(fixtureV1, name)
		entries = append(entries, ocixtest.FakeEntry{Name: name, Latest: "1.0.0", YAML: []byte(yaml)})
		yamls[name] = yaml
	}
	return entries, yamls
}
