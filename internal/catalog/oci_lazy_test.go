package catalog

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"oras.land/oras-go/v2/content/oci"

	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/ocix/ocixtest"
)

const ociGoYAML = `
schema: 2
name: go
description: The Go programming language
artifact:
  oci: ":{{.Version}}"
install:
  - extract: {}
versions:
  - v1.26.5
  - v1.26.4
`

func localOCI(name, storePath string) *OCI {
	return NewLazyOCI(name, func(ctx context.Context) (*ocix.Store, error) {
		return ocix.OpenLocalStore(ctx, storePath)
	})
}

func syncedStore(t *testing.T) string {
	t.Helper()
	src, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ocixtest.PushFakeCatalog(t, src, []ocixtest.FakeEntry{{
		Name: "go", Description: "The Go programming language", Latest: "v1.26.5",
		YAML: []byte(ociGoYAML),
	}}, "2")
	storePath := filepath.Join(t.TempDir(), "store")
	if _, err := ocix.SyncLocalCatalog(context.Background(), src, "v2", storePath, nil); err != nil {
		t.Fatal(err)
	}
	return storePath
}

func TestLazyOCILoadAndVersions(t *testing.T) {
	s := localOCI("official", syncedStore(t))
	ctx := context.Background()
	pkg, dig, err := s.Package(ctx, "go")
	if err != nil || pkg.Name != "go" || dig == "" {
		t.Fatalf("Load: %+v, %q, %v", pkg, dig, err)
	}
	pkg2, _, _ := s.Package(ctx, "go")
	if pkg2 != pkg {
		t.Fatal("memoization broken: distinct pointers")
	}
	vs, err := s.Versions(ctx, "go")
	if err != nil || len(vs) != 2 || vs[0] != "v1.26.5" {
		t.Fatalf("Versions: %v, %v", vs, err)
	}
	var nf *PackageNotFoundError
	if _, _, err := s.Package(ctx, "absent"); !errors.As(err, &nf) {
		t.Fatalf("want PackageNotFoundError, got %v", err)
	}
}

func BenchmarkLazyOCIResolvePattern(b *testing.B) {
	src, err := oci.New(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	const pkgs = 30
	entries := make([]ocixtest.FakeEntry, pkgs)
	for i := range entries {
		name := fmt.Sprintf("pkg%d", i)
		entries[i] = ocixtest.FakeEntry{
			Name: name, Description: "bench", Latest: "v1.0.0",
			YAML: []byte(fmt.Sprintf("schema: 2\nname: %s\nartifact: {oci: \":{{.Version}}\"}\ninstall: [{extract: {}}]\nversions: [v1.0.0]\n", name)),
		}
	}
	ocixtest.PushFakeCatalog(b, src, entries, "2")
	storePath := filepath.Join(b.TempDir(), "store")
	if _, err := ocix.SyncLocalCatalog(context.Background(), src, "v2", storePath, nil); err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	b.ResetTimer()
	for b.Loop() {
		s := localOCI("official", storePath)
		for range 4 {
			for i := range pkgs {
				if _, _, err := s.Package(ctx, fmt.Sprintf("pkg%d", i)); err != nil {
					b.Fatal(err)
				}
			}
		}
	}
}

func TestLazyOCIRetriesAfterFailedOpen(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "store")
	s := localOCI("official", storePath)

	if _, err := s.Summaries(ctx); !errors.Is(err, ocix.ErrNotSynced) {
		t.Fatalf("first Summaries: want ErrNotSynced, got %v", err)
	}
	if _, _, err := s.Package(ctx, "go"); !errors.Is(err, ocix.ErrNotSynced) {
		t.Fatalf("first Load: want ErrNotSynced, got %v", err)
	}

	src, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ocixtest.PushFakeCatalog(t, src, []ocixtest.FakeEntry{{
		Name: "go", Description: "The Go programming language", Latest: "v1.26.5",
		YAML: []byte(ociGoYAML),
	}}, "2")
	if _, err := ocix.SyncLocalCatalog(ctx, src, "v2", storePath, nil); err != nil {
		t.Fatal(err)
	}

	sums, err := s.Summaries(ctx)
	if err != nil || len(sums) != 1 || sums[0].Name != "go" {
		t.Fatalf("retry Summaries: %+v, %v", sums, err)
	}
	pkg, dig, err := s.Package(ctx, "go")
	if err != nil || pkg.Name != "go" || dig == "" {
		t.Fatalf("retry Load: %+v, %q, %v", pkg, dig, err)
	}
}
