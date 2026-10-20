package build

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/vi-dev/clix/console"

	"github.com/vi-dev/nem/internal/archive"
	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/fetch"
	"github.com/vi-dev/nem/internal/home"
	"github.com/vi-dev/nem/internal/install"
	"github.com/vi-dev/nem/internal/netx"
	"github.com/vi-dev/nem/internal/resolve"
	"github.com/vi-dev/nem/internal/spec"
	"github.com/vi-dev/nem/internal/usage"
)

type Options struct {
	Version string

	Test func(ctx context.Context, task console.Task, pkg *spec.Package, version, artifactPath string) error

	LocalStore *archive.Dir
}

func Build(ctx context.Context, h home.Home, set *catalog.Set, pkg *spec.Package,
	opts Options) error {
	rep := console.FromContext(ctx)
	if err := pkg.Validate(); err != nil {
		return err
	}

	version := opts.Version
	if version == "" {
		version = pkg.Versions[0].Version
	}
	r, err := pkg.Recipe(version)
	if err != nil {
		return err
	}
	if r.Build == nil {
		return errors.New("package has no build section")
	}

	task := rep.Start(fmt.Sprintf("Building %s %s", pkg.Name, version))
	err = buildPackage(ctx, h, set, pkg, r, version, opts, task)
	return task.End(ctx, err, console.Outcomes{
		Done:      fmt.Sprintf("Built %s %s", pkg.Name, version),
		Failed:    fmt.Sprintf("Failed to build %s %s", pkg.Name, version),
		Cancelled: fmt.Sprintf("Cancelled %s %s", pkg.Name, version),
	})
}

func buildPackage(ctx context.Context, h home.Home, set *catalog.Set, pkg *spec.Package, r *spec.Recipe,
	version string, opts Options, task console.Task) error {
	var steps []spec.BuildStep
	for _, step := range r.Build.Steps {
		if spec.PlatformsInclude(step.Platforms, spec.Current()) {
			steps = append(steps, step)
		}
	}
	if len(steps) == 0 {
		return fmt.Errorf("no build step applies to %s", spec.Current())
	}

	if err := os.MkdirAll(h.Tmp(), 0o755); err != nil {
		return fmt.Errorf("create tmp dir: %w", err)
	}
	staging, err := os.MkdirTemp(h.Tmp(), pkg.Name+home.BuildStagingInfix+"*")
	if err != nil {
		return fmt.Errorf("create staging dir: %w", err)
	}

	path, sha, verified, err := fetchBuildSource(ctx, pkg, version, staging, task)
	if err != nil {
		return err
	}
	if !verified {
		task.Infof("Record for reproducibility: sourceSha256: %s", sha)
	}

	srcDir := filepath.Join(staging, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		return fmt.Errorf("create source dir: %w", err)
	}
	task.Phase("unpacking")
	srcRoot, err := unpackSource(path, srcDir, sourceSingleName(pkg, version))
	if err != nil {
		return err
	}

	if len(r.Build.Deps) > 0 {
		task.Phase("installing deps")
	}
	deps, err := ResolveDeps(ctx, h, set, pkg.Name, r.Platforms, r.Build.Deps, opts.LocalStore)
	if err != nil {
		return err
	}

	prefix, err := h.PackageDir(pkg.Name, version)
	if err != nil {
		return fmt.Errorf("package dir for %s@%s: %w", pkg.Name, version, err)
	}
	outputDir := filepath.Join(srcRoot, r.Build.Output)
	env := ComposeEnv(os.Environ(), deps, EnvContext{
		Version: version, Platform: spec.Current(), Prefix: prefix, StagingDir: staging, OutputDir: outputDir,
	})

	env = ScrubEnv(env, []string{"PWD"}, "PWD="+srcRoot)

	for i, step := range steps {
		task.Phase(fmt.Sprintf("step %d/%d", i+1, len(steps)))
		c := exec.CommandContext(ctx, "sh", "-c", step.Run)
		c.Dir = srcRoot
		c.Env = env
		c.Stdout = task.Out()
		c.Stderr = task.ErrOut()
		if err := c.Run(); err != nil {
			return fmt.Errorf("build step %d (%q): %w", i+1, step.Run, err)
		}
	}

	if n := r.Build.Normalize; n == nil || *n {
		task.Phase("normalizing")
		if err := normalizeOutput(outputDir); err != nil {
			return fmt.Errorf("normalize output: %w", err)
		}
	}

	task.Phase("verifying")
	packagesRoot := h.Packages()
	vs, err := VerifyConformance(outputDir, []string{staging, packagesRoot})
	if err != nil {
		return fmt.Errorf("verify %s: %w", outputDir, err)
	}
	if len(vs) > 0 {
		return conformanceError(vs)
	}
	warnUnlinkedDepRpaths(task, pkg.Name, version, r, deps)

	var data []byte
	if opts.Test != nil || opts.LocalStore != nil {
		var buf bytes.Buffer
		if err := tarGzDir(&buf, outputDir); err != nil {
			return fmt.Errorf("archive %s: %w", outputDir, err)
		}
		data = buf.Bytes()
	}

	if err := os.RemoveAll(staging); err != nil {
		task.Warnf("sweep build staging: %v", err)
	}

	if opts.Test != nil {
		tmpArchive, err := writeTempArchive(h, pkg.Name, data)
		if err != nil {
			return err
		}

		defer os.Remove(tmpArchive)
		task.Phase("testing")
		if err := opts.Test(ctx, task, pkg, version, tmpArchive); err != nil {
			return err
		}
	}

	if opts.LocalStore != nil {
		task.Phase("staging archive")
		target, err := opts.LocalStore.OpenRW(ctx, pkg.Name)
		if err != nil {
			return fmt.Errorf("stage archive locally: %w", err)
		}
		if _, _, err := archive.Push(ctx, target, version, spec.Current(), archive.BytesBlob(data), true); err != nil {
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

func fetchBuildSource(ctx context.Context, pkg *spec.Package, version, staging string, task console.Task) (path, sha string, verified bool, err error) {
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

	task.Phase("downloading source")
	return fetchSource(ctx, netx.Client(), url, want, staging,
		fetch.Meta{Name: pkg.Name, Version: version, Platform: spec.Current()}, task)
}

func ResolveDeps(ctx context.Context, h home.Home, set *catalog.Set,
	name string, platforms []spec.Platform, deps []spec.Dep, local *archive.Dir) ([]ResolvedDep, error) {
	if len(deps) == 0 {
		return nil, nil
	}
	result, err := resolve.Dependencies(ctx, name, platforms, deps, set)
	if err != nil {
		return nil, err
	}
	return InstallResolvedDeps(ctx, h, set, result, local)
}

func InstallResolvedDeps(ctx context.Context, h home.Home, set *catalog.Set,
	result *resolve.Result, local *archive.Dir) ([]ResolvedDep, error) {
	jobs, err := install.Jobs(result, set, local)
	if err != nil {
		return nil, err
	}
	if err := install.Run(ctx, h, jobs); err != nil {
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

func warnUnlinkedDepRpaths(lines console.Lines, pkgName, version string, r *spec.Recipe, deps []ResolvedDep) {
	plat := spec.Current()
	linked := make(map[string]bool, len(r.Deps))
	for _, d := range r.Deps {
		if d.Kind == spec.DepKindLink && spec.PlatformsInclude(d.Platforms, plat) {
			linked[d.Name] = true
		}
	}
	loaderPath := make(map[string]bool, len(deps))
	for _, d := range deps {
		if d.OnLoaderPath {
			loaderPath[d.Name] = true
		}
	}
	unlinked := map[string]bool{}
	for _, d := range r.Build.Deps {
		if d.Kind == spec.DepKindLink && spec.PlatformsInclude(d.Platforms, plat) && !linked[d.Name] && loaderPath[d.Name] {
			unlinked[d.Name] = true
		}
	}
	for _, name := range slices.Sorted(maps.Keys(unlinked)) {
		lines.Warnf("%s %s: rpath links %s without a runtime link dependency", pkgName, version, name)
	}
}

func conformanceError(vs []Violation) error {
	var msg strings.Builder
	fmt.Fprintf(&msg, "%s:", console.Plural(len(vs), "conformance violation"))
	for _, v := range vs {
		fmt.Fprintf(&msg, "\n  %s: %s (%s)", v.File, v.Ref, v.Reason)
	}
	return errors.New(msg.String())
}
