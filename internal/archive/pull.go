package archive

import (
	"context"
	"fmt"
	"io"
	"os"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"

	"github.com/vi-dev/nem/internal/spec"
)

func Fetch(ctx context.Context, src oras.ReadOnlyTarget, version string, platform spec.Platform) (io.ReadCloser, error) {
	desc, err := Resolve(ctx, src, version, platform)
	if err != nil {
		return nil, err
	}
	rc, err := src.Fetch(ctx, desc)
	if err != nil {
		return nil, fmt.Errorf("fetch archive %s (%s): %w", version, platform, err)
	}
	return &verifyReadCloser{verify: content.NewVerifyReader(rc, desc), rc: rc}, nil
}

func Pull(ctx context.Context, src oras.ReadOnlyTarget, version string, platform spec.Platform, dir string) (string, error) {
	rc, err := Fetch(ctx, src, version, platform)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	return writeTempFile(dir, rc)
}

func writeTempFile(dir string, r io.Reader) (string, error) {
	f, err := os.CreateTemp(dir, "archive-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	fname := f.Name()
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		os.Remove(fname)
		return "", fmt.Errorf("write temp file to %s: %w", fname, err)
	}
	if err := f.Close(); err != nil {
		os.Remove(fname)
		return "", fmt.Errorf("close temp file %s: %w", fname, err)
	}
	return fname, nil
}
