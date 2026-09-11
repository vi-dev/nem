package main

import (
	"os"
	"path/filepath"

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
	var version string
	cmd := &cobra.Command{
		Use:               "test <pkg.yaml>",
		Short:             "Install a package and run its declared test steps",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: firstArgOnly(completeYAMLFiles),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := readFile(args[0])
			if err != nil {
				return err
			}
			pkg, err := spec.Parse(data)
			if err != nil {
				return err
			}
			if err := pkg.Validate(); err != nil {
				return err
			}
			if len(pkg.Test) == 0 {
				console.Info("%s declares no tests", pkg.Name)
				return nil
			}

			if plat := spec.Current(); !spec.PlatformsInclude(pkg.Platforms, plat) {
				console.Info("%s does not support %s", pkg.Name, plat)
				return nil
			}
			cfg, err := config.OpenConfig(nemHome)
			if err != nil {
				return err
			}
			sources, err := catalog.Open(cfg, nemHome)
			if err != nil {
				return err
			}
			sources = append(sources, manifestSource(args[0], pkg))

			result, err := resolve.Resolve(cmd.Context(),
				[]resolve.Tool{{Key: project.ToolKey{Name: pkg.Name}, Version: version}}, sources)
			if err != nil {
				return err
			}
			jobs := currentPlatformJobs(cfg, result)
			var root *install.Job
			for i := range jobs {
				if jobs[i].Pkg.Name == pkg.Name {
					root = &jobs[i]
					break
				}
			}
			if root == nil {
				console.Info("%s installs nothing on %s; nothing to test", pkg.Name, spec.Current())
				return nil
			}
			resolved := root.Version

			depsResult := &resolve.Result{Pkgs: result.Pkgs}
			for _, e := range result.Entries {
				if e.Name == pkg.Name {
					continue
				}
				depsResult.Entries = append(depsResult.Entries, e)
			}
			deps, err := build.InstallResolvedDeps(cmd.Context(), nemHome, cfg, console, depsResult)
			if err != nil {
				return err
			}

			task := console.Task("Downloading " + pkg.Name + " " + resolved)
			artifact, err := fetch.Acquire(cmd.Context(), root.Pkg, resolved, spec.Current(),
				root.Source, nemHome.Tmp(), task)
			if err != nil {
				task.Fail("Failed to download " + pkg.Name + " " + resolved)
				return err
			}
			task.Done("Downloaded " + pkg.Name + " " + resolved)
			defer os.Remove(artifact)

			return pkgtest.InstallAndRun(cmd.Context(), nemHome, deps, pkg, resolved,
				root.Catalog, artifact, console, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&version, "version", "", "version to test (default: the catalog's latest)")
	return cmd
}

func manifestSource(path string, pkg *spec.Package) catalog.Named {
	if abs, err := filepath.Abs(path); err == nil {
		pkgDir := filepath.Dir(abs)
		if filepath.Base(pkgDir) == pkg.Name && filepath.Base(filepath.Dir(pkgDir)) == "pkgs" {
			root := filepath.Dir(filepath.Dir(pkgDir))
			return catalog.Named{Name: root, Source: catalog.NewDir(root)}
		}
	}
	return catalog.Named{Name: path, Source: catalog.NewFile(pkg)}
}
