package archive

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"

	"github.com/vi-dev/nem/internal/spec"
)

func TestDirOpenMissingReturnsNotFoundAndCreatesNothing(t *testing.T) {
	root := t.TempDir()
	_, err := NewDir(root).Open(context.Background(), "tool")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open err = %v, want ErrNotFound", err)
	}
	if _, err := os.Stat(filepath.Join(root, "archives")); !os.IsNotExist(err) {
		t.Fatalf("read Open created files under %s", root)
	}
}

func TestDirOpenRWStagesAndOpenReads(t *testing.T) {
	ctx := context.Background()
	d := NewDir(t.TempDir())
	plat := spec.Platform{OS: "linux", Arch: "amd64"}

	w, err := d.OpenRW(context.Background(), "tool")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Push(ctx, w, "v1.0.0", plat, BytesBlob([]byte("blob")), false); err != nil {
		t.Fatal(err)
	}

	r, err := d.Open(context.Background(), "tool")
	if err != nil {
		t.Fatal(err)
	}
	rc, err := Fetch(ctx, r, "v1.0.0", plat)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "blob" {
		t.Fatalf("Read = %q, want %q", got, "blob")
	}
}

func TestNoOPReturnsNotFound(t *testing.T) {
	if _, err := NopStore.Open(context.Background(), "tool"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("NoOP.Open err = %v, want ErrNotFound", err)
	}
}

func TestRemoteOpensArchivesRefRepo(t *testing.T) {
	var gotRef string
	restore := SetRepoOpener(func(ref string) (oras.Target, error) {
		gotRef = ref
		return memory.New(), nil
	})
	defer restore()

	if _, err := Remote("registry.example/org/cat:v2").Open(context.Background(), "tool"); err != nil {
		t.Fatal(err)
	}
	if want := "registry.example/org/cat/archives/tool"; gotRef != want {
		t.Fatalf("opened %q, want %q", gotRef, want)
	}
}

var (
	_ ReadWriteStore = (*Dir)(nil)
	_ ReadWriteStore = remote{}
	_ Store          = NopStore
)
