package install

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"

	"golang.org/x/sync/errgroup"

	"github.com/vi-dev/nem/internal/fetch"
	"github.com/vi-dev/nem/internal/home"
	"github.com/vi-dev/nem/internal/report"
	"github.com/vi-dev/nem/internal/spec"
)

type Job struct {
	Pkg     *spec.Package
	Version string
	Catalog string
	Source  fetch.Source
}

var acquire = fetch.Acquire

func Run(ctx context.Context, h home.Home, rep report.Reporter, jobs []Job) error {
	if err := os.MkdirAll(h.Tmp(), 0o755); err != nil {
		return fmt.Errorf("create tmp dir: %w", err)
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(min(runtime.NumCPU(), 8))

	for _, job := range jobs {
		g.Go(func() error { return runJob(gctx, h, rep, job) })
	}
	if err := g.Wait(); err != nil {
		return err
	}
	return ctx.Err()
}

func runJob(gctx context.Context, h home.Home, rep report.Reporter, job Job) error {
	name, version := job.Pkg.Name, job.Version
	if IsInstalled(h, name, version) {
		return nil
	}

	label := fmt.Sprintf("Installing %s %s", name, version)
	cancelled := fmt.Sprintf("Cancelled %s %s", name, version)

	failedOutcome := fmt.Sprintf("Failed %s %s", name, version)

	task := rep.Task(label)
	if isCancellation(gctx, nil) {
		task.Fail(cancelled)
		return nil
	}

	task.Status("downloading")
	artifact, err := acquire(gctx, job.Pkg, version, spec.Current(), job.Source, h.Tmp(), task)
	if err != nil {
		if isCancellation(gctx, err) {
			task.Fail(cancelled)
			return nil
		}
		task.Fail(failedOutcome)
		return err
	}

	defer os.Remove(artifact)

	task.Status("extracting")
	if err := Install(gctx, h, job.Pkg, version, job.Catalog, artifact); err != nil {
		if isCancellation(gctx, err) {
			task.Fail(cancelled)
			return nil
		}
		task.Fail(failedOutcome)
		return err
	}

	task.Done(fmt.Sprintf("Installed %s %s", name, version))
	return nil
}

func isCancellation(gctx context.Context, err error) bool {
	if err != nil {
		return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
	}
	return gctx.Err() != nil
}
