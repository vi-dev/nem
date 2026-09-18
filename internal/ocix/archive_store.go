package ocix

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/oci"

	"github.com/vi-dev/nem/internal/spec"
)

type ArchiveStore struct{ root string }

func NewArchiveStore(root string) *ArchiveStore { return &ArchiveStore{root: root} }

func (s *ArchiveStore) Root() string { return s.root }

func (s *ArchiveStore) dir(name string) (string, error) {
	if name == "" || name == "." || name == ".." ||
		strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("invalid package name %q for archive store", name)
	}
	return filepath.Join(s.root, "archives", name), nil
}

func (s *ArchiveStore) Open(name string) (oras.Target, error) {
	dir, err := s.dir(name)
	if err != nil {
		return nil, err
	}
	return oci.New(dir)
}

func (s *ArchiveStore) readIndex(name string) (ocispec.Index, bool) {
	var idx ocispec.Index
	dir, err := s.dir(name)
	if err != nil {
		return idx, false
	}
	data, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		return idx, false
	}
	if err := json.Unmarshal(data, &idx); err != nil {
		return idx, false
	}
	return idx, true
}

func (s *ArchiveStore) Has(name string) bool {
	idx, ok := s.readIndex(name)
	if !ok {
		return false
	}
	return len(idx.Manifests) > 0
}

func (s *ArchiveStore) HasTag(name, version string) bool {
	idx, ok := s.readIndex(name)
	if !ok {
		return false
	}
	for _, m := range idx.Manifests {
		if m.Annotations[ocispec.AnnotationRefName] == version {
			return true
		}
	}
	return false
}

func ReadArchive(ctx context.Context, src oras.ReadOnlyTarget, tag string, plat spec.Platform) ([]byte, error) {
	layer, err := resolveArchiveLayer(ctx, src, tag, plat)
	if err != nil {
		return nil, err
	}
	data, err := content.FetchAll(ctx, src, layer)
	if err != nil {
		return nil, fmt.Errorf("read archive layer: %w", err)
	}
	return data, nil
}
