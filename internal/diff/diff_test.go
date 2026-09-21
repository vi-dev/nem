package diff

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vi-dev/nem/internal/catalog"
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
