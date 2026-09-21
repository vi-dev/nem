package mirror

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"

	"golang.org/x/sync/errgroup"
	"oras.land/oras-go/v2"

	"github.com/vi-dev/nem/internal/archive"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/report"
)

type Options struct {
	SrcRef, DstRef string
	DryRun         bool
}

type Summary struct {
	Packages int
	Copied   int
	Failed   int
	DryRun   bool
}

func (s Summary) String() string {
	verb := "Mirrored"
	if s.DryRun {
		verb = "Would mirror"
	}
	if s.Failed > 0 {
		return fmt.Sprintf("%s %d packages, %d tag(s), %d tag(s) failed",
			verb, s.Packages, s.Copied, s.Failed)
	}
	return fmt.Sprintf("%s %d packages, %d tag(s)", verb, s.Packages, s.Copied)
}

var (
	openSrcCatalog = ocix.RemoteCatalog
	openDstCatalog = ocix.RemoteCatalogRW
)

func SetSrcCatalogOpener(f func(ref string) (oras.ReadOnlyTarget, string, error)) (restore func()) {
	prev := openSrcCatalog
	openSrcCatalog = f
	return func() { openSrcCatalog = prev }
}

func SetDstCatalogOpener(f func(ref string) (oras.Target, string, error)) (restore func()) {
	prev := openDstCatalog
	openDstCatalog = f
	return func() { openDstCatalog = prev }
}

func Run(ctx context.Context, opts Options) (Summary, error) {
	if err := ocix.WithTagOrDigest(opts.SrcRef); err != nil {
		return Summary{}, err
	}
	if err := ocix.WithTagOrDigest(opts.DstRef); err != nil {
		return Summary{}, err
	}
	if err := ctx.Err(); err != nil {
		return Summary{}, err
	}

	store, err := syncCatalog(ctx, opts)
	if err != nil {
		return Summary{}, err
	}

	pkgs := store.Packages()
	summary := Summary{Packages: len(pkgs), DryRun: opts.DryRun}
	var agg aggregator

	srcArchives := archive.Remote(opts.SrcRef)
	dstArchives := archive.Remote(opts.DstRef)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(min(runtime.NumCPU(), 8))
	for _, pkg := range pkgs {
		g.Go(func() error {
			if gctx.Err() != nil {
				return nil
			}
			mirrorPackage(gctx, opts, pkg, store, srcArchives, dstArchives, &agg)
			return nil
		})
	}
	_ = g.Wait()

	summary.Copied = int(agg.copied.Load())
	summary.Failed = int(agg.failed.Load())

	if ctx.Err() != nil {
		return summary, ctx.Err()
	}
	return summary, nil
}

type aggregator struct {
	copied, failed atomic.Int64
}

func syncCatalog(ctx context.Context, opts Options) (*ocix.Store, error) {
	store, err := pullCatalog(ctx, opts.SrcRef)
	if err != nil {
		return nil, err
	}
	if !opts.DryRun {
		if err := pushCatalog(ctx, store, opts.DstRef); err != nil {
			return nil, err
		}
	}
	return store, nil
}

func pullCatalog(ctx context.Context, srcRef string) (*ocix.Store, error) {
	labels := report.TaskLabels{Run: "Pulling catalog", Segment: "copying", Done: "Pulled catalog", Fail: "Pull failed"}
	var store *ocix.Store
	err := report.RunTask(ctx, labels, func(progress report.ProgressFunc) error {
		src, srcTag, err := openSrcCatalog(srcRef)
		if err != nil {
			return err
		}
		store, err = ocix.OpenStoreInMemory(ctx, src, srcTag, progress)
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

func pushCatalog(ctx context.Context, store *ocix.Store, dstRef string) error {
	labels := report.TaskLabels{Run: "Pushing catalog", Segment: "copying", Done: "Pushed catalog", Fail: "Push failed"}
	return report.RunTask(ctx, labels, func(progress report.ProgressFunc) error {
		dst, dstTag, err := openDstCatalog(dstRef)
		if err != nil {
			return err
		}
		pushed, err := store.CopyTo(ctx, dst, dstTag, progress)
		if err != nil {
			return fmt.Errorf("publish catalog: %w", err)
		}
		got, err := dst.Resolve(ctx, dstTag)
		if err != nil {
			return fmt.Errorf("verify published catalog: %w", err)
		}
		if got.Digest != pushed.Digest {
			return fmt.Errorf("catalog %s digest mismatch after publish: copied %s, resolved %s", dstTag, pushed.Digest, got.Digest)
		}
		return nil
	})
}
