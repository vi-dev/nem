package catalog

import (
	"context"
	"errors"

	"github.com/vi-dev/nem/internal/config"
	"github.com/vi-dev/nem/internal/home"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/project"
	"github.com/vi-dev/nem/internal/spec"
)

func Open(cfg *config.Config, h home.Home) ([]Named, error) {
	out := make([]Named, 0, len(cfg.Catalogs))
	for _, e := range cfg.Catalogs {
		if e.Disabled {
			continue
		}
		switch e.Type {
		case "dir":
			out = append(out, Named{Name: e.Name, Source: NewDir(e.Path)})
		case "oci":
			store, err := h.CatalogStore(e.Name)
			if err != nil {
				return nil, err
			}
			out = append(out, Named{Name: e.Name, Source: NewOCI(e.Name, store)})
		}
	}
	return out, nil
}

func Lookup(ctx context.Context, sources []Named, key project.ToolKey) (*spec.Package, string, string, error) {
	if key.Catalog != "" {
		for _, n := range sources {
			if n.Name != key.Catalog {
				continue
			}
			pkg, dig, err := n.Source.Load(ctx, key.Name)
			if nf, ok := errors.AsType[*PackageNotFoundError](err); ok {
				nf.Catalogs = []string{n.Name}
				return nil, "", "", nf
			}
			if err != nil {
				return nil, "", "", err
			}
			return pkg, n.Name, dig, nil
		}
		return nil, "", "", &NotFoundError{Name: key.Catalog}
	}
	searched := make([]string, 0, len(sources))
	var notSynced []string
	for _, n := range sources {
		searched = append(searched, n.Name)
		pkg, dig, err := n.Source.Load(ctx, key.Name)
		if err != nil {
			if _, ok := errors.AsType[*PackageNotFoundError](err); ok {
				continue
			}
			if errors.Is(err, ocix.ErrNotSynced) {
				notSynced = append(notSynced, n.Name)
				continue
			}
			return nil, "", "", err
		}
		return pkg, n.Name, dig, nil
	}
	if len(notSynced) > 0 {
		return nil, "", "", &NotSyncedError{Name: key.Name, Catalogs: notSynced}
	}
	return nil, "", "", &PackageNotFoundError{Name: key.Name, Catalogs: searched}
}
