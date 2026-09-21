package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vi-dev/nem/internal/archive"
	"github.com/vi-dev/nem/internal/build"
	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/config"
	"github.com/vi-dev/nem/internal/pkgtest"
	"github.com/vi-dev/nem/internal/spec"
)

type buildInput struct {
	packages []string
	missing  bool
	withDeps bool
	push     bool
	dryRun   bool
	force    bool
}

func newCatalogBuildCmd() *cobra.Command {
	var in buildInput
	cmd := &cobra.Command{
		Use:   "build [catalog]",
		Short: "Build a catalog's compile-from-source packages on the host platform",
		Long: "Build and (optionally) push compile-from-source packages on the host platform.\n" +
			"The catalog supplies the package manifests and, with --push, receives the archives.",
		Args: cobra.MaximumNArgs(1),
		ValidArgsFunction: firstArgOnly(func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveFilterDirs
		}),
		RunE: func(cmd *cobra.Command, args []string) error {
			cat := "."
			if len(args) == 1 {
				cat = args[0]
			}
			return runCatalogBuild(cmd, cat, in)
		},
	}
	cmd.Flags().StringArrayVar(&in.packages, "package", nil,
		"build name[@version] (repeatable; omitted version means latest; default: every buildable package)")
	_ = cmd.RegisterFlagCompletionFunc("package", completeCatalogDirPackages)
	cmd.Flags().BoolVar(&in.missing, "missing", false,
		"select every version and platform of the selected packages whose archive is missing")
	cmd.Flags().BoolVar(&in.withDeps, "with-deps", false,
		"also build every package's dependency whose archive is missing")
	cmd.Flags().BoolVar(&in.push, "push", false, "publish built archives into the catalog")
	cmd.Flags().BoolVar(&in.dryRun, "dry-run", false, "report the plan without building")
	cmd.Flags().BoolVar(&in.force, "force", false, "with --push, overwrite existing archives")
	return cmd
}

func runCatalogBuild(cmd *cobra.Command, cat string, in buildInput) error {
	if in.withDeps && len(in.packages) == 0 {
		return errors.New("--with-deps needs at least one --package")
	}

	ctx := cmd.Context()
	target, err := build.OpenTarget(ctx, cat)
	if err != nil {
		return err
	}
	if target.File {
		return errors.New("catalog build takes a catalog directory or OCI ref, not a pkg.yaml")
	}

	cfg, err := config.OpenConfig(nemHome)
	if err != nil {
		return err
	}
	configured, err := catalog.OpenConfigured(cfg, nemHome)
	if err != nil {
		return err
	}

	roots, err := build.Select(ctx, target, in.packages)
	if err != nil {
		return err
	}
	var sels []build.Selection
	switch {
	case in.missing:
		refs, err := parsePackageRefs(in.packages)
		if err != nil {
			return err
		}
		missing, stats, err := build.SelectMissing(ctx, target, refs)
		if err != nil {
			return err
		}
		console.Info("Checked %d oci packages: %d incomplete versions, %d prebuilt skipped",
			stats.Checked, len(missing), stats.Skipped)
		sels = missing
	case len(roots) > 0:
		sels = roots
	default:
		if sels, err = build.SelectAll(ctx, target); err != nil {
			return err
		}
	}
	if in.withDeps {
		deps, err := build.SelectDeps(ctx, target, configured, roots)
		if err != nil {
			return err
		}
		sels = build.Merge(sels, deps)
	}
	plan, err := build.ComputePlan(sels)
	if err != nil {
		return err
	}
	if len(plan.Entries) == 0 {
		console.Success("Nothing to build")
		return nil
	}

	if in.dryRun {
		renderPlan(plan)
		return narrateDryRunPushes(plan, target, in.push)
	}

	sources := configured.Prepend(target.Entry)
	opts := build.BatchOptions{Target: target, Push: in.push, Force: in.force,
		TestFor: batchTestFor(sources)}
	rows, err := build.RunBatch(ctx, nemHome, sources, plan, opts)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		console.Success("Nothing to build on %s", spec.Current())
		return nil
	}
	return renderSummary(rows)
}

var runPkgTest = pkgtest.InstallAndRun

func batchTestFor(set *catalog.Set) func(
	*spec.Package, *archive.Dir) func(context.Context, *spec.Package, string, string) error {
	return func(pkg *spec.Package, store *archive.Dir) func(context.Context, *spec.Package, string, string) error {
		if len(pkg.Test) == 0 {
			return nil
		}
		return func(ctx context.Context, p *spec.Package, v, artifactPath string) error {
			deps, err := build.ResolveDeps(ctx, nemHome, set, p, p.Deps, store)
			if err != nil {
				return err
			}
			return runPkgTest(ctx, nemHome, deps, p, v, "", artifactPath)
		}
	}
}

func renderPlan(plan build.Plan) {
	rows := make([][]string, len(plan.Entries))
	for i, e := range plan.Entries {
		needs := "-"
		if len(e.Needs) > 0 {
			needs = strings.Join(e.Needs, ",")
		}
		plats := make([]string, len(e.Platforms))
		for j, p := range e.Platforms {
			plats[j] = p.String()
		}
		rows[i] = []string{strconv.Itoa(e.Wave), e.Pkg.Name, e.Version, e.Reason, needs, strings.Join(plats, ",")}
	}
	console.Table([]string{"WAVE", "PACKAGE", "VERSION", "REASON", "NEEDS", "PLATFORMS"}, rows)
}

func narrateDryRunPushes(plan build.Plan, t *build.Target, push bool) error {
	if !push {
		return nil
	}
	plat := spec.Current()
	for _, e := range plan.Entries {
		if !slices.Contains(e.Platforms, plat) {
			continue
		}
		ref, err := archiveTargetRef(t, e.Pkg.Name)
		if err != nil {
			return err
		}
		console.Info("Dry-run: would push %s:%s (%s)", ref, e.Version, plat)
	}
	return nil
}

func archiveTargetRef(t *build.Target, name string) (string, error) {
	if t.IsDir() {
		return filepath.Join(t.Dir, "archives", name), nil
	}
	return archive.Ref(t.Ref, name)
}

func renderSummary(rows []build.SummaryRow) error {
	table := make([][]string, len(rows))
	for i, r := range rows {
		table[i] = []string{r.Name, r.Version, r.Result, r.Detail}
	}
	console.Table([]string{"PACKAGE", "VERSION", "RESULT", "DETAIL"}, table)

	built, pushed, failed, skipped, pushFailed, err := build.Verdict(rows)
	verdict := fmt.Sprintf("%d built (%d pushed), %d failed, %d skipped", built, pushed, failed, skipped)
	if failed+skipped > 0 || pushFailed > 0 {
		console.Warn("%s", verdict)
	} else {
		console.Success("%s", verdict)
	}
	if pushFailed > 0 {
		console.Hint("Re-run with --missing to retry failed pushes")
	}
	return err
}
