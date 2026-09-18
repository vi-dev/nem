package catalog

import (
	"context"
	"fmt"
	"os"
	"strings"

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
			entries = append(entries, Entry{Name: e.Name, Catalog: NewDir(e.Path)})
		case "oci":
			storePath, err := h.CatalogStore(e.Name)
			if err != nil {
				return nil, err
			}
			src := NewLazyOCI(e.Name, func(ctx context.Context) (*ocix.Store, error) {
				return ocix.OpenLocalStore(ctx, storePath)
			})
			entries = append(entries, Entry{Name: e.Name, Ref: e.Ref, Catalog: src})
		}
	}
	return NewSet(entries...), nil
}

func Open(ctx context.Context, cat string) (Catalog, error) {
	info, err := os.Stat(cat)
	switch {
	case err == nil && info.IsDir():
		return NewDir(cat), nil
	case err == nil:
		return NewFile(cat), nil
	case !os.IsNotExist(err) || looksLikePath(cat):
		return nil, err
	}
	if err := ocix.WithTagOrDigest(cat); err != nil {
		return nil, err
	}
	return openRemote(ctx, cat)
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
