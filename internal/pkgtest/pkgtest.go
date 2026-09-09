package pkgtest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/vi-dev/nem/internal/build"
	"github.com/vi-dev/nem/internal/envx"
	"github.com/vi-dev/nem/internal/fsx"
	"github.com/vi-dev/nem/internal/home"
	"github.com/vi-dev/nem/internal/install"
	"github.com/vi-dev/nem/internal/report"
	"github.com/vi-dev/nem/internal/spec"
	"github.com/vi-dev/nem/internal/usage"
)

func InstallAndRun(ctx context.Context, h home.Home, deps []build.ResolvedDep,
	pkg *spec.Package, version, catalogName, artifactPath string,
	rep report.Reporter, stdout, stderr io.Writer) (err error) {

	plat := spec.Current()

	var steps []spec.TestStep
	if spec.PlatformsInclude(pkg.Platforms, plat) {
		for _, s := range pkg.Test {
			if spec.PlatformsInclude(s.Platforms, plat) {
				steps = append(steps, s)
			}
		}
	}
	if len(steps) == 0 {
		if len(pkg.Test) > 0 {
			rep.Info("No test of %s applies to %s; nothing asserted", pkg.Name, plat)
		}
		return nil
	}

	if mkdirErr := os.MkdirAll(h.Packages(), 0o755); mkdirErr != nil {
		return fmt.Errorf("create packages dir: %w", mkdirErr)
	}
	aliasDir, mkTempErr := os.MkdirTemp(h.Packages(), pkg.Name+home.TestInstallInfix+"*")
	if mkTempErr != nil {
		return fmt.Errorf("create test install dir: %w", mkTempErr)
	}
	aliasName := filepath.Base(aliasDir)

	defer func() {
		if rmErr := os.RemoveAll(aliasDir); rmErr != nil {
			err = errors.Join(err, fmt.Errorf("remove test install %s: %w (remove it by hand)", aliasDir, rmErr))
		}
		key := usage.Key(aliasName, version)
		if rowErr := removeUsageRow(h, key); rowErr != nil {
			err = errors.Join(err, fmt.Errorf("remove usage row %s: %w", key, rowErr))
		}
	}()

	alias := *pkg
	alias.Name = aliasName
	if installErr := install.Install(ctx, h, &alias, version, catalogName, artifactPath); installErr != nil {
		return fmt.Errorf("test-install %s@%s: %w", pkg.Name, version, installErr)
	}
	prefix := filepath.Join(aliasDir, version)

	self := build.ResolvedDep{
		Name: alias.Name, Version: version, Prefix: prefix,
		OnPath: true, OnLoaderPath: true, Bins: pkg.Bins, Libs: pkg.Libs,
	}
	entries := append([]build.ResolvedDep{self}, deps...)
	env := build.ComposeEnv(os.Environ(), entries,
		build.EnvContext{Version: version, Platform: plat, Prefix: prefix, AbsRpath: true})

	env = applyPackageEnv(pkg, prefix, version, plat, env, rep)

	if mkdirErr := os.MkdirAll(h.Tmp(), 0o755); mkdirErr != nil {
		return fmt.Errorf("create tmp dir: %w", mkdirErr)
	}

	scratch, scratchErr := os.MkdirTemp(h.Tmp(), pkg.Name+home.BuildStagingInfix+"test-*")
	if scratchErr != nil {
		return fmt.Errorf("create test scratch dir: %w", scratchErr)
	}
	defer os.RemoveAll(scratch)

	env = build.ScrubEnv(env,
		[]string{"PWD", "LD_LIBRARY_PATH", "DYLD_LIBRARY_PATH", "NEM_STAGING_DIR", "NEM_OUTPUT"},
		"PWD="+scratch)

	var loaderPrologue string
	if dirs := loaderPathDirs(entries); len(dirs) > 0 {
		value := strings.Join(dirs, string(filepath.ListSeparator))
		loaderPrologue = fmt.Sprintf("export %s=%s\n", envx.LoaderPathVar(), shellQuote(value))
	}

	for i, s := range steps {
		c := exec.CommandContext(ctx, "sh", "-c", loaderPrologue+s.Run)
		c.Dir = scratch
		c.Env = env
		c.Stdout = stdout
		c.Stderr = stderr
		if runErr := c.Run(); runErr != nil {
			return fmt.Errorf("test step %d (%q): %w", i+1, s.Run, runErr)
		}
	}
	noun := "steps"
	if len(steps) == 1 {
		noun = "step"
	}
	rep.Info("Tested %s %s (%d %s)", pkg.Name, version, len(steps), noun)
	return nil
}

func removeUsageRow(h home.Home, key string) error {
	release, err := fsx.Lock(h.LockFile())
	if err != nil {
		return err
	}
	defer release()

	idx := usage.Load(h)
	if _, ok := idx[key]; !ok {
		return nil
	}
	delete(idx, key)
	return usage.Save(h, idx)
}

func applyPackageEnv(pkg *spec.Package, installDir, version string, plat spec.Platform, env []string, rep report.Reporter) []string {
	vars := map[string]string{}
	for _, export := range pkg.Env {
		if !spec.PlatformsInclude(export.Platforms, plat) {
			continue
		}
		if envx.IsReserved(export.Name) {
			rep.Warn("reserved env var %q from %s skipped", export.Name, pkg.Name)
			continue
		}
		value, err := envx.RenderEnvTemplate(export.Value, installDir, version)
		if err != nil {
			rep.Warn("env %q from %s: %v", export.Name, pkg.Name, err)
			continue
		}
		vars[export.Name] = value
	}
	if len(vars) == 0 {
		return env
	}
	drop := make([]string, 0, len(vars))
	overrides := make([]string, 0, len(vars))
	for name, value := range vars {
		drop = append(drop, name)
		overrides = append(overrides, name+"="+value)
	}
	return build.ScrubEnv(env, drop, overrides...)
}

func loaderPathDirs(entries []build.ResolvedDep) []string {
	seen := map[string]bool{}
	var out []string
	for _, d := range entries {
		if !d.OnLoaderPath {
			continue
		}
		for _, lib := range d.Libs {
			dir := filepath.Join(d.Prefix, lib)
			if seen[dir] {
				continue
			}
			seen[dir] = true
			out = append(out, dir)
		}
	}
	return out
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
