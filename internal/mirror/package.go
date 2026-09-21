package mirror

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"oras.land/oras-go/v2"

	"github.com/vi-dev/nem/internal/archive"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/report"
	"github.com/vi-dev/nem/internal/spec"
)

func mirrorPackage(ctx context.Context, opts Options, manifest ocix.TitledManifest, store *ocix.Store, srcArchives archive.Store, dstArchives archive.ReadWriteStore, agg *aggregator) {
	rep := report.FromContext(ctx)
	label := fmt.Sprintf("Mirroring %s", manifest.Title)
	failedOutcome := fmt.Sprintf("Failed %s", manifest.Title)

	task := rep.Task(label)
	task.Segment("probing")

	data, _, err := store.PkgBytes(ctx, manifest.Title)
	if err != nil {
		agg.failed.Add(1)
		rep.Warn("%s: %v", manifest.Title, err)
		task.Fail(failedOutcome)
		return
	}
	pkg, err := spec.Parse(data)
	if err != nil {
		agg.failed.Add(1)
		rep.Warn("%s: %v", manifest.Title, err)
		task.Fail(failedOutcome)
		return
	}

	participates, prebuilt := classify(pkg)
	if !participates || len(pkg.Versions) == 0 {
		task.Discard()
		return
	}

	src, err := srcArchives.Open(ctx, manifest.Title)
	if err != nil {
		agg.failed.Add(1)
		rep.Warn("%s: %v", manifest.Title, err)
		task.Fail(failedOutcome)
		return
	}
	dst, err := dstArchives.OpenRW(ctx, manifest.Title)
	if err != nil {
		agg.failed.Add(1)
		rep.Warn("%s: %v", manifest.Title, err)
		task.Fail(failedOutcome)
		return
	}

	copied := 0
	failed := false
	for _, v := range pkg.Versions {
		if ctx.Err() != nil {
			break
		}
		outcome := mirrorVersion(ctx, src, dst, manifest.Title, v.Version, prebuilt, opts.DryRun, task)
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
		task.Done(fmt.Sprintf("%s %s (%d tag(s))", verb, manifest.Title, copied))
	default:
		task.Discard()
	}
}

func classify(pkg *spec.Package) (participates, prebuilt bool) {
	if pkg.Artifact.OCI != "" {
		return ociRefIsRelative(pkg.Artifact.OCI), true
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

func mirrorVersion(ctx context.Context, src oras.ReadOnlyTarget, dst oras.Target, name, version string, prebuilt, dryRun bool, task report.Task) versionOutcome {
	rep := report.FromContext(ctx)
	srcDesc, err := archive.ResolveIndex(ctx, src, version)
	switch {
	case errors.Is(err, archive.ErrNotFound):
		if prebuilt {
			rep.Warn("%s %s: archive missing from source", name, version)
			return outcomeFailed
		}
		return outcomeUnfilled
	case err != nil:
		if report.IsCancellation(err) {
			return outcomeCancelled
		}
		rep.Warn("%s %s: %v", name, version, err)
		return outcomeFailed
	}

	dstDesc, err := archive.ResolveIndex(ctx, dst, version)
	switch {
	case err == nil:
		if dstDesc.Digest == srcDesc.Digest {
			return outcomePresent
		}
	case !errors.Is(err, archive.ErrNotFound):
		if report.IsCancellation(err) {
			return outcomeCancelled
		}
		rep.Warn("%s %s: %v", name, version, err)
		return outcomeFailed
	}

	if dryRun {
		task.Segment(fmt.Sprintf("would copy %s", version))
		return outcomeCopied
	}
	task.Segment(fmt.Sprintf("copying %s", version))
	if _, err := ocix.CopyTag(ctx, src, dst, version); err != nil {
		if report.IsCancellation(err) {
			return outcomeCancelled
		}
		rep.Warn("%s %s: %v", name, version, err)
		return outcomeFailed
	}
	return outcomeCopied
}
