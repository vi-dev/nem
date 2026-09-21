package catalog

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/vi-dev/nem/internal/archive"
	"github.com/vi-dev/nem/internal/config"
	"github.com/vi-dev/nem/internal/home"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/report"
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
	src, err := openRemote(ctx, cat)
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

var openRemote = func(ctx context.Context, ref string) (Catalog, error) {
	labels := report.TaskLabels{
		Run:     "Pulling catalog " + ref,
		Segment: "pulling manifest",
		Done:    "Pulled catalog " + ref,
		Fail:    "Failed to pull catalog " + ref,
	}
	var store *ocix.Store
	err := report.RunTask(ctx, labels, func(progress report.ProgressFunc) error {
		src, tag, err := ocix.RemoteCatalog(ref)
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
