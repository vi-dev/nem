package catalog

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/vi-dev/nem/internal/archive"
	"github.com/vi-dev/nem/internal/config"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/testx"
)

func TestOpenConfiguredBuildsEntriesInOrder(t *testing.T) {
	h := testx.Home(t)
	cfg := &config.Config{Catalogs: []config.CatalogEntry{
		{Name: "dev", Type: "dir", Path: t.TempDir()},
		{Name: "official", Type: "oci", Ref: config.OfficialRef},
	}}
	set, err := OpenConfigured(cfg, h)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	entries := set.Entries()
	if len(entries) != 2 || entries[0].Name != "dev" || entries[1].Name != "official" {
		t.Fatalf("Open: %+v", entries)
	}
	if entries[0].Ref != "" {
		t.Fatalf("dir entry must have no ref, got %q", entries[0].Ref)
	}
	if entries[1].Ref != config.OfficialRef {
		t.Fatalf("oci entry ref: %q", entries[1].Ref)
	}
	if _, ok := entries[0].Catalog.(*Dir); !ok {
		t.Fatal("dev should be a Dir source")
	}
	src, ok := entries[1].Catalog.(*OCI)
	if !ok {
		t.Fatalf("official should be an OCI source, got %T", entries[1].Catalog)
	}
	if _, err := src.PackageNames(context.Background()); !errors.Is(err, ocix.ErrNotSynced) {
		t.Fatalf("want ErrNotSynced from the unopened store, got %v", err)
	}
	if _, ok := entries[0].Archives.(*archive.Dir); !ok {
		t.Fatalf("dev entry archives = %T, want *archive.Dir", entries[0].Archives)
	}
	if entries[1].Archives == nil {
		t.Fatal("official entry archives should not be nil")
	}
}

func TestOpenConfiguredSkipsDisabled(t *testing.T) {
	cfg := &config.Config{Catalogs: []config.CatalogEntry{
		{Name: "a", Type: "dir", Path: "/tmp/a"},
		{Name: "b", Type: "dir", Path: "/tmp/b", Disabled: true},
		{Name: "c", Type: "dir", Path: "/tmp/c"},
	}}
	set, err := OpenConfigured(cfg, testx.Home(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	var got []string
	for _, e := range set.Entries() {
		got = append(got, e.Name)
	}
	if !slices.Equal(got, []string{"a", "c"}) {
		t.Fatalf("Open should skip disabled and preserve order, got %v", got)
	}
}

func TestOpenDetectsDirCatalog(t *testing.T) {
	entry, err := Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, ok := entry.Catalog.(*Dir); !ok {
		t.Fatalf("want *Dir, got %T", entry.Catalog)
	}
}

func TestOpenDetectsFileCatalog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pkg.yaml")
	if err := os.WriteFile(path, []byte("name: demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	entry, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, ok := entry.Catalog.(*File); !ok {
		t.Fatalf("want *File, got %T", entry.Catalog)
	}
}

func TestOpenDirEntryServesDirArchives(t *testing.T) {
	dir := t.TempDir()
	entry, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, ok := entry.Archives.(*archive.Dir); !ok {
		t.Fatalf("dir entry archives = %T, want *archive.Dir", entry.Archives)
	}
}

func TestOpenFileEntryHasNoArchives(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pkg.yaml")
	if err := os.WriteFile(path, []byte("name: demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	entry, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, aerr := entry.Archives.Open(context.Background(), "tool"); !errors.Is(aerr, archive.ErrNotFound) {
		t.Fatalf("file entry archives err = %v, want ErrNotFound", aerr)
	}
}

func TestOpenMissingPathIsNotTreatedAsRef(t *testing.T) {
	for _, arg := range []string{"./missing", "../missing", "/nonexistent/catalog", "missing/pkg.yaml", "missing.yml"} {
		if _, err := Open(context.Background(), arg); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Open(%q): want not-exist error, got %v", arg, err)
		}
	}
}

func TestOpenRejectsRefWithoutTagOrDigest(t *testing.T) {
	if _, err := Open(context.Background(), "ghcr.io/vi-dev/nem-catalog"); err == nil {
		t.Fatal("want error for OCI ref without tag or digest")
	}
}

func TestOpenPullsRemoteRef(t *testing.T) {
	want := NewDir(t.TempDir())
	var gotRef string
	testx.Swap(t, &openRemote, func(ctx context.Context, ref string) (Catalog, error) {
		gotRef = ref
		return want, nil
	})
	const ref = "ghcr.io/vi-dev/nem-catalog:latest"
	entry, err := Open(context.Background(), ref)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if entry.Catalog != Catalog(want) {
		t.Fatalf("want the remote-opened catalog, got %T", entry.Catalog)
	}
	if entry.Ref != ref {
		t.Fatalf("entry ref = %q, want %q", entry.Ref, ref)
	}
	if entry.Archives == nil {
		t.Fatal("remote entry archives should not be nil")
	}
	if gotRef != ref {
		t.Fatalf("remote opener got ref %q, want %q", gotRef, ref)
	}
}
