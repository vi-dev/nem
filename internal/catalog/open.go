package catalog

import (
	"context"

	"github.com/vi-dev/nem/internal/config"
	"github.com/vi-dev/nem/internal/home"
	"github.com/vi-dev/nem/internal/ocix"
)

func Open(cfg *config.Config, h home.Home) (*Set, error) {
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
