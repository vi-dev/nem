package build_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"

	"github.com/vi-dev/nem/internal/build"
	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/spec"
)

const selectPrebuiltPkg = `
schema: 2
name: pretool
artifact:
  url: "https://example.com/pretool-{{.Version}}-{{.OS}}-{{.Arch}}.tar.gz"
install:
  - extract: {}
versions:
  - version: v1.0.0
    sha256:
      darwin/arm64: "aaa"
      darwin/amd64: "bbb"
      linux/arm64: "ccc"
      linux/amd64: "ddd"
`

func writeSelectFixture(t *testing.T, root, name, yaml string) {
	t.Helper()
	dir := filepath.Join(root, "pkgs", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
}

func platformsEqual(a, b []spec.Platform) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

const selectTargetPkg = `
schema: 2
name: tgttool
artifact:
  oci: ":{{.Version}}"
install:
  - extract: {}
build:
  source:
    url: "https://example.com/tgttool-{{.Version}}-src.tar.gz"
  output: tgttool
  steps:
    - run: "make build"
versions:
  - version: v2.0.0
  - version: v1.0.0
`

const selectAppPkg = `
schema: 2
name: apptool
artifact:
  oci: ":{{.Version}}"
install:
  - extract: {}
build:
  source:
    url: "https://example.com/apptool-{{.Version}}-src.tar.gz"
  output: apptool
  deps:
    - name: tgttool
    - name: pretool
    - name: nobuildtool
  steps:
    - run: "make build"
versions:
  - version: v1.0.0
`

const selectNoBuildPkg = `
schema: 2
name: nobuildtool
artifact:
  oci: ":{{.Version}}"
install:
  - extract: {}
versions:
  - version: v1.0.0
`

func removePlatform(all []spec.Platform, drop spec.Platform) []spec.Platform {
	var out []spec.Platform
	for _, p := range all {
		if p != drop {
			out = append(out, p)
		}
	}
	return out
}

func parseSelectPkg(t *testing.T, yaml string) *spec.Package {
	t.Helper()
	pkg, err := spec.Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return pkg
}

func TestSelectMissingDirOverlay(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeSelectFixture(t, root, "tgttool", selectTargetPkg)

	tgt, err := build.OpenTarget(ctx, root)
	if err != nil {
		t.Fatalf("OpenTarget: %v", err)
	}

	sels, stats, err := build.SelectMissing(ctx, tgt)
	if err != nil {
		t.Fatalf("SelectMissing: %v", err)
	}
	if stats.Checked != 1 || stats.Skipped != 0 {
		t.Fatalf("stats = %+v, want Checked=1 Skipped=0", stats)
	}
	if len(sels) != 2 {
		t.Fatalf("sels = %+v, want both versions selected against an empty overlay", sels)
	}
	all := spec.SupportedPlatforms
	for _, s := range sels {
		if s.Pkg.Name != "tgttool" || s.Reason != "missing archive" {
			t.Fatalf("selection = %+v", s)
		}
		if !platformsEqual(s.Platforms, all) {
			t.Fatalf("Platforms for %s = %v, want %v", s.Version, s.Platforms, all)
		}
	}

	layout, err := tgt.Overlay.Open("tgttool")
	if err != nil {
		t.Fatalf("Overlay.Open: %v", err)
	}
	plat := spec.Current()
	if _, _, err := ocix.PushArchive(ctx, layout, "v2.0.0", plat, []byte("archive-bytes"), false); err != nil {
		t.Fatalf("PushArchive: %v", err)
	}

	sels, stats, err = build.SelectMissing(ctx, tgt)
	if err != nil {
		t.Fatalf("SelectMissing (after staging): %v", err)
	}
	if stats.Checked != 1 || stats.Skipped != 0 {
		t.Fatalf("stats after staging = %+v, want Checked=1 Skipped=0", stats)
	}
	if len(sels) != 2 {
		t.Fatalf("sels after staging = %+v, want both versions still selected", sels)
	}
	byVersion := map[string][]spec.Platform{}
	for _, s := range sels {
		byVersion[s.Version] = s.Platforms
	}
	if want := removePlatform(all, plat); !platformsEqual(byVersion["v2.0.0"], want) {
		t.Fatalf("Platforms for v2.0.0 = %v, want %v", byVersion["v2.0.0"], want)
	}
	if !platformsEqual(byVersion["v1.0.0"], all) {
		t.Fatalf("Platforms for v1.0.0 = %v, want %v", byVersion["v1.0.0"], all)
	}
}

func TestSelectMissingSkipsNoBuild(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeSelectFixture(t, root, "nobuildtool", selectNoBuildPkg)

	tgt, err := build.OpenTarget(ctx, root)
	if err != nil {
		t.Fatalf("OpenTarget: %v", err)
	}

	sels, stats, err := build.SelectMissing(ctx, tgt)
	if err != nil {
		t.Fatalf("SelectMissing: %v", err)
	}
	if len(sels) != 0 {
		t.Fatalf("sels = %+v, want none selected for a build-less oci package", sels)
	}
	if stats.Checked != 0 || stats.Skipped != 1 {
		t.Fatalf("stats = %+v, want Checked=0 Skipped=1", stats)
	}
}

const selectBadNamePkg = `
schema: 2
name: wrongname
artifact:
  url: "https://example.com/x-{{.Version}}-{{.OS}}-{{.Arch}}.tar.gz"
install:
  - extract: {}
versions:
  - version: v1.0.0
    sha256:
      darwin/arm64: "aaa"
      darwin/amd64: "bbb"
      linux/arm64: "ccc"
      linux/amd64: "ddd"
`

func TestSelectMissingDirRejectsInvalidSpec(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeSelectFixture(t, root, "badname", selectBadNamePkg)

	tgt, err := build.OpenTarget(ctx, root)
	if err != nil {
		t.Fatalf("OpenTarget: %v", err)
	}

	_, _, err = build.SelectMissing(ctx, tgt)
	if err == nil {
		t.Fatal("want an error: the dir scan must route through the validating Load path")
	}
	if !strings.Contains(err.Error(), "badname") {
		t.Fatalf("err = %v, want it to name the offending package", err)
	}
}

type fakeCatalogSource struct {
	names []string
	pkgs  map[string]*spec.Package
}

var _ catalog.Catalog = (*fakeCatalogSource)(nil)

func (f *fakeCatalogSource) PackageNames(context.Context) ([]string, error) { return f.names, nil }

func (f *fakeCatalogSource) Summaries(context.Context) ([]catalog.Summary, error) {
	out := make([]catalog.Summary, 0, len(f.names))
	for _, n := range f.names {
		out = append(out, catalog.Summary{Name: n})
	}
	return out, nil
}

func (f *fakeCatalogSource) Package(_ context.Context, name string) (*spec.Package, string, error) {
	pkg, ok := f.pkgs[name]
	if !ok {
		return nil, "", &catalog.PackageNotFoundError{Name: name}
	}
	return pkg, "sha256:fake", nil
}

func (f *fakeCatalogSource) Versions(ctx context.Context, name string) ([]string, error) {
	pkg, _, err := f.Package(ctx, name)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(pkg.Versions))
	for i, v := range pkg.Versions {
		out[i] = v.Version
	}
	return out, nil
}

func TestSelectMissingOCI(t *testing.T) {
	ctx := context.Background()
	const ref = "ghcr.io/org/cat:v2"
	src := &fakeCatalogSource{
		names: []string{"pretool", "tgttool"},
		pkgs: map[string]*spec.Package{
			"pretool": parseSelectPkg(t, selectPrebuiltPkg),
			"tgttool": parseSelectPkg(t, selectTargetPkg),
		},
	}
	tgt := &build.Target{Ref: ref, Entry: catalog.Entry{Name: ref, Catalog: src}}

	store := memory.New()
	var gotRef string
	t.Cleanup(build.SetArchivesReader(func(catalogRef, _ string) (oras.ReadOnlyTarget, error) {
		gotRef = catalogRef
		return store, nil
	}))

	sels, stats, err := build.SelectMissing(ctx, tgt)
	if err != nil {
		t.Fatalf("SelectMissing: %v", err)
	}
	if gotRef != ref {
		t.Fatalf("archives opener got ref %q, want %q", gotRef, ref)
	}
	if stats.Checked != 1 || stats.Skipped != 1 {
		t.Fatalf("stats = %+v, want Checked=1 Skipped=1", stats)
	}
	if len(sels) != 2 {
		t.Fatalf("sels = %+v, want both tgttool versions", sels)
	}

	plat := spec.Current()
	if _, _, err := ocix.PushArchive(ctx, store, "v2.0.0", plat, []byte("archive-bytes"), false); err != nil {
		t.Fatalf("PushArchive: %v", err)
	}

	sels, _, err = build.SelectMissing(ctx, tgt)
	if err != nil {
		t.Fatalf("SelectMissing (after push): %v", err)
	}
	if len(sels) != 2 {
		t.Fatalf("sels after push = %+v, want both versions still selected", sels)
	}
	for _, s := range sels {
		want := spec.SupportedPlatforms
		if s.Version == "v2.0.0" {
			want = removePlatform(spec.SupportedPlatforms, plat)
		}
		if !platformsEqual(s.Platforms, want) {
			t.Fatalf("Platforms for %s = %v, want %v", s.Version, s.Platforms, want)
		}
	}
}

type erroringArchives struct{ err error }

func (e erroringArchives) Fetch(context.Context, ocispec.Descriptor) (io.ReadCloser, error) {
	return nil, e.err
}

func (e erroringArchives) Exists(context.Context, ocispec.Descriptor) (bool, error) {
	return false, e.err
}

func (e erroringArchives) Resolve(context.Context, string) (ocispec.Descriptor, error) {
	return ocispec.Descriptor{}, e.err
}

func TestSelectMissingAbortsOnRegistryFailure(t *testing.T) {
	ctx := context.Background()
	const ref = "ghcr.io/org/cat:v2"
	src := &fakeCatalogSource{
		names: []string{"tgttool"},
		pkgs: map[string]*spec.Package{
			"tgttool": parseSelectPkg(t, selectTargetPkg),
		},
	}
	tgt := &build.Target{Ref: ref, Entry: catalog.Entry{Name: ref, Catalog: src}}

	t.Cleanup(build.SetArchivesReader(func(string, string) (oras.ReadOnlyTarget, error) {
		return erroringArchives{err: errors.New("registry unreachable")}, nil
	}))

	_, _, err := build.SelectMissing(ctx, tgt)
	if err == nil {
		t.Fatal("a non-not-found registry failure must abort the scan")
	}
	if !strings.Contains(err.Error(), "tgttool@v2.0.0") {
		t.Fatalf("err = %v, want it to name the package@version being probed", err)
	}
}

func selectDirTarget(t *testing.T) *build.Target {
	t.Helper()
	root := t.TempDir()
	writeSelectFixture(t, root, "apptool", selectAppPkg)
	writeSelectFixture(t, root, "tgttool", selectTargetPkg)
	writeSelectFixture(t, root, "pretool", selectPrebuiltPkg)
	writeSelectFixture(t, root, "nobuildtool", selectNoBuildPkg)

	tgt, err := build.OpenTarget(context.Background(), root)
	if err != nil {
		t.Fatalf("OpenTarget: %v", err)
	}
	return tgt
}

func selectReasons(sels []build.Selection) map[string]string {
	out := make(map[string]string, len(sels))
	for _, s := range sels {
		out[s.Pkg.Name+"@"+s.Version] = s.Reason
	}
	return out
}

func assertReasons(t *testing.T, sels []build.Selection, want map[string]string) {
	t.Helper()
	got := selectReasons(sels)
	if len(got) != len(sels) {
		t.Fatalf("selections carry duplicates: %v", got)
	}
	if len(got) != len(want) {
		t.Fatalf("selections = %v, want %v", got, want)
	}
	for key, reason := range want {
		if got[key] != reason {
			t.Fatalf("selection %s reason = %q, want %q (all: %v)", key, got[key], reason, got)
		}
	}
}

func TestSelectBarePackageNameTakesTheLatestVersion(t *testing.T) {
	tgt := selectDirTarget(t)

	sels, err := build.Select(context.Background(), tgt, catalog.NewSet(), []string{"tgttool"}, false)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if len(sels) != 1 {
		t.Fatalf("sels = %v, want one selection", selectReasons(sels))
	}
	if sels[0].Version != "v2.0.0" || sels[0].Reason != "forced" {
		t.Fatalf("selection = %+v, want the spec's first version selected as forced", sels[0])
	}
	if !platformsEqual(sels[0].Platforms, sels[0].Pkg.SupportedBy()) {
		t.Fatalf("Platforms = %v, want every platform the package supports", sels[0].Platforms)
	}
}

func TestSelectRejectsMalformedSelectors(t *testing.T) {
	tgt := selectDirTarget(t)

	tests := []struct {
		name     string
		selector string
		want     string
	}{
		{"empty version", "tgttool@",
			`invalid package selection "tgttool@": want name or name@version`},
		{"empty name", "@v1.0.0",
			`invalid package selection "@v1.0.0": want name or name@version`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := build.Select(context.Background(), tgt, catalog.NewSet(), []string{tc.selector}, false)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestSelectNamesTheCatalogForAnUnknownPackage(t *testing.T) {
	tgt := selectDirTarget(t)

	_, err := build.Select(context.Background(), tgt, catalog.NewSet(), []string{"ghost@v1.0.0"}, false)
	if err == nil {
		t.Fatal("an unknown package must be rejected")
	}
	if want := "package ghost not found in catalog(s) " + tgt.Ref; err.Error() != want {
		t.Fatalf("err = %q, want %q", err.Error(), want)
	}
	if _, ok := errors.AsType[*catalog.PackageNotFoundError](err); !ok {
		t.Fatalf("err = %v, want a *catalog.PackageNotFoundError underneath so the hint still fires", err)
	}
}

func TestMergeKeepsForcedSelectionsOverTheMissingScan(t *testing.T) {
	ctx := context.Background()
	tgt := selectDirTarget(t)

	sels, err := build.Select(ctx, tgt, catalog.NewSet(), []string{"tgttool@v2.0.0"}, false)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	missing, _, err := build.SelectMissing(ctx, tgt)
	if err != nil {
		t.Fatalf("SelectMissing: %v", err)
	}
	assertReasons(t, build.Merge(sels, missing), map[string]string{
		"tgttool@v2.0.0": "forced",
		"tgttool@v1.0.0": "missing archive",
		"apptool@v1.0.0": "missing archive",
	})
}

func TestSelectWithDepsSkipsPrebuiltAndBuildLessDeps(t *testing.T) {
	tgt := selectDirTarget(t)

	sels, err := build.Select(context.Background(), tgt, catalog.NewSet(), []string{"apptool@v1.0.0"}, true)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	assertReasons(t, sels, map[string]string{
		"apptool@v1.0.0": "forced",
		"tgttool@v2.0.0": "dep of apptool",
	})
}

func TestSelectWithDepsSkipsDepsTheTargetHolds(t *testing.T) {
	ctx := context.Background()
	tgt := selectDirTarget(t)

	layout, err := tgt.Overlay.Open("tgttool")
	if err != nil {
		t.Fatalf("Overlay.Open: %v", err)
	}
	if _, _, err := ocix.PushArchive(ctx, layout, "v2.0.0", spec.Current(), []byte("archive-bytes"), false); err != nil {
		t.Fatalf("PushArchive: %v", err)
	}

	sels, err := build.Select(ctx, tgt, catalog.NewSet(), []string{"apptool@v1.0.0"}, true)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	assertReasons(t, sels, map[string]string{"apptool@v1.0.0": "forced"})
}

func TestSelectWithDepsComposesWithTheMissingScan(t *testing.T) {
	ctx := context.Background()
	tgt := selectDirTarget(t)

	sels, err := build.Select(ctx, tgt, catalog.NewSet(), []string{"apptool@v1.0.0"}, true)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	missing, stats, err := build.SelectMissing(ctx, tgt)
	if err != nil {
		t.Fatalf("SelectMissing: %v", err)
	}
	if stats.Checked != 2 || len(missing) != 3 || stats.Skipped != 2 {
		t.Fatalf("scan = checked %d, %d incomplete, %d skipped; want 2, 3, 2",
			stats.Checked, len(missing), stats.Skipped)
	}
	assertReasons(t, build.Merge(sels, missing), map[string]string{
		"apptool@v1.0.0": "forced",
		"tgttool@v2.0.0": "dep of apptool",
		"tgttool@v1.0.0": "missing archive",
	})
}
