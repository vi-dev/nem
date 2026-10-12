package ocix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/errdef"

	"github.com/vi-dev/nem/internal/spec"
)

type TitledManifest struct {
	Title string
	Desc  ocispec.Descriptor
}

func enumerateTitledManifests(idx ocispec.Index) []TitledManifest {
	seen := make(map[string]bool, len(idx.Manifests))
	out := make([]TitledManifest, 0, len(idx.Manifests))
	for _, m := range idx.Manifests {
		title := m.Annotations[AnnotationTitle]
		if title == "" || seen[title] {
			continue
		}
		seen[title] = true
		out = append(out, TitledManifest{Title: title, Desc: m})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out
}

type Store struct {
	target oras.ReadOnlyTarget
	tag    string
	idx    ocispec.Index
	digest string
	byName map[string]ocispec.Descriptor
}

func newStore(target oras.ReadOnlyTarget, tag string, idx ocispec.Index, digest string) *Store {
	byName := make(map[string]ocispec.Descriptor, len(idx.Manifests))
	for _, tm := range enumerateTitledManifests(idx) {
		byName[tm.Title] = tm.Desc
	}
	return &Store{target: target, tag: tag, idx: idx, digest: digest, byName: byName}
}

func (s *Store) Index() ocispec.Index { return s.idx }

func (s *Store) Digest() string { return s.digest }

func (s *Store) Packages() []TitledManifest { return enumerateTitledManifests(s.idx) }

func (s *Store) Manifest(name string) (ocispec.Descriptor, bool) {
	m, ok := s.byName[name]
	return m, ok
}

func (s *Store) Files(ctx context.Context, name string) (spec.Files, string, error) {
	m, ok := s.byName[name]
	if !ok {
		return spec.Files{}, "", &PkgNotInIndexError{Name: name}
	}
	files, err := FetchFiles(ctx, s.target, m)
	if err != nil {
		return spec.Files{}, "", fmt.Errorf("load package files for %s: %w", name, err)
	}
	return files, m.Digest.String(), nil
}

func (s *Store) CopyTo(ctx context.Context, dst oras.Target, dstRef string, progress ProgressFunc) (ocispec.Descriptor, error) {
	return CopyIndexClosureWithProgress(ctx, s.target, s.tag, dst, dstRef, progress)
}

type localTarget struct {
	*oci.ReadOnlyStorage
	tags map[string]ocispec.Descriptor
}

var _ oras.ReadOnlyTarget = (*localTarget)(nil)

func (t *localTarget) Resolve(_ context.Context, ref string) (ocispec.Descriptor, error) {
	if d, ok := t.tags[ref]; ok {
		return d, nil
	}
	return ocispec.Descriptor{}, errdef.ErrNotFound
}

func openLocalTarget(storePath string) (*localTarget, error) {
	data, err := os.ReadFile(filepath.Join(storePath, ocispec.ImageIndexFile))
	if os.IsNotExist(err) {
		return nil, ErrNotUpdated
	}
	if err != nil {
		return nil, fmt.Errorf("open catalog store %s: %w", storePath, err)
	}
	var layout ocispec.Index
	if err := json.Unmarshal(data, &layout); err != nil {
		return nil, fmt.Errorf("open catalog store %s: parse index.json: %w", storePath, err)
	}
	storage := oci.NewStorageFromFS(os.DirFS(storePath))
	tags := make(map[string]ocispec.Descriptor)
	for _, m := range layout.Manifests {
		if ref := m.Annotations[ocispec.AnnotationRefName]; ref != "" {
			tags[ref] = m
		}
	}
	return &localTarget{ReadOnlyStorage: storage, tags: tags}, nil
}

func LastUpdated(storePath string) (time.Time, error) {
	fi, err := os.Stat(filepath.Join(storePath, ocispec.ImageIndexFile))
	if err != nil {
		return time.Time{}, err
	}
	return fi.ModTime(), nil
}

func OpenLocalStore(ctx context.Context, storePath string) (*Store, error) {
	target, err := openLocalTarget(storePath)
	if err != nil {
		return nil, err
	}
	idx, desc, err := FetchCatalogIndex(ctx, target, LocalTag)
	if err != nil {
		if errors.Is(err, errdef.ErrNotFound) {
			return nil, ErrNotUpdated
		}
		return nil, err
	}
	return newStore(target, LocalTag, idx, desc.Digest.String()), nil
}

func OpenStore(ctx context.Context, src oras.ReadOnlyTarget, ref string) (*Store, error) {
	idx, desc, err := FetchCatalogIndex(ctx, src, ref)
	if err != nil {
		return nil, err
	}
	return newStore(src, ref, idx, desc.Digest.String()), nil
}

func OpenStoreInMemory(ctx context.Context, src oras.ReadOnlyTarget, ref string, progress ProgressFunc) (*Store, error) {
	mem := memory.New()
	if _, err := CopyIndexClosureWithProgress(ctx, src, ref, mem, LocalTag, progress); err != nil {
		return nil, err
	}
	return OpenStore(ctx, mem, LocalTag)
}

func UpdateLocalCatalog(ctx context.Context, src oras.ReadOnlyTarget, srcRef, storePath string, progress ProgressFunc) (*Store, error) {
	_, srcDesc, err := FetchCatalogIndex(ctx, src, srcRef)
	if err != nil {
		return nil, err
	}

	dst, err := oci.New(storePath)
	if err != nil {
		return nil, fmt.Errorf("open catalog store %s: %w", storePath, err)
	}
	destDesc, err := CopyIndexClosureWithProgress(ctx, src, srcRef, dst, LocalTag, progress)
	if err != nil {
		return nil, fmt.Errorf("update catalog: %w", err)
	}
	if destDesc.Digest != srcDesc.Digest {
		return nil, errors.New("catalog changed during update; retry")
	}
	return OpenLocalStore(ctx, storePath)
}

func FetchFiles(ctx context.Context, src oras.ReadOnlyTarget, man ocispec.Descriptor) (spec.Files, error) {
	manData, err := content.FetchAll(ctx, src, man)
	if err != nil {
		return spec.Files{}, fmt.Errorf("read manifest: %w", err)
	}
	var m ocispec.Manifest
	if err := json.Unmarshal(manData, &m); err != nil {
		return spec.Files{}, fmt.Errorf("parse manifest: %w", err)
	}
	var files spec.Files
	for _, layer := range m.Layers {
		switch layer.MediaType {
		case MediaTypePkg, MediaTypePkgV2:
			data, err := content.FetchAll(ctx, src, layer)
			if err != nil {
				return spec.Files{}, fmt.Errorf("read %s layer: %w", spec.PkgFileName, err)
			}
			files.Pkg = data
		case MediaTypeRecipe:
			title := layer.Annotations[AnnotationTitle]
			if _, ok := spec.ParseRecipeFileName(title); !ok {
				return spec.Files{}, fmt.Errorf("recipe layer %s: title %q must be recipe@<prefix>.yaml", layer.Digest, title)
			}
			if _, dup := files.Recipes[title]; dup {
				return spec.Files{}, fmt.Errorf("recipe layer %s: duplicate title %q", layer.Digest, title)
			}
			data, err := content.FetchAll(ctx, src, layer)
			if err != nil {
				return spec.Files{}, fmt.Errorf("read %s layer: %w", title, err)
			}
			if files.Recipes == nil {
				files.Recipes = map[string][]byte{}
			}
			files.Recipes[title] = data
		}
	}
	if files.Pkg == nil {
		return spec.Files{}, errors.New("manifest has no pkg.yaml layer")
	}
	return files, nil
}
