package catalog

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/vi-dev/clix/console"
	"oras.land/oras-go/v2"

	"github.com/vi-dev/nem/internal/archive"
	"github.com/vi-dev/nem/internal/config"
	"github.com/vi-dev/nem/internal/home"
	"github.com/vi-dev/nem/internal/ocix"
)

func OpenConfigured(cfg *config.Config, h home.Home) (*Set, error) {
	entries := make([]Entry, 0, len(cfg.Catalogs))
	for _, e := range cfg.Catalogs {
		if e.Disabled {
			continue
		}
		switch e.Type {
		case "dir":
			entries = append(entries, Entry{Name: e.Name, Catalog: NewDir(e.Path),
				Archives: archive.NewDir(e.Path)})
		case "oci":
			storePath, err := h.CatalogStore(e.Name)
			if err != nil {
				return nil, err
			}
			src := NewLazyOCI(e.Name, func(ctx context.Context) (*ocix.Store, error) {
				return ocix.OpenLocalStore(ctx, storePath)
			})
			entries = append(entries, Entry{Name: e.Name, Ref: e.Ref, Catalog: src,
				Archives: archive.Remote(e.Ref)})
		}
	}
	return NewSet(entries...), nil
}

func Open(ctx context.Context, cat string) (Entry, error) {
	return open(ctx, cat, openRemote)
}

func OpenIndex(ctx context.Context, cat string) (Entry, error) {
	return open(ctx, cat, openRemoteIndex)
}

func Local(cat string) bool {
	_, err := os.Stat(cat)
	return err == nil || looksLikePath(cat)
}

func open(ctx context.Context, cat string, remote func(context.Context, string) (Catalog, error)) (Entry, error) {
	info, err := os.Stat(cat)
	switch {
	case err == nil && info.IsDir():
		return Entry{Name: cat, Catalog: NewDir(cat), Archives: archive.NewDir(cat)}, nil
	case err == nil:
		return Entry{Name: cat, Catalog: NewFile(cat), Archives: archive.NopStore}, nil
	case !os.IsNotExist(err) || looksLikePath(cat):
		return Entry{}, err
	}
	if err := ocix.WithTagOrDigest(cat); err != nil {
		return Entry{}, err
	}
	src, err := remote(ctx, cat)
	if err != nil {
		return Entry{}, err
	}
	return Entry{Name: cat, Ref: cat, Catalog: src, Archives: archive.Remote(cat)}, nil
}

func looksLikePath(arg string) bool {
	return strings.HasPrefix(arg, "./") || strings.HasPrefix(arg, "../") ||
		strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, "~") ||
		strings.HasSuffix(arg, ".yaml") || strings.HasSuffix(arg, ".yml")
}

var remoteTarget = ocix.RemoteCatalog

func SetRemoteTarget(f func(ref string) (oras.ReadOnlyTarget, string, error)) (restore func()) {
	prev := remoteTarget
	remoteTarget = f
	return func() { remoteTarget = prev }
}

var openRemote = func(ctx context.Context, ref string) (Catalog, error) {
	outcomes := console.Outcomes{
		Done:   "Pulled catalog " + ref,
		Failed: "Failed to pull catalog " + ref,
	}
	var store *ocix.Store
	err := console.RunTask(ctx, "Pulling catalog "+ref, "pulling manifest", outcomes, func(progress console.ProgressFunc) error {
		src, tag, err := remoteTarget(ref)
		if err != nil {
			return err
		}
		store, err = ocix.OpenStoreInMemory(ctx, src, tag, progress)
		if err != nil {
			return fmt.Errorf("pull catalog %s: %w", ref, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return NewOCI(ref, store), nil
}

var openRemoteIndex = func(ctx context.Context, ref string) (Catalog, error) {
	outcomes := console.Outcomes{
		Done:   "Read catalog " + ref,
		Failed: "Failed to read catalog " + ref,
	}
	var store *ocix.Store
	err := console.RunTask(ctx, "Reading catalog "+ref, "reading index", outcomes, func(console.ProgressFunc) error {
		src, tag, err := remoteTarget(ref)
		if err != nil {
			return err
		}
		store, err = ocix.OpenStore(ctx, src, tag)
		if err != nil {
			return fmt.Errorf("read catalog %s: %w", ref, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return NewOCI(ref, store), nil
}
