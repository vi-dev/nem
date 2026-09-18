package ocix

import (
	"context"
	"fmt"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"

	"github.com/vi-dev/nem/internal/spec"
)

func VerifyArchiveCommitted(ctx context.Context, target oras.ReadOnlyTarget, tag string, plat spec.Platform, want ocispec.Descriptor) error {
	idx, err := fetchArchiveIndex(ctx, target, tag)
	if err != nil {
		return fmt.Errorf("verify archive %s (%s): %w", tag, plat, err)
	}
	got, ok := manifestForPlatform(idx, plat)
	if !ok {
		return fmt.Errorf("verify archive %s: index has no manifest for %s after commit", tag, plat)
	}
	if got.Digest != want.Digest {
		return fmt.Errorf("verify archive %s (%s): committed %s, index holds %s", tag, plat, want.Digest, got.Digest)
	}
	return nil
}
