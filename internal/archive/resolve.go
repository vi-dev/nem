package archive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote/errcode"

	"github.com/vi-dev/nem/internal/spec"
)

func ResolveIndex(ctx context.Context, target oras.ReadOnlyTarget, version string) (ocispec.Descriptor, error) {
	desc, err := target.Resolve(ctx, version)
	if err != nil {
		if isNotFoundError(err) {
			return ocispec.Descriptor{}, fmt.Errorf("resolve archive %s: %w", version, ErrNotFound)
		}
		return ocispec.Descriptor{}, fmt.Errorf("resolve archive %s: %w", version, err)
	}
	return desc, nil
}

func fetchIndex(ctx context.Context, src oras.ReadOnlyTarget, version string) (ocispec.Descriptor, ocispec.Index, error) {
	desc, err := ResolveIndex(ctx, src, version)
	if err != nil {
		return ocispec.Descriptor{}, ocispec.Index{}, err
	}
	data, err := content.FetchAll(ctx, src, desc)
	if err != nil {
		return ocispec.Descriptor{}, ocispec.Index{}, fmt.Errorf("fetch archive %s index: %w", version, err)
	}
	var idx ocispec.Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return ocispec.Descriptor{}, ocispec.Index{}, fmt.Errorf("parse archive %s index: %w", version, err)
	}
	return desc, idx, nil
}

func ResolvePlatforms(ctx context.Context, src oras.ReadOnlyTarget, version string) ([]spec.Platform, error) {
	_, idx, err := fetchIndex(ctx, src, version)
	if err != nil {
		return nil, err
	}
	var platforms []spec.Platform
	for _, m := range idx.Manifests {
		if m.Platform != nil {
			platforms = append(platforms, spec.Platform{OS: m.Platform.OS, Arch: m.Platform.Architecture})
		}
	}
	return platforms, nil
}

func Resolve(ctx context.Context, src oras.ReadOnlyTarget, version string, platform spec.Platform) (ocispec.Descriptor, error) {
	_, idx, err := fetchIndex(ctx, src, version)
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	platDesc, ok := selectPlatform(idx, platform)
	if !ok {
		return ocispec.Descriptor{}, fmt.Errorf("archive %s: platform %s: %w", version, platform, ErrNotFound)
	}

	platData, err := content.FetchAll(ctx, src, platDesc)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("fetch archive %s (%s) manifest: %w", version, platform, err)
	}
	var platMan ocispec.Manifest
	if err := json.Unmarshal(platData, &platMan); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("parse archive %s (%s) manifest: %w", version, platform, err)
	}

	for _, desc := range platMan.Layers {
		if desc.MediaType == MediaType {
			return desc, nil
		}
	}

	return ocispec.Descriptor{}, fmt.Errorf("archive %s (%s): manifest has no %s layer", version, platform, MediaType)
}
func selectPlatform(idx ocispec.Index, platform spec.Platform) (ocispec.Descriptor, bool) {
	for _, m := range idx.Manifests {
		if m.Platform != nil && m.Platform.OS == platform.OS && m.Platform.Architecture == platform.Arch {
			return m, true
		}
	}
	return ocispec.Descriptor{}, false
}

func isNotFoundError(err error) bool {
	if errors.Is(err, errdef.ErrNotFound) {
		return true
	}
	if resp, ok := errors.AsType[*errcode.ErrorResponse](err); ok {
		return resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound
	}
	return false
}
