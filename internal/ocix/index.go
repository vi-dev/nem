package ocix

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"

	"github.com/vi-dev/nem/internal/spec"
)

type CatalogIndexEntry struct {
	Manifest    ocispec.Descriptor
	Title       string
	Description string
	Version     string
	Schema      int
}

func AssembleCatalogIndex(entries []CatalogIndexEntry) ([]byte, ocispec.Descriptor) {
	sorted := append([]CatalogIndexEntry(nil), entries...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Title < sorted[j].Title })

	schema := minIndexSchema
	mans := make([]ocispec.Descriptor, len(sorted))
	for i, e := range sorted {
		schema = max(schema, e.Schema)
		d := e.Manifest
		ann := map[string]string{AnnotationTitle: e.Title}
		if e.Description != "" {
			ann[AnnotationDescription] = e.Description
		}
		if e.Version != "" {
			ann[AnnotationVersion] = e.Version
		}
		d.Annotations = ann
		mans[i] = d
	}
	idx := ocispec.Index{
		SchemaVersion: OCISchemaVersion,
		MediaType:     ocispec.MediaTypeImageIndex,
		Manifests:     mans,
		Annotations: map[string]string{
			AnnotationSchemaVersion: strconv.Itoa(schema),
		},
	}
	b, _ := json.Marshal(idx)
	return b, content.NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, b)
}

func FetchCatalogIndex(ctx context.Context, src oras.ReadOnlyTarget, ref string) (ocispec.Index, ocispec.Descriptor, error) {
	var idx ocispec.Index
	desc, err := src.Resolve(ctx, ref)
	if err != nil {
		return idx, ocispec.Descriptor{}, fmt.Errorf("resolve catalog ref %s: %w", ref, err)
	}
	data, err := content.FetchAll(ctx, src, desc)
	if err != nil {
		return idx, ocispec.Descriptor{}, fmt.Errorf("read catalog index: %w", err)
	}
	if err := json.Unmarshal(data, &idx); err != nil {
		return idx, ocispec.Descriptor{}, fmt.Errorf("parse catalog index: %w", err)
	}
	if err := validateCatalogIndexSchema(idx); err != nil {
		return idx, ocispec.Descriptor{}, err
	}
	return idx, desc, nil
}

func validateCatalogIndexSchema(idx ocispec.Index) error {
	got := idx.Annotations[AnnotationSchemaVersion]
	if n, err := strconv.Atoi(got); err != nil || n < spec.MinSchema || n > spec.MaxSchema {
		return fmt.Errorf("unsupported catalog schema %q (want at most %d; a newer nem may be required)", got, spec.MaxSchema)
	}
	return nil
}

func packageLayers(files spec.Files) (string, []ocispec.Descriptor, error) {
	schema, _ := spec.PeekSchema(files.Pkg)
	pkgType, recipeType, err := MediaTypes(schema)
	if err != nil {
		return "", nil, err
	}
	layers := []ocispec.Descriptor{titled(content.NewDescriptorFromBytes(pkgType, files.Pkg), spec.PkgFileName)}
	for _, name := range files.Names() {
		layers = append(layers, titled(content.NewDescriptorFromBytes(recipeType, files.Recipes[name]), name))
	}
	return pkgType, layers, nil
}

func titled(d ocispec.Descriptor, title string) ocispec.Descriptor {
	d.Annotations = map[string]string{AnnotationTitle: title}
	return d
}

func PackageManifest(files spec.Files) ([]byte, ocispec.Descriptor, error) {
	pkgType, layers, err := packageLayers(files)
	if err != nil {
		return nil, ocispec.Descriptor{}, err
	}
	man := ocispec.Manifest{
		SchemaVersion: OCISchemaVersion,
		MediaType:     ocispec.MediaTypeImageManifest,
		ArtifactType:  pkgType,
		Config:        ocispec.DescriptorEmptyJSON,
		Layers:        layers,
	}
	manBytes, err := json.Marshal(man)
	if err != nil {
		return nil, ocispec.Descriptor{}, err
	}
	return manBytes, content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, manBytes), nil
}

func PushPackageManifest(ctx context.Context, target oras.Target, files spec.Files) (ocispec.Descriptor, error) {
	blobs := map[string][]byte{spec.PkgFileName: files.Pkg}
	for name, b := range files.Recipes {
		blobs[name] = b
	}
	_, layers, err := packageLayers(files)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	for _, layer := range layers {
		data := blobs[layer.Annotations[AnnotationTitle]]
		if err := PushBlobIfAbsent(ctx, target, layer, bytes.NewReader(data)); err != nil {
			return ocispec.Descriptor{}, err
		}
	}
	if err := PushEmptyConfig(ctx, target); err != nil {
		return ocispec.Descriptor{}, err
	}
	manBytes, manDesc, err := PackageManifest(files)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	if err := PushBlobIfAbsent(ctx, target, manDesc, bytes.NewReader(manBytes)); err != nil {
		return ocispec.Descriptor{}, err
	}
	return manDesc, nil
}
