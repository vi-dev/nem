package archive

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/oci"
)

type Dir struct{ root string }

func NewDir(root string) *Dir { return &Dir{root: root} }

func (s *Dir) dir(name string) (string, error) {
	if err := validName(name); err != nil {
		return "", err
	}
	return filepath.Join(s.root, "archives", name), nil
}

func (s *Dir) Open(_ context.Context, name string) (oras.ReadOnlyTarget, error) {
	dir, err := s.dir(name)
	if err != nil {
		return nil, err
	}
	if _, serr := os.Stat(filepath.Join(dir, "index.json")); os.IsNotExist(serr) {
		return nil, fmt.Errorf("no archives for %s in %s: %w", name, s.root, ErrNotFound)
	} else if serr != nil {
		return nil, serr
	}
	return oci.New(dir)
}

func (s *Dir) OpenRW(_ context.Context, name string) (oras.Target, error) {
	dir, err := s.dir(name)
	if err != nil {
		return nil, err
	}
	return oci.New(dir)
}

func (s *Dir) readIndex(name string) (ocispec.Index, bool) {
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

func (s *Dir) Has(name string) bool {
	idx, ok := s.readIndex(name)
	if !ok {
		return false
	}
	return len(idx.Manifests) > 0
}

func (s *Dir) HasVersion(name, version string) bool {
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
