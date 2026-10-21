package ocix

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"

	"github.com/vi-dev/nem/internal/spec"
)

func TestPackageManifestCarriesRecipeLayers(t *testing.T) {
	files := spec.Files{Pkg: []byte("schema: 3\nname: a\n"), Recipes: map[string][]byte{
		"recipe@2.yaml": []byte("bins: [b]\n"), "recipe@1.yaml": []byte("bins: [a]\n"),
	}}
	manBytes, desc, err := PackageManifest(files)
	if err != nil {
		t.Fatalf("PackageManifest: %v", err)
	}
	var m ocispec.Manifest
	if err := json.Unmarshal(manBytes, &m); err != nil {
		t.Fatal(err)
	}
	if m.ArtifactType != MediaTypePkgV3 || len(m.Layers) != 3 {
		t.Fatalf("manifest: %+v", m)
	}
	if m.Layers[0].MediaType != MediaTypePkgV3 || m.Layers[0].Annotations[AnnotationTitle] != "pkg.yaml" {
		t.Fatalf("layer 0: %+v", m.Layers[0])
	}
	if m.Layers[1].Annotations[AnnotationTitle] != "recipe@1.yaml" || m.Layers[2].Annotations[AnnotationTitle] != "recipe@2.yaml" ||
		m.Layers[1].MediaType != MediaTypeRecipeV3 {
		t.Fatalf("recipe layers must be sorted and typed: %+v", m.Layers[1:])
	}
	_, again, _ := PackageManifest(files)
	if again.Digest != desc.Digest {
		t.Fatal("manifest digest must be deterministic")
	}
	_, noRecipe, _ := PackageManifest(spec.Files{Pkg: files.Pkg})
	if noRecipe.Digest == desc.Digest {
		t.Fatal("a recipe layer must change the package digest")
	}
}

func TestCatalogIndexSchemaAcceptsV2ToV4(t *testing.T) {
	for _, s := range []string{"2", "3", "4"} {
		idx := ocispec.Index{Annotations: map[string]string{AnnotationSchemaVersion: s}}
		if err := validateCatalogIndexSchema(idx); err != nil {
			t.Fatalf("schema %s: %v", s, err)
		}
	}
	for _, s := range []string{"5", "", "x"} {
		idx := ocispec.Index{Annotations: map[string]string{AnnotationSchemaVersion: s}}
		if err := validateCatalogIndexSchema(idx); err == nil || !strings.Contains(err.Error(), "a newer nem may be required") {
			t.Fatalf("schema %q: %v", s, err)
		}
	}
	b, _ := AssembleCatalogIndex(nil)
	var out ocispec.Index
	_ = json.Unmarshal(b, &out)
	if out.Annotations[AnnotationSchemaVersion] != "3" || out.SchemaVersion != OCISchemaVersion {
		t.Fatalf("assembled index: %+v", out)
	}
}

func TestFetchFilesReturnsPkgAndRecipes(t *testing.T) {
	ctx := context.Background()
	mem := memory.New()
	files := spec.Files{Pkg: []byte("schema: 3\nname: a\n"), Recipes: map[string][]byte{"recipe@1.yaml": []byte("bins: [a]\n")}}
	desc, err := PushPackageManifest(ctx, mem, files)
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	got, err := FetchFiles(ctx, mem, desc)
	if err != nil {
		t.Fatalf("FetchFiles: %v", err)
	}
	if string(got.Pkg) != string(files.Pkg) || string(got.Recipes["recipe@1.yaml"]) != "bins: [a]\n" {
		t.Fatalf("files: %+v", got)
	}
}

func TestFetchFilesAcceptsV2PkgLayer(t *testing.T) {
	ctx := context.Background()
	mem := memory.New()
	pkg := []byte("schema: 2\nname: a\n")
	layer := content.NewDescriptorFromBytes(MediaTypePkgV2, pkg)
	if err := PushBlobIfAbsent(ctx, mem, layer, bytes.NewReader(pkg)); err != nil {
		t.Fatal(err)
	}
	if err := PushEmptyConfig(ctx, mem); err != nil {
		t.Fatal(err)
	}
	man := ocispec.Manifest{SchemaVersion: OCISchemaVersion, MediaType: ocispec.MediaTypeImageManifest,
		ArtifactType: MediaTypePkgV2, Config: ocispec.DescriptorEmptyJSON, Layers: []ocispec.Descriptor{layer}}
	manBytes, _ := json.Marshal(man)
	manDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, manBytes)
	if err := PushBlobIfAbsent(ctx, mem, manDesc, bytes.NewReader(manBytes)); err != nil {
		t.Fatal(err)
	}
	got, err := FetchFiles(ctx, mem, manDesc)
	if err != nil || string(got.Pkg) != string(pkg) || len(got.Recipes) != 0 {
		t.Fatalf("v2 layer: %+v, %v", got, err)
	}
}
