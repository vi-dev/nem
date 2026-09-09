package ocix_test

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/oci"

	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/ocix/ocixtest"
)

func TestAssembleIndexAnnotationsAndOrder(t *testing.T) {
	mk := func(name string) ocispec.Descriptor {
		return content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, []byte(name))
	}
	entries := []ocix.CatalogIndexEntry{
		{Manifest: mk("kubectl"), Title: "kubectl", Version: "v1.31.0"},
		{Manifest: mk("go"), Title: "go", Description: "The Go language", Version: "v1.26.5"},
	}
	idxBytes, idxDesc := ocix.AssembleCatalogIndex(entries)

	var idx ocispec.Index
	if err := json.Unmarshal(idxBytes, &idx); err != nil {
		t.Fatal(err)
	}
	if idx.MediaType != ocispec.MediaTypeImageIndex {
		t.Fatalf("index mediaType = %q", idx.MediaType)
	}
	if idx.Annotations[ocix.AnnotationSchemaVersion] != ocix.SchemaVersion {
		t.Fatalf("schemaVersion annotation = %q", idx.Annotations[ocix.AnnotationSchemaVersion])
	}

	if idx.Manifests[0].Annotations[ocix.AnnotationTitle] != "go" ||
		idx.Manifests[1].Annotations[ocix.AnnotationTitle] != "kubectl" {
		t.Fatalf("entries not sorted by title: %v", idx.Manifests)
	}
	if idx.Manifests[0].Annotations[ocix.AnnotationVersion] != "v1.26.5" {
		t.Fatalf("version annotation missing on go")
	}
	if idx.Manifests[0].Annotations[ocix.AnnotationDescription] != "The Go language" {
		t.Fatalf("description annotation missing on go")
	}

	if _, ok := idx.Manifests[1].Annotations[ocix.AnnotationDescription]; ok {
		t.Fatalf("empty description must be omitted")
	}
	if idxDesc.MediaType != ocispec.MediaTypeImageIndex {
		t.Fatalf("index desc mediaType = %q", idxDesc.MediaType)
	}
}

func decodeJSON(r io.Reader, v any) error {
	return json.NewDecoder(r).Decode(v)
}

func TestPushPackageManifestDeterministicAndShaped(t *testing.T) {
	ctx := context.Background()
	pkgBytes := []byte("schema: 2\nname: go\n")

	newStore := func() *oci.Store {
		s, err := oci.New(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	s1 := newStore()
	if err := ocix.PushEmptyConfig(ctx, s1); err != nil {
		t.Fatalf("PushEmptyConfig: %v", err)
	}
	d1, err := ocix.PushPackageManifest(ctx, s1, pkgBytes)
	if err != nil {
		t.Fatalf("PushPackageManifest: %v", err)
	}

	s2 := newStore()
	if err := ocix.PushEmptyConfig(ctx, s2); err != nil {
		t.Fatal(err)
	}
	d2, err := ocix.PushPackageManifest(ctx, s2, pkgBytes)
	if err != nil {
		t.Fatal(err)
	}
	if d1.Digest != d2.Digest {
		t.Fatalf("manifest digest not deterministic: %s vs %s", d1.Digest, d2.Digest)
	}
	if d1.MediaType != ocispec.MediaTypeImageManifest {
		t.Fatalf("manifest mediaType = %q", d1.MediaType)
	}
	if len(d1.Annotations) != 0 {
		t.Fatalf("manifest descriptor should carry no annotations, got %v", d1.Annotations)
	}

	rc, err := s1.Fetch(ctx, d1)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	var m ocispec.Manifest
	if err := decodeJSON(rc, &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Annotations) != 0 {
		t.Fatalf("manifest JSON must be annotation-free, got %v", m.Annotations)
	}
	if m.SchemaVersion != 2 {
		t.Fatalf("manifest schemaVersion = %d, want 2", m.SchemaVersion)
	}
	if m.ArtifactType != ocix.MediaTypePkg {
		t.Fatalf("artifactType = %q, want %q", m.ArtifactType, ocix.MediaTypePkg)
	}
	if len(m.Layers) != 1 || m.Layers[0].MediaType != ocix.MediaTypePkg {
		t.Fatalf("layers = %+v", m.Layers)
	}
}

func TestPackageManifestDescriptorMatchesPush(t *testing.T) {
	ctx := context.Background()
	pkgBytes := []byte("schema: 2\nname: helm\n")
	s, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := ocix.PushEmptyConfig(ctx, s); err != nil {
		t.Fatal(err)
	}
	pushed, err := ocix.PushPackageManifest(ctx, s, pkgBytes)
	if err != nil {
		t.Fatal(err)
	}
	_, computed, err := ocix.PackageManifest(pkgBytes)
	if err != nil {
		t.Fatal(err)
	}
	if pushed.Digest != computed.Digest {
		t.Fatalf("PackageManifestDescriptor digest = %s, want %s", computed.Digest, pushed.Digest)
	}
}

func TestFetchIndexReturnsParsedIndex(t *testing.T) {
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ocixtest.PushFakeCatalog(t, store, []ocixtest.FakeEntry{
		{Name: "atool", Description: "A tool", Latest: "1.0.0", YAML: []byte("name: atool")},
	}, ocix.SchemaVersion)

	idx, desc, err := ocix.FetchCatalogIndex(context.Background(), store, "v2")
	if err != nil {
		t.Fatalf("FetchIndex: %v", err)
	}
	if len(idx.Manifests) != 1 {
		t.Fatalf("manifests = %d, want 1", len(idx.Manifests))
	}
	if got := idx.Manifests[0].Annotations[ocix.AnnotationTitle]; got != "atool" {
		t.Fatalf("title = %q, want atool", got)
	}
	if desc.Digest == "" {
		t.Fatal("descriptor digest is empty")
	}
}

func TestFetchIndexRejectsWrongSchema(t *testing.T) {
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ocixtest.PushFakeCatalog(t, store, nil, "999")

	if _, _, err := ocix.FetchCatalogIndex(context.Background(), store, "v2"); err == nil {
		t.Fatal("FetchIndex accepted schema 999, want error")
	}
}
