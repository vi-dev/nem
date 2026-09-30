package install

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"runtime"
	"slices"

	"golang.org/x/sync/errgroup"

	"github.com/vi-dev/nem/internal/fetch"
	"github.com/vi-dev/nem/internal/fsx"
	"github.com/vi-dev/nem/internal/home"
	"github.com/vi-dev/nem/internal/report"
	"github.com/vi-dev/nem/internal/spec"
)

type Job struct {
	Pkg       *spec.Package
	Version   string
	Catalog   string
	Source    fetch.Source
	Reinstall bool
	Links     map[string]string
}

var acquire = fetch.Acquire

func Run(ctx context.Context, h home.Home, jobs []Job) error {
	if err := os.MkdirAll(h.Tmp(), 0o755); err != nil {
		return fmt.Errorf("create tmp dir: %w", err)
	}

	var installs, relinks []Job
	for _, job := range jobs {
		if !job.Reinstall && IsInstalled(h, job.Pkg.Name, job.Version) {
			if !NeedsRelayout(h, job.Pkg.Name, job.Version, job.Links) {
				relinks = append(relinks, job)
				continue
			}
			job.Reinstall = true
		}
		installs = append(installs, job)
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(min(runtime.NumCPU(), 8))

	for _, job := range installs {
		g.Go(func() error { return runJob(gctx, h, job) })
	}
	if err := g.Wait(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return relinkAll(ctx, h, relinks)
}

func runJob(gctx context.Context, h home.Home, job Job) error {
	rep := report.FromContext(gctx)
	name, version := job.Pkg.Name, job.Version

	label := fmt.Sprintf("Installing %s %s", name, version)
	cancelled := fmt.Sprintf("Cancelled %s %s", name, version)

	failedOutcome := fmt.Sprintf("Failed to install %s %s", name, version)

	task := rep.Task(label)
	if isCancellation(gctx, nil) {
		task.Fail(cancelled)
		return nil
	}

	task.Segment("downloading")
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

	task.Segment("extracting")
	if err := Install(gctx, h, job.Pkg, version, job.Catalog, artifact, job.Reinstall, job.Links); err != nil {
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

func relinkAll(ctx context.Context, h home.Home, jobs []Job) error {
	if len(jobs) == 0 {
		return nil
	}
	release, err := fsx.Lock(h.LockFile())
	if err != nil {
		return err
	}
	defer release()
	rep := report.FromContext(ctx)
	for _, job := range jobs {
		name, version := job.Pkg.Name, job.Version
		dir, err := h.PackageDir(name, version)
		if err != nil {
			return fmt.Errorf("relink %s@%s: %w", name, version, err)
		}
		retargeted, err := WriteLinks(dir, job.Links)
		if err != nil {
			return fmt.Errorf("relink %s@%s: %w", name, version, err)
		}
		for _, dep := range slices.Sorted(maps.Keys(retargeted)) {
			rep.Debug("Relinked %s %s: %s %s → %s", name, version, dep, retargeted[dep], job.Links[dep])
		}
	}
	return nil
}

func NeedsRelayout(h home.Home, name, version string, links map[string]string) bool {
	if len(links) == 0 {
		return false
	}
	meta, err := ReadMeta(h, name, version)
	return err != nil || meta.Layout < LayoutVersion
}

func isCancellation(gctx context.Context, err error) bool {
	if err != nil {
		return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
	}
	return gctx.Err() != nil
}
