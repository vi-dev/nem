package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vi-dev/nem/internal/build"
	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/config"
	"github.com/vi-dev/nem/internal/fetch"
	"github.com/vi-dev/nem/internal/install"
	"github.com/vi-dev/nem/internal/pkgtest"
	"github.com/vi-dev/nem/internal/project"
	"github.com/vi-dev/nem/internal/resolve"
	"github.com/vi-dev/nem/internal/spec"
)

func newCatalogTestCmd() *cobra.Command {
	var packages []string
	cmd := &cobra.Command{
		Use:               "test [catalog]",
		Short:             "Install packages and run their declared test steps",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: firstArgOnly(completeYAMLOrDir),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			catalogArg := "."
			if len(args) == 1 {
				catalogArg = args[0]
			}
			target, err := build.OpenTarget(ctx, catalogArg)
			if err != nil {
				return err
			}
			cfg, err := config.OpenConfig(nemHome)
			if err != nil {
				return err
			}
			sources, err := catalog.OpenConfigured(cfg, nemHome)
			if err != nil {
				return err
			}
			selectors := packages
			if len(selectors) == 0 {
				if selectors, err = target.Entry.Catalog.PackageNames(ctx); err != nil {
					return err
				}
			}
			sels, err := build.Select(ctx, target, selectors)
			if err != nil {
				return err
			}
			sels = build.Merge(sels, nil)
			if len(sels) == 0 {
				console.Info("Nothing to test")
				return nil
			}
			sources = sources.Prepend(target.Entry)
			var tested, failed, skipped int
			for _, sel := range sels {
				ran, err := runPackageTest(cmd, sources, sel.Pkg, sel.Version)
				switch {
				case err != nil:
					failed++
					console.Warn("%s: %v", sel.Pkg.Name, err)
				case ran:
					tested++
				default:
					skipped++
				}
			}
			console.Success("%s", testSummary(tested, failed, skipped))
			if failed > 0 {
				return &ExitError{Code: 1}
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&packages, "package", nil,
		"test name@version package (repeatable; omitted version means latest; default: every package)")
	_ = cmd.RegisterFlagCompletionFunc("package", completeCatalogDirPackages)
	return cmd
}

func testSummary(tested, failed, skipped int) string {
	var parts []string
	if tested > 0 {
		parts = append(parts, fmt.Sprintf("Tested %d packages", tested))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	if skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", skipped))
	}
	if len(parts) == 0 {
		return "Nothing to test"
	}
	return strings.Join(parts, ", ")
}

func runPackageTest(cmd *cobra.Command, sources *catalog.Set, pkg *spec.Package, version string) (bool, error) {
	if plat := spec.Current(); !spec.PlatformsInclude(pkg.Platforms, plat) {
		console.Info("%s does not support %s", pkg.Name, plat)
		return false, nil
	}
	result, err := resolve.Resolve(cmd.Context(),
		[]resolve.Tool{{Key: project.ToolKey{Name: pkg.Name}, Version: version}}, sources)
	if err != nil {
		return false, err
	}
	jobs := install.Jobs(result, sources, nil)
	var root *install.Job
	for i := range jobs {
		if jobs[i].Pkg.Name == pkg.Name {
			root = &jobs[i]
			break
		}
	}
	if root == nil {
		console.Info("%s installs nothing on %s; nothing to test", pkg.Name, spec.Current())
		return false, nil
	}
	resolved := root.Version

	depsResult := &resolve.Result{Pkgs: result.Pkgs}
	for _, e := range result.Entries {
		if e.Name == pkg.Name {
			continue
		}
		depsResult.Entries = append(depsResult.Entries, e)
	}
	deps, err := build.InstallResolvedDeps(cmd.Context(), nemHome, sources, depsResult, nil)
	if err != nil {
		return false, err
	}

	task := console.Task("Downloading " + pkg.Name + " " + resolved)
	artifact, err := fetch.Acquire(cmd.Context(), root.Pkg, resolved, spec.Current(),
		root.Source, nemHome.Tmp(), task)
	if err != nil {
		task.Fail("Failed to download " + pkg.Name + " " + resolved)
		return false, err
	}
	task.Done("Downloaded " + pkg.Name + " " + resolved)
	defer os.Remove(artifact)

	if err := pkgtest.InstallAndRun(cmd.Context(), nemHome, deps, pkg, resolved,
		root.Catalog, artifact); err != nil {
		return false, err
	}
	return true, nil
}
