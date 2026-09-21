package fill

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"sort"
	"sync/atomic"

	"golang.org/x/sync/errgroup"
	"oras.land/oras-go/v2"

	"github.com/vi-dev/nem/internal/archive"
	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/home"
	"github.com/vi-dev/nem/internal/netx"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/report"
)

type Options struct {
	CatalogRef string
	Packages   []string
	DryRun     bool
}

type Summary struct {
	Packages    int
	Filled      int
	Healed      int
	Present     int
	NotFillable int
	Failed      int
}

var openCatalog = ocix.RemoteCatalog

func SetCatalogOpener(f func(ref string) (oras.ReadOnlyTarget, string, error)) (restore func()) {
	prev := openCatalog
	openCatalog = f
	return func() { openCatalog = prev }
}

var httpClient = netx.Client()

func SetHTTPClient(c *http.Client) (restore func()) {
	prev := httpClient
	httpClient = c
	return func() { httpClient = prev }
}

func Run(ctx context.Context, h home.Home, opts Options) (Summary, error) {
	if err := ocix.WithTagOrDigest(opts.CatalogRef); err != nil {
		return Summary{}, err
	}
	if err := ctx.Err(); err != nil {
		return Summary{}, err
	}

	store, err := stageCatalog(ctx, opts.CatalogRef)
	if err != nil {
		return Summary{}, err
	}

	pkgs, err := scopePackages(opts.CatalogRef, store.Packages(), opts.Packages)
	if err != nil {
		return Summary{}, err
	}

	if !opts.DryRun {
		if err := os.MkdirAll(h.Tmp(), 0o755); err != nil {
			return Summary{}, fmt.Errorf("create tmp dir: %w", err)
		}
	}

	summary := Summary{Packages: len(pkgs)}
	var agg aggregator
	archives := archive.Remote(opts.CatalogRef)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(min(runtime.NumCPU(), 8))
	for _, nm := range pkgs {
		g.Go(func() error {
			if gctx.Err() != nil {
				return nil
			}
			fillPackage(gctx, h, opts, nm, store, archives, &agg)
			return nil
		})
	}
	_ = g.Wait()

	summary.Filled = int(agg.filled.Load())
	summary.Healed = int(agg.healed.Load())
	summary.Present = int(agg.present.Load())
	summary.NotFillable = int(agg.notFillable.Load())
	summary.Failed = int(agg.failed.Load())

	if ctx.Err() != nil {
		return summary, ctx.Err()
	}
	return summary, nil
}

func stageCatalog(ctx context.Context, ref string) (*ocix.Store, error) {
	labels := report.TaskLabels{
		Run:     "Pulling catalog " + ref,
		Segment: "pulling manifest",
		Done:    "Pulled catalog " + ref,
		Fail:    "Failed to pull catalog " + ref,
	}
	var store *ocix.Store
	err := report.RunTask(ctx, labels, func(count report.ProgressFunc) error {
		src, srcTag, err := openCatalog(ref)
		if err != nil {
			return err
		}
		store, err = ocix.OpenStoreInMemory(ctx, src, srcTag, count)
		if err != nil {
			return fmt.Errorf("stage catalog: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return store, nil
}

func scopePackages(ref string, all []ocix.TitledManifest, want []string) ([]ocix.TitledManifest, error) {
	if len(want) == 0 {
		return all, nil
	}
	byName := make(map[string]ocix.TitledManifest, len(all))
	for _, nm := range all {
		byName[nm.Title] = nm
	}
	seen := make(map[string]bool, len(want))
	var out []ocix.TitledManifest
	var unknown []string
	for _, w := range want {
		if seen[w] {
			continue
		}
		seen[w] = true
		if nm, ok := byName[w]; ok {
			out = append(out, nm)
		} else {
			unknown = append(unknown, w)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, &catalog.PackageNotFoundError{Name: unknown[0], Catalogs: []string{ref}}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out, nil
}

type aggregator struct {
	filled, healed, present, notFillable, failed atomic.Int64
}
