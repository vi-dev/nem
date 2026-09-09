package ocix

import (
	"bytes"
	"context"
	"errors"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/errdef"
)

func PushEmptyConfig(ctx context.Context, target oras.Target) error {
	return pushBlobIfAbsent(ctx, target, ocispec.DescriptorEmptyJSON, ocispec.DescriptorEmptyJSON.Data)
}

func PushBlobAndTag(ctx context.Context, target oras.Target, bytes []byte, desc ocispec.Descriptor, tags []string) error {
	if err := pushBlobIfAbsent(ctx, target, desc, bytes); err != nil {
		return err
	}
	for _, tag := range tags {
		if err := target.Tag(ctx, desc, tag); err != nil {
			return err
		}
	}
	return nil
}

func pushBlobIfAbsent(ctx context.Context, target oras.Target, desc ocispec.Descriptor, data []byte) error {
	ok, err := target.Exists(ctx, desc)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	if err := target.Push(ctx, desc, bytes.NewReader(data)); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return err
	}
	return nil
}
