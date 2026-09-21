package archive

import (
	"context"
	"fmt"

	"oras.land/oras-go/v2"
)

type Store interface {
	Open(ctx context.Context, name string) (oras.ReadOnlyTarget, error)
}

type ReadWriteStore interface {
	Store
	OpenRW(ctx context.Context, name string) (oras.Target, error)
}

type nopStore struct{}

var NopStore Store = nopStore{}

func (nopStore) Open(_ context.Context, name string) (oras.ReadOnlyTarget, error) {
	return nil, fmt.Errorf("no archive source for %s: %w", name, ErrNotFound)
}
