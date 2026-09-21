package archive

import (
	"context"

	"oras.land/oras-go/v2"

	"github.com/vi-dev/nem/internal/ocix"
)

type remote struct{ ref string }

func Remote(catalogRef string) ReadWriteStore { return remote{ref: catalogRef} }

func (r remote) Open(ctx context.Context, name string) (oras.ReadOnlyTarget, error) {
	return r.OpenRW(ctx, name)
}

func (r remote) OpenRW(_ context.Context, name string) (oras.Target, error) {
	archivesRef, err := Ref(r.ref, name)
	if err != nil {
		return nil, err
	}
	return openRepo(archivesRef)
}

var openRepo = func(ref string) (oras.Target, error) {
	return ocix.NewRemoteRepository(ref)
}

func SetRepoOpener(f func(ref string) (oras.Target, error)) (restore func()) {
	prev := openRepo
	openRepo = f
	return func() { openRepo = prev }
}
