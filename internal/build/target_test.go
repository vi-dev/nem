package build

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"

	"github.com/vi-dev/nem/internal/archive"
	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/report"
	"github.com/vi-dev/nem/internal/spec"
	"github.com/vi-dev/nem/internal/testx"
)

const targetPkgYAML = `schema: 2
name: xtool
description: Target fixture
artifact:
  oci: ":{{.Version}}"
install:
  - extract: {}
versions:
  - version: v1.0.0
`

func TestOpenTargetDir(t *testing.T) {
	root := t.TempDir()
	testx.WriteFile(t, filepath.Join(root, "pkgs", "xtool", "pkg.yaml"), targetPkgYAML)

	rep := &testx.Reporter{}
	tgt, err := OpenTarget(report.NewContext(context.Background(), rep), root)
	if err != nil {
		t.Fatalf("OpenTarget: %v", err)
	}
	if len(rep.Tasks()) != 0 || len(rep.Infos()) != 0 {
		t.Fatalf("a dir target must open silently, got tasks=%v infos=%v", rep.Tasks(), rep.Infos())
	}
	if !tgt.IsDir() {
		t.Fatalf("IsDir = false, want true for %s", root)
	}
	if tgt.Ref != root || tgt.Dir != root {
		t.Fatalf("Ref/Dir = %q/%q, want %q", tgt.Ref, tgt.Dir, root)
	}
	if tgt.Overlay == nil {
		t.Fatal("Overlay = nil, want an archive store rooted at the catalog dir")
	}
	w, err := tgt.Overlay.OpenRW(context.Background(), "probe")
	if err != nil {
		t.Fatalf("Overlay.OpenRW: %v", err)
	}
	if _, _, err := archive.Push(context.Background(), w, "v1", spec.Current(), archive.BytesBlob([]byte("x")), false); err != nil {
		t.Fatalf("stage probe archive: %v", err)
	}
	if !archive.NewDir(root).HasVersion("probe", "v1") {
		t.Fatalf("Overlay is not rooted at the catalog dir %s", root)
	}
	if tgt.Entry.Name != root {
		t.Fatalf("Entry.Name = %q, want %q", tgt.Entry.Name, root)
	}
	pkg, _, err := tgt.Entry.Catalog.Package(context.Background(), "xtool")
	if err != nil {
		t.Fatalf("Entry.Load: %v", err)
	}
	if pkg.Name != "xtool" {
		t.Fatalf("loaded package %q, want xtool", pkg.Name)
	}
}

func TestOpenTargetOCIRefError(t *testing.T) {

	missing := filepath.Join(t.TempDir(), "nope")
	_, err := OpenTarget(context.Background(), missing)
	if err == nil {
		t.Fatal("OpenTarget succeeded for a nonexistent path, want an error")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Fatalf("error %q does not mention the ref %q", err, missing)
	}
}

func TestTargetArchivesDirDoesNotCreateLayouts(t *testing.T) {
	root := t.TempDir()
	testx.WriteFile(t, filepath.Join(root, "pkgs", "xtool", "pkg.yaml"), targetPkgYAML)

	tgt, err := OpenTarget(context.Background(), root)
	if err != nil {
		t.Fatalf("OpenTarget: %v", err)
	}

	src, err := tgt.Archives(context.Background(), "xtool")
	if !errors.Is(err, archive.ErrNotFound) {
		t.Fatalf("Archives error = %v, want one satisfying archive.ErrNotFound", err)
	}
	if src != nil {
		t.Fatalf("Archives returned %#v with an error, want nil", src)
	}
	index := filepath.Join(root, "archives", "xtool", "index.json")
	if _, err := os.Stat(index); !os.IsNotExist(err) {
		t.Fatalf("probing archives created %s (stat err = %v); it must never open a layout", index, err)
	}

	layout, err := tgt.Overlay.OpenRW(context.Background(), "xtool")
	if err != nil {
		t.Fatalf("Overlay.Open: %v", err)
	}
	plat := spec.Current()
	if _, _, err := archive.Push(context.Background(), layout, "v1.0.0", plat, archive.BytesBlob([]byte("archive-bytes")), false); err != nil {
		t.Fatalf("PushArchive: %v", err)
	}
	src, err = tgt.Archives(context.Background(), "xtool")
	if err != nil {
		t.Fatalf("Archives after staging: %v", err)
	}
	plats, err := archive.ResolvePlatforms(context.Background(), src, "v1.0.0")
	if err != nil {
		t.Fatalf("ArchivePlatforms: %v", err)
	}
	if len(plats) != 1 || plats[0] != plat {
		t.Fatalf("staged platforms = %v, want [%s]", plats, plat)
	}
}

func TestTargetArchivesOCIUsesRegistry(t *testing.T) {
	store := memory.New()
	var gotRef string
	t.Cleanup(archive.SetRepoOpener(func(ref string) (oras.Target, error) {
		gotRef = ref
		return store, nil
	}))

	tgt := &Target{Ref: "ghcr.io/org/cat:v2", Entry: catalog.Entry{Archives: archive.Remote("ghcr.io/org/cat:v2")}}
	src, err := tgt.Archives(context.Background(), "xtool")
	if err != nil {
		t.Fatalf("Archives: %v", err)
	}
	if src != oras.ReadOnlyTarget(store) {
		t.Fatalf("Archives returned %#v, want the registry target", src)
	}
	want, err := archive.Ref("ghcr.io/org/cat:v2", "xtool")
	if err != nil {
		t.Fatalf("archive.Ref: %v", err)
	}
	if gotRef != want {
		t.Fatalf("opener called with ref %q, want %q", gotRef, want)
	}
}
