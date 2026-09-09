package ocixtest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/errdef"

	"github.com/vi-dev/nem/internal/ocix"
)

type FakeEntry struct {
	Name, Description, Latest string
	YAML                      []byte
}

func PushFakeCatalog(t testing.TB, store oras.Target, entries []FakeEntry, schemaVersion string) ocispec.Descriptor {
	t.Helper()
	desc, err := pushFakeCatalog(store, entries, schemaVersion)
	if err != nil {
		t.Fatalf("push fake catalog: %v", err)
	}
	return desc
}

func PushFakeArchive(t testing.TB, store oras.Target, tag string, platforms map[string][]byte) {
	t.Helper()
	if err := pushFakeArchive(store, tag, platforms); err != nil {
		t.Fatalf("push fake archive: %v", err)
	}
}

func pushFakeCatalog(store oras.Target, entries []FakeEntry, schemaVersion string) (ocispec.Descriptor, error) {
	ctx := context.Background()

	emptyConfig := ocispec.DescriptorEmptyJSON
	if err := pushFakeBlob(ctx, store, emptyConfig, []byte("{}")); err != nil {
		return ocispec.Descriptor{}, err
	}

	manifests := make([]ocispec.Descriptor, 0, len(entries))
	for _, e := range entries {
		layerDesc := content.NewDescriptorFromBytes(ocix.MediaTypePkg, e.YAML)
		if err := pushFakeBlob(ctx, store, layerDesc, e.YAML); err != nil {
			return ocispec.Descriptor{}, err
		}

		manifest := ocispec.Manifest{
			SchemaVersion: 2,
			MediaType:     ocispec.MediaTypeImageManifest,
			ArtifactType:  ocix.MediaTypePkg,
			Config:        emptyConfig,
			Layers:        []ocispec.Descriptor{layerDesc},
		}
		manifestBytes, err := json.Marshal(manifest)
		if err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("marshal manifest for %s: %w", e.Name, err)
		}
		manifestDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, manifestBytes)
		if err := pushFakeBlob(ctx, store, manifestDesc, manifestBytes); err != nil {
			return ocispec.Descriptor{}, err
		}

		manifestDesc.ArtifactType = ocix.MediaTypePkg
		manifestDesc.Annotations = map[string]string{
			ocix.AnnotationTitle:       e.Name,
			ocix.AnnotationDescription: e.Description,
			ocix.AnnotationVersion:     e.Latest,
		}
		manifests = append(manifests, manifestDesc)
	}

	idx := ocispec.Index{
		SchemaVersion: 2,
		MediaType:     ocispec.MediaTypeImageIndex,
		Manifests:     manifests,
		Annotations: map[string]string{
			ocix.AnnotationSchemaVersion: schemaVersion,
		},
	}
	idxBytes, err := json.Marshal(idx)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("marshal index: %w", err)
	}
	idxDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, idxBytes)
	if err := pushFakeBlob(ctx, store, idxDesc, idxBytes); err != nil {
		return ocispec.Descriptor{}, err
	}

	if err := store.Tag(ctx, idxDesc, "v2"); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("tag fake catalog index: %w", err)
	}
	return idxDesc, nil
}

func pushFakeArchive(store oras.Target, tag string, platforms map[string][]byte) error {
	ctx := context.Background()

	emptyConfig := ocispec.DescriptorEmptyJSON
	if err := pushFakeBlob(ctx, store, emptyConfig, []byte("{}")); err != nil {
		return err
	}

	keys := make([]string, 0, len(platforms))
	for k := range platforms {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	manifests := make([]ocispec.Descriptor, 0, len(keys))
	for _, k := range keys {
		osName, arch, _ := strings.Cut(k, "/")
		payload := platforms[k]

		layerDesc := content.NewDescriptorFromBytes(ocix.MediaTypeArchive, payload)
		if err := pushFakeBlob(ctx, store, layerDesc, payload); err != nil {
			return err
		}

		manifest := ocispec.Manifest{
			SchemaVersion: 2,
			MediaType:     ocispec.MediaTypeImageManifest,
			Config:        emptyConfig,
			Layers:        []ocispec.Descriptor{layerDesc},
		}
		manifestBytes, err := json.Marshal(manifest)
		if err != nil {
			return fmt.Errorf("marshal archive manifest for %s: %w", k, err)
		}
		manifestDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, manifestBytes)
		if err := pushFakeBlob(ctx, store, manifestDesc, manifestBytes); err != nil {
			return err
		}

		manifestDesc.Platform = &ocispec.Platform{OS: osName, Architecture: arch}
		manifests = append(manifests, manifestDesc)
	}

	idx := ocispec.Index{
		SchemaVersion: 2,
		MediaType:     ocispec.MediaTypeImageIndex,
		Manifests:     manifests,
	}
	idxBytes, err := json.Marshal(idx)
	if err != nil {
		return fmt.Errorf("marshal archive index: %w", err)
	}
	idxDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, idxBytes)
	if err := pushFakeBlob(ctx, store, idxDesc, idxBytes); err != nil {
		return err
	}

	if err := store.Tag(ctx, idxDesc, tag); err != nil {
		return fmt.Errorf("tag fake archive index: %w", err)
	}
	return nil
}

func pushFakeBlob(ctx context.Context, store oras.Target, desc ocispec.Descriptor, data []byte) error {
	if err := store.Push(ctx, desc, bytes.NewReader(data)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return fmt.Errorf("push fake blob %s: %w", desc.Digest, err)
	}
	return nil
}
