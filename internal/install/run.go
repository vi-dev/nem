package install

import (
	"context"
	"fmt"
	"maps"
	"os"
	"runtime"
	"slices"

	"github.com/vi-dev/clix/console"
	"golang.org/x/sync/errgroup"

	"github.com/vi-dev/nem/internal/fetch"
	"github.com/vi-dev/nem/internal/fsx"
	"github.com/vi-dev/nem/internal/home"
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
	rep := console.FromContext(gctx)
	name, version := job.Pkg.Name, job.Version

	label := fmt.Sprintf("Installing %s %s", name, version)
	o := console.Outcomes{
		Done:      fmt.Sprintf("Installed %s %s", name, version),
		Failed:    fmt.Sprintf("Failed to install %s %s", name, version),
		Cancelled: fmt.Sprintf("Cancelled %s %s", name, version),
	}

	task := rep.Start(label)
	if gctx.Err() != nil {
		return task.End(gctx, gctx.Err(), o)
	}

	task.Phase("downloading")
	artifact, err := acquire(gctx, job.Pkg, version, spec.Current(), job.Source, h.Tmp(), task)
	if err != nil {
		return task.End(context.WithoutCancel(gctx), err, o)
	}

	defer os.Remove(artifact)

	task.Phase("extracting")
	err = Install(gctx, h, job.Pkg, version, job.Catalog, artifact, job.Reinstall, job.Links)
	return task.End(context.WithoutCancel(gctx), err, o)
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
	rep := console.FromContext(ctx)
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
			rep.Debugf("Relinked %s %s: %s %s → %s", name, version, dep, retargeted[dep], job.Links[dep])
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
