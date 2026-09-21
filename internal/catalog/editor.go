package catalog

import (
	"context"
	"fmt"

	"github.com/vi-dev/nem/internal/spec"
)

type Editor interface {
	ReadManifest(ctx context.Context, name string) ([]byte, error)
	CreateManifest(ctx context.Context, name string, data []byte) error
	UpdateManifest(ctx context.Context, name string, data []byte) error
}

func validateManifest(name string, data []byte) error {
	pkg, err := spec.Parse(data)
	if err != nil {
		return err
	}
	if err := pkg.Validate(); err != nil {
		return err
	}
	if pkg.Name != name {
		return fmt.Errorf("manifest declares name %q, want %q", pkg.Name, name)
	}
	return nil
}
