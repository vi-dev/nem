package ocix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/errdef"

	"github.com/vi-dev/nem/internal/spec"
)

func RemoteArchivesRW(catalogRef, name string) (oras.Target, error) {
	archivesRef, err := ArchivesRef(catalogRef, name)
	if err != nil {
		return nil, err
	}
	return NewRemoteRepository(archivesRef)
}

func PushArchive(ctx context.Context, target oras.Target, tag string, plat spec.Platform, archiveBytes []byte, force bool) (ocispec.Descriptor, bool, error) {
	layerDesc := content.NewDescriptorFromBytes(MediaTypeArchive, archiveBytes)
	manBytes, manDesc, err := archiveManifest(layerDesc)
	if err != nil {
		return ocispec.Descriptor{}, false, err
	}

	idx, err := loadArchiveIndex(ctx, target, tag)
	if err != nil {
		return ocispec.Descriptor{}, false, err
	}
	if cur, ok := manifestForPlatform(idx, plat); ok && cur.Digest == manDesc.Digest && !force {
		return withPlatform(manDesc, plat), false, nil
	}

	if err := PushEmptyConfig(ctx, target); err != nil {
		return ocispec.Descriptor{}, false, err
	}
	if err := pushBlobIfAbsent(ctx, target, layerDesc, archiveBytes); err != nil {
		return ocispec.Descriptor{}, false, err
	}
	if err := pushBlobIfAbsent(ctx, target, manDesc, manBytes); err != nil {
		return ocispec.Descriptor{}, false, err
	}

	entry := withPlatform(manDesc, plat)
	if _, err := CommitArchiveManifests(ctx, target, tag, []ocispec.Descriptor{entry}); err != nil {
		return ocispec.Descriptor{}, false, err
	}
	return entry, true, nil
}

func PublishArchiveLayerFile(ctx context.Context, target oras.Target, plat spec.Platform, path, sha256Hex string, size int64) (ocispec.Descriptor, error) {
	layerDesc := ocispec.Descriptor{
		MediaType: MediaTypeArchive,
		Digest:    digest.NewDigestFromEncoded(digest.SHA256, sha256Hex),
		Size:      size,
	}
	manBytes, manDesc, err := archiveManifest(layerDesc)
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	err = withRetry(ctx, func(ctx context.Context) error {
		if err := PushEmptyConfig(ctx, target); err != nil {
			return err
		}
		if err := pushArchiveLayerFile(ctx, target, layerDesc, path); err != nil {
			return err
		}
		return pushBlobIfAbsent(ctx, target, manDesc, manBytes)
	})
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	return withPlatform(manDesc, plat), nil
}

func pushArchiveLayerFile(ctx context.Context, target oras.Target, layerDesc ocispec.Descriptor, path string) error {
	exists, err := target.Exists(ctx, layerDesc)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	if err := target.Push(ctx, layerDesc, f); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return err
	}
	return nil
}

func archiveManifest(layerDesc ocispec.Descriptor) ([]byte, ocispec.Descriptor, error) {
	manBytes, err := json.Marshal(ocispec.Manifest{
		SchemaVersion: 2,
		MediaType:     ocispec.MediaTypeImageManifest,
		Config:        ocispec.DescriptorEmptyJSON,
		Layers:        []ocispec.Descriptor{layerDesc},
	})
	if err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("marshal archive manifest: %w", err)
	}
	return manBytes, content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, manBytes), nil
}

func CommitArchiveManifests(ctx context.Context, target oras.Target, tag string, entries []ocispec.Descriptor) (ocispec.Descriptor, error) {
	if len(entries) == 0 {
		return ocispec.Descriptor{}, nil
	}

	var result ocispec.Descriptor
	err := withRetry(ctx, func(ctx context.Context) error {
		cur, curErr := target.Resolve(ctx, tag)
		var idx ocispec.Index
		switch {
		case curErr == nil:
			data, err := content.FetchAll(ctx, target, cur)
			if err != nil {
				return fmt.Errorf("read archive index %s: %w", tag, err)
			}
			if err := json.Unmarshal(data, &idx); err != nil {
				return fmt.Errorf("parse archive index %s: %w", tag, err)
			}
		case archiveAbsent(curErr):

		default:
			return fmt.Errorf("resolve archive index %s: %w", tag, curErr)
		}

		merged := idx.Manifests
		for _, entry := range entries {
			merged = mergeManifests(merged, entry)
		}
		idxBytes, err := json.Marshal(ocispec.Index{
			SchemaVersion: 2,
			MediaType:     ocispec.MediaTypeImageIndex,
			Manifests:     merged,
		})
		if err != nil {
			return fmt.Errorf("marshal archive index: %w", err)
		}
		idxDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, idxBytes)

		if curErr == nil && cur.Digest == idxDesc.Digest {
			result = idxDesc
			return nil
		}
		if err := PushBlobAndTag(ctx, target, idxBytes, idxDesc, []string{tag}); err != nil {
			return err
		}
		result = idxDesc
		return nil
	})
	return result, err
}

func withPlatform(d ocispec.Descriptor, plat spec.Platform) ocispec.Descriptor {
	d.Platform = &ocispec.Platform{OS: plat.OS, Architecture: plat.Arch}
	return d
}

func mergeManifests(mans []ocispec.Descriptor, entry ocispec.Descriptor) []ocispec.Descriptor {
	out := make([]ocispec.Descriptor, 0, len(mans)+1)
	for _, m := range mans {
		if m.Platform != nil && platKey(m) == platKey(entry) {
			continue
		}
		out = append(out, m)
	}
	out = append(out, entry)
	sort.Slice(out, func(i, j int) bool { return platKey(out[i]) < platKey(out[j]) })
	return out
}

func platKey(d ocispec.Descriptor) string {
	if d.Platform == nil {
		return ""
	}
	return d.Platform.OS + "/" + d.Platform.Architecture
}

func loadArchiveIndex(ctx context.Context, target oras.Target, tag string) (ocispec.Index, error) {
	desc, err := target.Resolve(ctx, tag)
	if err != nil {
		if archiveAbsent(err) {
			return ocispec.Index{}, nil
		}
		return ocispec.Index{}, fmt.Errorf("resolve archive index %s: %w", tag, err)
	}
	data, err := content.FetchAll(ctx, target, desc)
	if err != nil {
		return ocispec.Index{}, fmt.Errorf("read archive index %s: %w", tag, err)
	}
	var idx ocispec.Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return ocispec.Index{}, fmt.Errorf("parse archive index %s: %w", tag, err)
	}
	return idx, nil
}
