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

func TestUpdateAndLoad(t *testing.T) {
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
	s, err := ocix.UpdateLocalCatalog(ctx, src, "v2", storePath, nil)
	if err != nil {
		t.Fatalf("UpdateLocalCatalog: %v", err)
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

	files, mdig, err := s.Files(ctx, "go")
	if err != nil || string(files.Pkg) != string(goYAML) || !strings.HasPrefix(mdig, "sha256:") {
		t.Fatalf("Files: %q, %q, %v", files.Pkg, mdig, err)
	}

	var nf *ocix.PkgNotInIndexError
	if _, _, err := s.Files(ctx, "absent"); !errors.As(err, &nf) {
		t.Fatalf("want PkgNotInIndexError, got %v", err)
	}
}

func TestUpdateRejectsWrongSchema(t *testing.T) {
	ctx := context.Background()
	storePath := filepath.Join(t.TempDir(), "store")

	good, _ := oci.New(t.TempDir())
	ocixtest.PushFakeCatalog(t, good, []ocixtest.FakeEntry{{Name: "go", YAML: []byte("x")}}, "2")
	if _, err := ocix.UpdateLocalCatalog(ctx, good, "v2", storePath, nil); err != nil {
		t.Fatalf("seed update: %v", err)
	}

	bad, _ := oci.New(t.TempDir())
	ocixtest.PushFakeCatalog(t, bad, []ocixtest.FakeEntry{{Name: "go", YAML: []byte("x")}}, "9")
	_, err := ocix.UpdateLocalCatalog(ctx, bad, "v2", storePath, nil)
	if err == nil || !strings.Contains(err.Error(), "schema") {
		t.Fatalf("want schema error, got %v", err)
	}

	s, err := ocix.OpenLocalStore(ctx, storePath)
	if err != nil || len(s.Index().Manifests) != 1 {
		t.Fatalf("good mirror clobbered by bad-schema update attempt: %v", err)
	}
}

func TestOpenStoreNeverUpdated(t *testing.T) {
	_, err := ocix.OpenLocalStore(context.Background(), filepath.Join(t.TempDir(), "nope"))
	if !errors.Is(err, ocix.ErrNotUpdated) {
		t.Fatalf("want ErrNotUpdated, got %v", err)
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

func TestUpdateDetectsMovingTag(t *testing.T) {
	ctx := context.Background()
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatalf("oci.New: %v", err)
	}
	oldDesc := ocixtest.PushFakeCatalog(t, store, []ocixtest.FakeEntry{{Name: "go", YAML: []byte("old")}}, "3")
	newDesc := ocixtest.PushFakeCatalog(t, store, []ocixtest.FakeEntry{{Name: "go", YAML: []byte("new")}}, "3")
	if oldDesc.Digest == newDesc.Digest {
		t.Fatal("fixture bug: old and new catalog content must digest differently")
	}

	moving := &movingResolveTarget{ReadOnlyTarget: store, ref: "v2", first: oldDesc, rest: newDesc}
	storePath := filepath.Join(t.TempDir(), "store")
	_, err = ocix.UpdateLocalCatalog(ctx, moving, "v2", storePath, nil)
	if err == nil || !strings.Contains(err.Error(), "catalog changed during update; retry") {
		t.Fatalf("UpdateLocalCatalog error = %v, want a catalog-changed-during-update error", err)
	}
}

func TestUpdateIsIdempotent(t *testing.T) {
	ctx := context.Background()
	src, _ := oci.New(t.TempDir())
	ocixtest.PushFakeCatalog(t, src, []ocixtest.FakeEntry{{Name: "go", YAML: []byte("y")}}, "2")
	storePath := filepath.Join(t.TempDir(), "store")
	s1, err := ocix.UpdateLocalCatalog(ctx, src, "v2", storePath, nil)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := ocix.UpdateLocalCatalog(ctx, src, "v2", storePath, nil)
	if err != nil || s1.Digest() != s2.Digest() {
		t.Fatalf("second update: %q vs %q, %v", s1.Digest(), s2.Digest(), err)
	}
}

func TestFetchFilesReturnsPkgLayer(t *testing.T) {
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	yaml := []byte("name: atool\n")
	ocixtest.PushFakeCatalog(t, store, []ocixtest.FakeEntry{
		{Name: "atool", Latest: "1.0.0", YAML: yaml},
	}, "3")
	idx, _, err := ocix.FetchCatalogIndex(context.Background(), store, "v2")
	if err != nil {
		t.Fatal(err)
	}

	files, err := ocix.FetchFiles(context.Background(), store, idx.Manifests[0])
	if err != nil {
		t.Fatalf("FetchFiles: %v", err)
	}
	got := files.Pkg
	if !bytes.Equal(got, yaml) {
		t.Fatalf("bytes = %q, want %q", got, yaml)
	}
}

func TestFetchFilesRejectsManifestWithoutPkgLayer(t *testing.T) {
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

	if _, err := ocix.FetchFiles(context.Background(), store, archIdx.Manifests[0]); err == nil {
		t.Fatal("FetchFiles accepted a manifest without a pkg.yaml layer, want error")
	}
}

func TestStageIndexClosureStable(t *testing.T) {
	ctx := context.Background()
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatalf("oci.New: %v", err)
	}
	desc := ocixtest.PushFakeCatalog(t, store, []ocixtest.FakeEntry{{Name: "go", YAML: []byte("y")}}, "3")

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
	if !errors.Is(err, ocix.ErrNotUpdated) {
		t.Fatalf("want ErrNotUpdated, got %v", err)
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
	ocixtest.PushFakeCatalog(b, src, fakeCatalogEntries(300), "3")
	storePath := filepath.Join(b.TempDir(), "store")
	if _, err := ocix.UpdateLocalCatalog(ctx, src, "v2", storePath, nil); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		if _, err := ocix.OpenLocalStore(ctx, storePath); err != nil {
			b.Fatal(err)
		}
	}
}
