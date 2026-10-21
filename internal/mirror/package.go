package mirror

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/vi-dev/clix/console"
	"oras.land/oras-go/v2"

	"github.com/vi-dev/nem/internal/archive"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/spec"
)

func mirrorPackage(ctx context.Context, opts Options, manifest ocix.TitledManifest, store *ocix.Store, srcArchives archive.Store, dstArchives archive.ReadWriteStore, agg *aggregator) {
	rep := console.FromContext(ctx)
	label := fmt.Sprintf("Mirroring %s", manifest.Title)
	failedOutcome := fmt.Sprintf("Failed to mirror %s", manifest.Title)

	task := rep.Start(label)
	task.Phase("probing")

	files, _, err := store.Files(ctx, manifest.Title)
	if err != nil {
		agg.failed.Add(1)
		rep.Warnf("%s: %v", manifest.Title, err)
		task.Fail(failedOutcome)
		return
	}
	pkg, err := spec.ParseFiles(files)
	if err != nil {
		agg.failed.Add(1)
		rep.Warnf("%s: %v", manifest.Title, err)
		task.Fail(failedOutcome)
		return
	}

	if len(pkg.Versions) == 0 {
		task.Discard()
		return
	}
	plans := planVersions(pkg)
	if len(plans) == 0 {
		task.Discard()
		return
	}

	src, err := srcArchives.Open(ctx, manifest.Title)
	if err != nil {
		agg.failed.Add(1)
		rep.Warnf("%s: %v", manifest.Title, err)
		task.Fail(failedOutcome)
		return
	}
	dst, err := dstArchives.OpenRW(ctx, manifest.Title)
	if err != nil {
		agg.failed.Add(1)
		rep.Warnf("%s: %v", manifest.Title, err)
		task.Fail(failedOutcome)
		return
	}

	copied := 0
	failed := false
	for _, p := range plans {
		if ctx.Err() != nil {
			break
		}
		if p.err != nil {
			rep.Warnf("%s %s: %v", manifest.Title, p.version, p.err)
			agg.failed.Add(1)
			failed = true
			continue
		}
		outcome := mirrorVersion(ctx, src, dst, manifest.Title, p.version, p.prebuilt, opts.DryRun, task)
		if outcome == outcomeCancelled {
			break
		}
		switch outcome {
		case outcomeCopied:
			agg.copied.Add(1)
			copied++
		case outcomeFailed:
			agg.failed.Add(1)
			failed = true
		}
	}

	switch {
	case ctx.Err() != nil:
		task.Fail(fmt.Sprintf("Cancelled %s", manifest.Title))
	case failed:
		task.Fail(failedOutcome)
	case copied > 0:
		verb := "Mirrored"
		if opts.DryRun {
			verb = "Would mirror"
		}
		task.Done(fmt.Sprintf("%s %s (%s)", verb, manifest.Title, console.Plural(copied, "tag")))
	default:
		task.Discard()
	}
}

type versionPlan struct {
	version  string
	prebuilt bool
	err      error
}

func planVersions(pkg *spec.Package) []versionPlan {
	var plans []versionPlan
	for _, v := range pkg.Offered() {
		r, err := pkg.Recipe(v.Version)
		if err != nil {
			plans = append(plans, versionPlan{version: v.Version, err: err})
			continue
		}
		if participates, prebuilt := classify(r); participates {
			plans = append(plans, versionPlan{version: v.Version, prebuilt: prebuilt})
		}
	}
	return plans
}

func classify(r *spec.Recipe) (participates, prebuilt bool) {
	if r.Artifact.OCI != "" {
		return ociRefIsRelative(r.Artifact.OCI), true
	}
	return true, false
}

func ociRefIsRelative(tmpl string) bool {
	return tmpl == "" || strings.HasPrefix(tmpl, ":") || strings.HasPrefix(tmpl, "@")
}

type versionOutcome int

const (
	outcomePresent versionOutcome = iota
	outcomeUnfilled
	outcomeCopied
	outcomeFailed
	outcomeCancelled
)

func mirrorVersion(ctx context.Context, src oras.ReadOnlyTarget, dst oras.Target, name, version string, prebuilt, dryRun bool, task console.Task) versionOutcome {
	rep := console.FromContext(ctx)
	srcDesc, err := archive.ResolveIndex(ctx, src, version)
	switch {
	case errors.Is(err, archive.ErrNotFound):
		if prebuilt {
			rep.Warnf("%s %s: archive missing from source", name, version)
			return outcomeFailed
		}
		return outcomeUnfilled
	case err != nil:
		if console.IsCancellation(err) {
			return outcomeCancelled
		}
		rep.Warnf("%s %s: %v", name, version, err)
		return outcomeFailed
	}

	dstDesc, err := archive.ResolveIndex(ctx, dst, version)
	switch {
	case err == nil:
		if dstDesc.Digest == srcDesc.Digest {
			return outcomePresent
		}
	case !errors.Is(err, archive.ErrNotFound):
		if console.IsCancellation(err) {
			return outcomeCancelled
		}
		rep.Warnf("%s %s: %v", name, version, err)
		return outcomeFailed
	}

	if dryRun {
		task.Phase(fmt.Sprintf("would copy %s", version))
		return outcomeCopied
	}
	task.Phase(fmt.Sprintf("copying %s", version))
	if _, err := ocix.CopyTag(ctx, src, dst, version); err != nil {
		if console.IsCancellation(err) {
			return outcomeCancelled
		}
		rep.Warnf("%s %s: %v", name, version, err)
		return outcomeFailed
	}
	return outcomeCopied
}
