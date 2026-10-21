package ocix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"

	"github.com/vi-dev/nem/internal/spec"
)

// Digests computed by the release before schema 4: a schema-2 or schema-3
// package must publish byte-identical manifests so existing locks stay valid.
func TestPackageManifestDigestUnchangedForSchema2And3(t *testing.T) {
	for _, tc := range []struct {
		files spec.Files
		want  string
	}{
		{spec.Files{Pkg: []byte("schema: 3\nname: a\nversions: [1.0.0]\n"), Recipes: map[string][]byte{"recipe@1.yaml": []byte("bins: [a]\n")}},
			"sha256:27fa7392c2960f262ef0d2dd971ba38b4c614f66752665834081c447a2be9edb"},
		{spec.Files{Pkg: []byte("schema: 2\nname: a\nversions: [1.0.0]\n")},
			"sha256:ddf19d2ef878602c16adeabc59ca60c1a61aef73b1c9fd8e3647ca70d5e2984b"},
	} {
		_, desc, err := PackageManifest(tc.files)
		if err != nil {
			t.Fatalf("PackageManifest: %v", err)
		}
		if desc.Digest.String() != tc.want {
			t.Errorf("%q: digest %s, want %s", tc.files.Pkg, desc.Digest, tc.want)
		}
	}
}

func TestPackageManifestSchema4UsesV4Layers(t *testing.T) {
	files := spec.Files{Pkg: []byte("schema: 4\nname: a\n"), Recipes: map[string][]byte{"recipe@1.yaml": []byte("bins: [a]\n")}}
	manBytes, _, err := PackageManifest(files)
	if err != nil {
		t.Fatalf("PackageManifest: %v", err)
	}
	var m ocispec.Manifest
	if err := json.Unmarshal(manBytes, &m); err != nil {
		t.Fatal(err)
	}
	if m.ArtifactType != MediaTypePkgV4 || m.Layers[0].MediaType != MediaTypePkgV4 || m.Layers[1].MediaType != MediaTypeRecipeV4 {
		t.Fatalf("schema 4 must publish v4 layers: %+v", m)
	}
}

func TestPackageManifestRefusesUnknownSchema(t *testing.T) {
	_, _, err := PackageManifest(spec.Files{Pkg: []byte("schema: 5\nname: a\n")})
	if _, ok := errors.AsType[*spec.UnsupportedSchemaError](err); !ok {
		t.Fatalf("schema 5: %v", err)
	}
	if _, _, err := PackageManifest(spec.Files{Pkg: []byte("name: a\n")}); err == nil {
		t.Fatal("a missing schema must be refused")
	}
}

func TestAssembleCatalogIndexCarriesHighestSchema(t *testing.T) {
	for _, tc := range []struct {
		schemas []int
		want    string
	}{
		{nil, "3"},
		{[]int{2, 3}, "3"},
		{[]int{3, 4, 2}, "4"},
	} {
		var entries []CatalogIndexEntry
		for _, s := range tc.schemas {
			entries = append(entries, CatalogIndexEntry{Title: "p", Schema: s})
		}
		b, _ := AssembleCatalogIndex(entries)
		var idx ocispec.Index
		_ = json.Unmarshal(b, &idx)
		if got := idx.Annotations[AnnotationSchemaVersion]; got != tc.want {
			t.Errorf("schemas %v: annotation %q, want %q", tc.schemas, got, tc.want)
		}
	}
}

func pushRawManifest(t *testing.T, pkg []byte, pkgType string, recipes map[string]string) (*memory.Store, ocispec.Descriptor) {
	t.Helper()
	ctx := context.Background()
	mem := memory.New()
	layer := content.NewDescriptorFromBytes(pkgType, pkg)
	layer.Annotations = map[string]string{AnnotationTitle: spec.PkgFileName}
	if err := PushBlobIfAbsent(ctx, mem, layer, bytes.NewReader(pkg)); err != nil {
		t.Fatal(err)
	}
	layers := []ocispec.Descriptor{layer}
	for title, mediaType := range recipes {
		data := []byte("bins: [a]\n")
		rd := content.NewDescriptorFromBytes(mediaType, data)
		rd.Annotations = map[string]string{AnnotationTitle: title}
		if err := PushBlobIfAbsent(ctx, mem, rd, bytes.NewReader(data)); err != nil {
			t.Fatal(err)
		}
		layers = append(layers, rd)
	}
	if err := PushEmptyConfig(ctx, mem); err != nil {
		t.Fatal(err)
	}
	man := ocispec.Manifest{SchemaVersion: OCISchemaVersion, MediaType: ocispec.MediaTypeImageManifest,
		ArtifactType: pkgType, Config: ocispec.DescriptorEmptyJSON, Layers: layers}
	manBytes, _ := json.Marshal(man)
	manDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, manBytes)
	if err := PushBlobIfAbsent(ctx, mem, manDesc, bytes.NewReader(manBytes)); err != nil {
		t.Fatal(err)
	}
	return mem, manDesc
}

func TestFetchFilesRoundTripsSchema4(t *testing.T) {
	ctx := context.Background()
	mem := memory.New()
	files := spec.Files{Pkg: []byte("schema: 4\nname: a\n"), Recipes: map[string][]byte{"recipe@1.yaml": []byte("bins: [a]\n")}}
	desc, err := PushPackageManifest(ctx, mem, files)
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	got, err := FetchFiles(ctx, mem, desc)
	if err != nil || len(got.Recipes) != 1 {
		t.Fatalf("FetchFiles: %+v, %v", got, err)
	}
}

func TestFetchFilesRejectsInconsistentLayers(t *testing.T) {
	for _, tc := range []struct {
		name    string
		pkg     string
		pkgType string
		recipes map[string]string
		want    string
	}{
		{"mixed recipe version", "schema: 4\nname: a\n", MediaTypePkgV4,
			map[string]string{"recipe@1.yaml": MediaTypeRecipeV3}, "recipe@1.yaml is recipe.v3 but pkg.yaml is pkg.v4"},
		{"v4 recipe under v3 pkg", "schema: 3\nname: a\n", MediaTypePkgV3,
			map[string]string{"recipe@1.yaml": MediaTypeRecipeV4}, "recipe@1.yaml is recipe.v4 but pkg.yaml is pkg.v3"},
		{"recipes under v2 pkg", "schema: 2\nname: a\n", MediaTypePkgV2,
			map[string]string{"recipe@1.yaml": MediaTypeRecipeV3}, "pkg.v2 manifests carry no recipe layers"},
		{"schema 4 in v3 layer", "schema: 4\nname: a\n", MediaTypePkgV3, nil,
			"pkg.yaml declares schema 4 but its layer is pkg.v3"},
		{"schema 3 in v4 layer", "schema: 3\nname: a\n", MediaTypePkgV4, nil,
			"pkg.yaml declares schema 3 but its layer is pkg.v4"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mem, desc := pushRawManifest(t, []byte(tc.pkg), tc.pkgType, tc.recipes)
			_, err := FetchFiles(context.Background(), mem, desc)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestFetchFilesAcceptsSchema2InV3Layer(t *testing.T) {
	mem, desc := pushRawManifest(t, []byte("schema: 2\nname: a\n"), MediaTypePkgV3, nil)
	if _, err := FetchFiles(context.Background(), mem, desc); err != nil {
		t.Fatalf("schema 2 has always been published as pkg.v3: %v", err)
	}
}

// A client capped at schema 3 reading a catalog that contains a schema-4
// package must be refused at the index, before any package is copied.
func TestOldClientRefusesSchema4Index(t *testing.T) {
	b, _ := AssembleCatalogIndex([]CatalogIndexEntry{{Title: "a", Schema: 4}})
	var idx ocispec.Index
	_ = json.Unmarshal(b, &idx)
	got := idx.Annotations[AnnotationSchemaVersion]
	if got == "2" || got == "3" {
		t.Fatalf("index annotation %q would be accepted by a schema-3 client", got)
	}
}
