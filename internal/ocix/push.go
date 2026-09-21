package ocix

import (
	"bytes"
	"context"
	"errors"
	"io"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/errdef"
)

func PushEmptyConfig(ctx context.Context, target oras.Target) error {
	return PushBlobIfAbsent(ctx, target, ocispec.DescriptorEmptyJSON, bytes.NewReader(ocispec.DescriptorEmptyJSON.Data))
}

func PushBlobAndTag(ctx context.Context, target oras.Target, data []byte, desc ocispec.Descriptor, tags []string) error {
	if err := PushBlobIfAbsent(ctx, target, desc, bytes.NewReader(data)); err != nil {
		return err
	}
	for _, tag := range tags {
		if err := target.Tag(ctx, desc, tag); err != nil {
			return err
		}
	}
	return nil
}

func PushBlobIfAbsent(ctx context.Context, target oras.Target, desc ocispec.Descriptor, r io.Reader) error {
	ok, err := target.Exists(ctx, desc)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	if err := target.Push(ctx, desc, r); err != nil && !errors.Is(err, errdef.ErrAlreadyExists) {
		return err
	}
	return nil
}
