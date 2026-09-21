package archive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"

	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/spec"
)

func Push(ctx context.Context, target oras.Target, version string, platform spec.Platform, blob Blob, force bool) (ocispec.Descriptor, bool, error) {
	manBytes, manDesc, err := createManifest(blob.descriptor())
	if err != nil {
		return ocispec.Descriptor{}, false, err
	}

	_, idx, err := fetchIndex(ctx, target, version)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return ocispec.Descriptor{}, false, err
	}
	if found, ok := selectPlatform(idx, platform); ok && found.Digest == manDesc.Digest && !force {
		return withPlatform(manDesc, platform), false, nil
	}

	if err := stageBlob(ctx, target, blob, manBytes, manDesc); err != nil {
		return ocispec.Descriptor{}, false, err
	}

	entry := withPlatform(manDesc, platform)
	if _, err := Commit(ctx, target, version, []ocispec.Descriptor{entry}); err != nil {
		return ocispec.Descriptor{}, false, err
	}
	return entry, true, nil
}

func Stage(ctx context.Context, target oras.Target, platform spec.Platform, blob Blob) (ocispec.Descriptor, error) {
	manBytes, manDesc, err := createManifest(blob.descriptor())
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	if err := stageBlob(ctx, target, blob, manBytes, manDesc); err != nil {
		return ocispec.Descriptor{}, err
	}
	return withPlatform(manDesc, platform), nil
}

func stageBlob(ctx context.Context, target oras.Target, blob Blob, manBytes []byte, manDesc ocispec.Descriptor) error {
	return ocix.WithRetry(ctx, func(ctx context.Context) error {
		if err := ocix.PushEmptyConfig(ctx, target); err != nil {
			return err
		}
		if err := pushBlob(ctx, target, blob); err != nil {
			return err
		}
		return ocix.PushBlobIfAbsent(ctx, target, manDesc, bytes.NewReader(manBytes))
	})
}

func pushBlob(ctx context.Context, target oras.Target, blob Blob) error {
	rc, err := blob.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	return ocix.PushBlobIfAbsent(ctx, target, blob.descriptor(), rc)
}

func Commit(ctx context.Context, target oras.Target, version string, entries []ocispec.Descriptor) (ocispec.Descriptor, error) {
	if len(entries) == 0 {
		return ocispec.Descriptor{}, nil
	}

	var result ocispec.Descriptor
	err := ocix.WithRetry(ctx, func(ctx context.Context) error {
		cur, idx, rerr := fetchIndex(ctx, target, version)
		if rerr != nil && !errors.Is(rerr, ErrNotFound) {
			return rerr
		}

		merged := idx.Manifests
		for _, entry := range entries {
			merged = mergeByPlatform(merged, entry)
		}
		idxBytes, err := json.Marshal(ocispec.Index{
			SchemaVersion: 2,
			MediaType:     ocispec.MediaTypeImageIndex,
			Manifests:     merged,
		})
		if err != nil {
			return fmt.Errorf("marshal archive %s index: %w", version, err)
		}
		idxDesc := content.NewDescriptorFromBytes(ocispec.MediaTypeImageIndex, idxBytes)

		if rerr == nil && cur.Digest == idxDesc.Digest {
			result = idxDesc
			return nil
		}
		if err := ocix.PushBlobAndTag(ctx, target, idxBytes, idxDesc, []string{version}); err != nil {
			return err
		}
		result = idxDesc
		return nil
	})
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	if err := verifyCommitted(ctx, target, version, entries); err != nil {
		return ocispec.Descriptor{}, err
	}
	return result, nil
}

func verifyCommitted(ctx context.Context, target oras.ReadOnlyTarget, version string, want []ocispec.Descriptor) error {
	_, idx, err := fetchIndex(ctx, target, version)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return fmt.Errorf("verify archive %s: index not resolvable after commit", version)
		}
		return fmt.Errorf("verify archive %s: %w", version, err)
	}
	for _, w := range want {
		if w.Platform == nil {
			continue
		}
		plat := spec.Platform{OS: w.Platform.OS, Arch: w.Platform.Architecture}
		got, found := selectPlatform(idx, plat)
		if !found {
			return fmt.Errorf("verify archive %s: index has no manifest for %s after commit", version, plat)
		}
		if got.Digest != w.Digest {
			return fmt.Errorf("verify archive %s (%s): committed %s, index holds %s", version, plat, w.Digest, got.Digest)
		}
	}
	return nil
}

func createManifest(blobDescriptor ocispec.Descriptor) ([]byte, ocispec.Descriptor, error) {
	manifestBytes, err := json.Marshal(ocispec.Manifest{
		SchemaVersion: 2,
		MediaType:     ocispec.MediaTypeImageManifest,
		Config:        ocispec.DescriptorEmptyJSON,
		Layers:        []ocispec.Descriptor{blobDescriptor},
	})
	if err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("marshal archive manifest: %w", err)
	}
	return manifestBytes, content.NewDescriptorFromBytes(ocispec.MediaTypeImageManifest, manifestBytes), nil
}

func withPlatform(desc ocispec.Descriptor, platform spec.Platform) ocispec.Descriptor {
	desc.Platform = &ocispec.Platform{OS: platform.OS, Architecture: platform.Arch}
	return desc
}

func mergeByPlatform(elems []ocispec.Descriptor, desc ocispec.Descriptor) []ocispec.Descriptor {
	var platform = func(d ocispec.Descriptor) string {
		if d.Platform == nil {
			return ""
		}
		return d.Platform.OS + "/" + d.Platform.Architecture
	}
	merged := make([]ocispec.Descriptor, 0, len(elems)+1)
	for _, e := range elems {
		if e.Platform != nil && platform(e) == platform(desc) {
			continue
		}
		merged = append(merged, e)
	}
	merged = append(merged, desc)
	sort.Slice(merged, func(i, j int) bool { return platform(merged[i]) < platform(merged[j]) })
	return merged
}
