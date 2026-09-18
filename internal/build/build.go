package build

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/fetch"
	"github.com/vi-dev/nem/internal/home"
	"github.com/vi-dev/nem/internal/install"
	"github.com/vi-dev/nem/internal/netx"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/report"
	"github.com/vi-dev/nem/internal/resolve"
	"github.com/vi-dev/nem/internal/spec"
	"github.com/vi-dev/nem/internal/usage"
)

type Options struct {
	Version string

	Test func(ctx context.Context, pkg *spec.Package, version, artifactPath string) error

	LocalStore *ocix.ArchiveStore
}

var archivesOpener = ocix.RemoteArchivesRW

func Build(ctx context.Context, h home.Home, set *catalog.Set, pkg *spec.Package,
	opts Options) error {
	rep := report.FromContext(ctx)
	if pkg.Build == nil {
		return errors.New("package has no build section")
	}
	if err := pkg.Validate(); err != nil {
		return err
	}

	version := opts.Version
	if version == "" {
		version = pkg.Versions[0].Version
	}

	if err := os.MkdirAll(h.Tmp(), 0o755); err != nil {
		return fmt.Errorf("create tmp dir: %w", err)
	}
	staging, err := os.MkdirTemp(h.Tmp(), pkg.Name+home.BuildStagingInfix+"*")
	if err != nil {
		return fmt.Errorf("create staging dir: %w", err)
	}

	path, sha, verified, err := fetchBuildSource(ctx, pkg, version, staging)
	if err != nil {
		return err
	}
	if !verified {
		rep.Info("Record for reproducibility: sourceSha256: %s", sha)
	}

	srcDir := filepath.Join(staging, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		return fmt.Errorf("create source dir: %w", err)
	}
	srcRoot, err := unpackSource(path, srcDir, sourceSingleName(pkg, version))
	if err != nil {
		return err
	}

	deps, err := ResolveDeps(ctx, h, set, pkg, pkg.Build.Deps, opts.LocalStore)
	if err != nil {
		return err
	}

	prefix, err := h.PackageDir(pkg.Name, version)
	if err != nil {
		return fmt.Errorf("package dir for %s@%s: %w", pkg.Name, version, err)
	}
	outputDir := filepath.Join(srcRoot, pkg.Build.Output)
	env := ComposeEnv(os.Environ(), deps, EnvContext{
		Version: version, Platform: spec.Current(), Prefix: prefix, StagingDir: staging, OutputDir: outputDir,
	})

	env = ScrubEnv(env, []string{"PWD"}, "PWD="+srcRoot)

	ran := 0
	for i, step := range pkg.Build.Steps {
		if !spec.PlatformsInclude(step.Platforms, spec.Current()) {
			continue
		}
		c := exec.CommandContext(ctx, "sh", "-c", step.Run)
		c.Dir = srcRoot
		c.Env = env
		c.Stdout = rep.Out()
		c.Stderr = rep.ErrOut()
		if err := c.Run(); err != nil {
			return fmt.Errorf("build step %d (%q): %w", i+1, step.Run, err)
		}
		ran++
	}

	if ran == 0 {
		return fmt.Errorf("no build step applies to %s", spec.Current())
	}

	if n := pkg.Build.Normalize; n == nil || *n {
		if err := normalizeOutput(outputDir); err != nil {
			return fmt.Errorf("normalize output: %w", err)
		}
	}

	packagesRoot := h.Packages()
	vs, err := VerifyConformance(outputDir, []string{staging, packagesRoot})
	if err != nil {
		return fmt.Errorf("verify %s: %w", outputDir, err)
	}
	if len(vs) > 0 {
		return conformanceError(vs)
	}

	var archive []byte
	if opts.Test != nil || opts.LocalStore != nil {
		var buf bytes.Buffer
		if err := tarGzDir(&buf, outputDir); err != nil {
			return fmt.Errorf("archive %s: %w", outputDir, err)
		}
		archive = buf.Bytes()
	}

	if err := os.RemoveAll(staging); err != nil {
		rep.Warn("sweep build staging: %v", err)
	}

	if opts.Test != nil {
		tmpArchive, err := writeTempArchive(h, pkg.Name, archive)
		if err != nil {
			return err
		}

		defer os.Remove(tmpArchive)
		if err := opts.Test(ctx, pkg, version, tmpArchive); err != nil {
			return err
		}
	}

	if opts.LocalStore != nil {
		target, err := opts.LocalStore.Open(pkg.Name)
		if err != nil {
			return fmt.Errorf("stage archive locally: %w", err)
		}
		if _, _, err := ocix.PushArchive(ctx, target, version, spec.Current(), archive, true); err != nil {
			return fmt.Errorf("stage archive locally: %w", err)
		}
	}

	keys := []string{usage.Key(pkg.Name, version)}
	for _, d := range deps {
		keys = append(keys, usage.Key(d.Name, d.Version))
	}
	usage.Stamp(h, time.Now(), keys)
	return nil
}

func fetchBuildSource(ctx context.Context, pkg *spec.Package, version, staging string) (path, sha string, verified bool, err error) {
	rep := report.FromContext(ctx)
	var want string
	for _, v := range pkg.Versions {
		if v.Version == version {
			want = v.SourceSha256
			break
		}
	}
	url, err := pkg.BuildSourceURL(version, spec.Current())
	if err != nil {
		return "", "", false, err
	}

	label := fmt.Sprintf("Downloading source for %s %s", pkg.Name, version)
	failedOutcome := fmt.Sprintf("Failed to download source for %s %s", pkg.Name, version)
	task := rep.Task(label)
	task.Segment("downloading")

	path, sha, verified, err = fetchSource(ctx, netx.Client(), url, want, staging,
		fetch.Meta{Name: pkg.Name, Version: version, Platform: spec.Current()}, task)
	if err != nil {
		task.Fail(failedOutcome)
		return "", "", false, err
	}
	task.Done(fmt.Sprintf("Downloaded source for %s %s", pkg.Name, version))
	return path, sha, verified, nil
}

func ResolveDeps(ctx context.Context, h home.Home, set *catalog.Set,
	pkg *spec.Package, deps []spec.Dep, local *ocix.ArchiveStore) ([]ResolvedDep, error) {
	if len(deps) == 0 {
		return nil, nil
	}
	result, err := resolve.Dependencies(ctx, pkg, deps, set)
	if err != nil {
		return nil, err
	}
	return InstallResolvedDeps(ctx, h, set, result, local)
}

func InstallResolvedDeps(ctx context.Context, h home.Home, set *catalog.Set,
	result *resolve.Result, local *ocix.ArchiveStore) ([]ResolvedDep, error) {
	if err := install.Run(ctx, h, install.Jobs(result, set, local)); err != nil {
		return nil, err
	}
	current := spec.Current().String()
	var out []ResolvedDep
	for _, e := range result.Entries {
		if !slices.Contains(e.Platforms, current) {
			continue
		}
		prefix, err := h.PackageDir(e.Name, e.Version)
		if err != nil {
			return nil, fmt.Errorf("package dir for %s@%s: %w", e.Name, e.Version, err)
		}
		meta, err := install.ReadMeta(h, e.Name, e.Version)
		if err != nil {
			return nil, err
		}
		out = append(out, ResolvedDep{
			Name: e.Name, Version: e.Version, Prefix: prefix,
			OnPath: e.OnPath, OnLoaderPath: e.OnLoaderPath,
			Bins: meta.Bins, Libs: meta.Libs,
		})
	}
	return out, nil
}

func writeTempArchive(h home.Home, name string, data []byte) (string, error) {
	if err := os.MkdirAll(h.Tmp(), 0o755); err != nil {
		return "", fmt.Errorf("create tmp dir: %w", err)
	}
	f, err := os.CreateTemp(h.Tmp(), name+home.BuildStagingInfix+"archive-*"+home.TmpSuffix)
	if err != nil {
		return "", fmt.Errorf("create archive temp file: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		os.Remove(f.Name())
		return "", fmt.Errorf("write archive temp file: %w", err)
	}
	return f.Name(), nil
}

func conformanceError(vs []Violation) error {
	var msg strings.Builder
	fmt.Fprintf(&msg, "%d conformance violation(s):", len(vs))
	for _, v := range vs {
		fmt.Fprintf(&msg, "\n  %s: %s (%s)", v.File, v.Ref, v.Reason)
	}
	return errors.New(msg.String())
}
