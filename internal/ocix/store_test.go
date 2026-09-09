package ocix_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/oci"

	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/ocix/ocixtest"
)

func TestSyncAndLoad(t *testing.T) {
	ctx := context.Background()
	srcDir := t.TempDir()
	src, err := oci.New(srcDir)
	if err != nil {
		t.Fatalf("oci.New: %v", err)
	}
	goYAML := []byte("schema: 2\nname: go\n")
	kubectlYAML := []byte("schema: 2\nname: kubectl\n")
	ocixtest.PushFakeCatalog(t, src, []ocixtest.FakeEntry{
		{Name: "go", Description: "The Go programming language", Latest: "v1.26.5", YAML: goYAML},
		{Name: "kubectl", Description: "Kubernetes CLI", Latest: "v1.34.1", YAML: kubectlYAML},
	}, "2")

	storePath := filepath.Join(t.TempDir(), "store")
	s, err := ocix.SyncLocalCatalog(ctx, src, "v2", storePath, nil)
	if err != nil {
		t.Fatalf("SyncFrom: %v", err)
	}
	if !strings.HasPrefix(s.Digest(), "sha256:") {
		t.Fatalf("index digest: %q", s.Digest())
	}
	if len(s.Index().Manifests) != 2 {
		t.Fatalf("Index: %+v", s.Index())
	}

	s, err = ocix.OpenLocalStore(ctx, storePath)
	if err != nil || len(s.Index().Manifests) != 2 {
		t.Fatalf("OpenStore: %v", err)
	}

	pkgs := s.Packages()
	if len(pkgs) != 2 || pkgs[0].Title != "go" || pkgs[1].Title != "kubectl" {
		t.Fatalf("Packages: %+v", pkgs)
	}

	data, mdig, err := s.PkgBytes(ctx, "go")
	if err != nil || string(data) != string(goYAML) || !strings.HasPrefix(mdig, "sha256:") {
		t.Fatalf("PkgBytes: %q, %q, %v", data, mdig, err)
	}

	var nf *ocix.PkgNotInIndexError
	if _, _, err := s.PkgBytes(ctx, "absent"); !errors.As(err, &nf) {
		t.Fatalf("want PkgNotInIndexError, got %v", err)
	}
}

func TestSyncRejectsWrongSchema(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "store")

	good, _ := oci.New(t.TempDir())
	ocixtest.PushFakeCatalog(t, good, []ocixtest.FakeEntry{{Name: "go", YAML: []byte("x")}}, "2")
	if _, err := ocix.SyncLocalCatalog(ctx, good, "v2", storePath, nil); err != nil {
		t.Fatalf("seed sync: %v", err)
	}

	bad, _ := oci.New(t.TempDir())
	ocixtest.PushFakeCatalog(t, bad, []ocixtest.FakeEntry{{Name: "go", YAML: []byte("x")}}, "9")
	_, err := ocix.SyncLocalCatalog(ctx, bad, "v2", storePath, nil)
	if err == nil || !strings.Contains(err.Error(), "schema") {
		t.Fatalf("want schema error, got %v", err)
	}

	s, err := ocix.OpenLocalStore(ctx, storePath)
	if err != nil || len(s.Index().Manifests) != 1 {
		t.Fatalf("good mirror clobbered by bad-schema sync attempt: %v", err)
	}
}

func TestOpenStoreUnsynced(t *testing.T) {
	_, err := ocix.OpenLocalStore(context.Background(), filepath.Join(t.TempDir(), "nope"))
	if !errors.Is(err, ocix.ErrNotSynced) {
		t.Fatalf("want ErrNotSynced, got %v", err)
	}
}

type movingResolveTarget struct {
	oras.ReadOnlyTarget
	ref         string
	first, rest ocispec.Descriptor
	calls       int
}

func (m *movingResolveTarget) Resolve(ctx context.Context, ref string) (ocispec.Descriptor, error) {
	if ref != m.ref {
		return m.ReadOnlyTarget.Resolve(ctx, ref)
	}
	m.calls++
	if m.calls == 1 {
		return m.first, nil
	}
	return m.rest, nil
}

func TestSyncFromDetectsMovingTag(t *testing.T) {
	ctx := context.Background()
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatalf("oci.New: %v", err)
	}
	oldDesc := ocixtest.PushFakeCatalog(t, store, []ocixtest.FakeEntry{{Name: "go", YAML: []byte("old")}}, ocix.SchemaVersion)
	newDesc := ocixtest.PushFakeCatalog(t, store, []ocixtest.FakeEntry{{Name: "go", YAML: []byte("new")}}, ocix.SchemaVersion)
	if oldDesc.Digest == newDesc.Digest {
		t.Fatal("fixture bug: old and new catalog content must digest differently")
	}

	moving := &movingResolveTarget{ReadOnlyTarget: store, ref: "v2", first: oldDesc, rest: newDesc}
	storePath := filepath.Join(t.TempDir(), "store")
	_, err = ocix.SyncLocalCatalog(ctx, moving, "v2", storePath, nil)
	if err == nil || !strings.Contains(err.Error(), "catalog changed during sync; retry") {
		t.Fatalf("SyncFrom error = %v, want a catalog-changed-during-sync error", err)
	}
}

func TestSyncIsIdempotent(t *testing.T) {
	ctx := context.Background()
	src, _ := oci.New(t.TempDir())
	ocixtest.PushFakeCatalog(t, src, []ocixtest.FakeEntry{{Name: "go", YAML: []byte("y")}}, "2")
	storePath := filepath.Join(t.TempDir(), "store")
	s1, err := ocix.SyncLocalCatalog(ctx, src, "v2", storePath, nil)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := ocix.SyncLocalCatalog(ctx, src, "v2", storePath, nil)
	if err != nil || s1.Digest() != s2.Digest() {
		t.Fatalf("resync: %q vs %q, %v", s1.Digest(), s2.Digest(), err)
	}
}

func TestFetchPkgBytesReturnsLayer(t *testing.T) {
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	yaml := []byte("name: atool\n")
	ocixtest.PushFakeCatalog(t, store, []ocixtest.FakeEntry{
		{Name: "atool", Latest: "1.0.0", YAML: yaml},
	}, ocix.SchemaVersion)
	idx, _, err := ocix.FetchCatalogIndex(context.Background(), store, "v2")
	if err != nil {
		t.Fatal(err)
	}

	got, err := ocix.FetchPkgBytes(context.Background(), store, idx.Manifests[0])
	if err != nil {
		t.Fatalf("FetchPkgBytes: %v", err)
	}
	if !bytes.Equal(got, yaml) {
		t.Fatalf("bytes = %q, want %q", got, yaml)
	}
}

func TestFetchPkgBytesRejectsManifestWithoutPkgLayer(t *testing.T) {
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	ocixtest.PushFakeArchive(t, store, "1.0.0", map[string][]byte{"linux/amd64": []byte("a")})
	desc, err := store.Resolve(context.Background(), "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	idxData, err := content.FetchAll(context.Background(), store, desc)
	if err != nil {
		t.Fatal(err)
	}
	var archIdx ocispec.Index
	if err := json.Unmarshal(idxData, &archIdx); err != nil {
		t.Fatal(err)
	}

	if _, err := ocix.FetchPkgBytes(context.Background(), store, archIdx.Manifests[0]); err == nil {
		t.Fatal("FetchPkgBytes accepted a manifest without a pkg.yaml layer, want error")
	}
}

func TestStageIndexClosureStable(t *testing.T) {
	ctx := context.Background()
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatalf("oci.New: %v", err)
	}
	desc := ocixtest.PushFakeCatalog(t, store, []ocixtest.FakeEntry{{Name: "go", YAML: []byte("y")}}, ocix.SchemaVersion)

	staged, err := ocix.OpenStoreInMemory(ctx, store, "v2", nil)
	if err != nil {
		t.Fatalf("StageInMemory: %v", err)
	}
	if len(staged.Index().Manifests) != 1 {
		t.Fatalf("manifests = %d, want 1", len(staged.Index().Manifests))
	}
	if staged.Digest() != desc.Digest.String() {
		t.Fatalf("staged digest = %s, want %s", staged.Digest(), desc.Digest)
	}
}

func TestStageIndexClosureRejectsWrongSchema(t *testing.T) {
	ctx := context.Background()
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatalf("oci.New: %v", err)
	}
	ocixtest.PushFakeCatalog(t, store, []ocixtest.FakeEntry{{Name: "go", YAML: []byte("x")}}, "9")

	_, err = ocix.OpenStoreInMemory(ctx, store, "v2", nil)
	if err == nil || !strings.Contains(err.Error(), "schema") {
		t.Fatalf("StageInMemory error = %v, want a schema error", err)
	}
}

func TestOpenLocalStoreNeverWritesToBrokenStore(t *testing.T) {
	storePath := t.TempDir()
	if err := os.WriteFile(filepath.Join(storePath, "stray"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ocix.OpenLocalStore(context.Background(), storePath)
	if !errors.Is(err, ocix.ErrNotSynced) {
		t.Fatalf("want ErrNotSynced, got %v", err)
	}

	entries, err := os.ReadDir(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "stray" {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Fatalf("OpenLocalStore wrote into the store dir: %v", names)
	}
}

func BenchmarkOpenLocalStore(b *testing.B) {
	ctx := context.Background()
	src, err := oci.New(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	ocixtest.PushFakeCatalog(b, src, fakeCatalogEntries(300), ocix.SchemaVersion)
	storePath := filepath.Join(b.TempDir(), "store")
	if _, err := ocix.SyncLocalCatalog(ctx, src, "v2", storePath, nil); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		if _, err := ocix.OpenLocalStore(ctx, storePath); err != nil {
			b.Fatal(err)
		}
	}
}
