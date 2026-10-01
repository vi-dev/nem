package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vi-dev/nem/internal/bump"
	"github.com/vi-dev/nem/internal/report"
)

func newCatalogBumpCmd() *cobra.Command {
	var packages []string
	var backfill int
	var dryRun bool
	var output string
	cmd := &cobra.Command{
		Use:         "bump [catalog]",
		Short:       "Add newer upstream versions to package manifests",
		Annotations: guide("Version discovery", "/docs/writing-packages/version-discovery"),
		Long: "Discover newer upstream versions for the selected packages, download each new " +
			"artifact to compute its checksums, and add the resulting entries to the package " +
			"manifests. With --backfill <n>, the newest n discovered versions are given " +
			"entries even when they are older than the current latest. --dry-run only " +
			"discovers and reports.",
		Example: "  nem catalog bump                         # every package\n" +
			"  nem catalog bump . --package kubectl     # one package\n" +
			"  nem catalog bump . --backfill 3          # ensure the newest three versions exist\n" +
			"  nem catalog bump . --dry-run             # report without writing",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: firstArgOnly(completeYAMLOrDir),
		RunE: func(cmd *cobra.Command, args []string) error {
			cat := "."
			if len(args) == 1 {
				cat = args[0]
			}
			if err := validateOutput(output); err != nil {
				return err
			}
			refs, err := parsePackageRefs(packages)
			if err != nil {
				return err
			}
			res, err := bump.Run(cmd.Context(), cat, bump.Options{Packages: refs, Backfill: backfill, DryRun: dryRun})
			if err != nil {
				return err
			}
			if output == outputJSON {
				if err := console.JSON(res.Rows); err != nil {
					return err
				}
			}
			console.Success("%s", bumpSummary(res, dryRun))
			for _, h := range bumpHints(res) {
				console.Hint(h)
			}
			if res.Failed() > 0 {
				return &ExitError{Code: 1}
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&packages, "package", nil,
		"bump name[@version] (repeatable; omitted version means every newer version; default: every package)")
	_ = cmd.RegisterFlagCompletionFunc("package", completeCatalogDirPackages)
	cmd.Flags().IntVar(&backfill, "backfill", 0, "also ensure the newest <n> discovered versions have entries")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report the versions that would be added without downloading or writing anything")
	addOutputFlag(cmd, &output)
	return cmd
}

func bumpSummary(res *bump.Result, dryRun bool) string {
	var parts []string
	if n := res.Bumped(); n > 0 {
		verb := "Bumped"
		if dryRun {
			verb = "Would bump"
		}
		parts = append(parts, verb+" "+report.Plural(n, "package"))
	}
	if n := res.Failed(); n > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", n))
	}
	if n := res.UpToDate(); n > 0 {
		parts = append(parts, fmt.Sprintf("%d up to date", n))
	}
	return countSummary("Nothing to bump", parts, res.Skipped)
}

func bumpHints(res *bump.Result) []string {
	var hints []string
	for _, row := range res.Rows {
		for _, v := range row.Missing {
			hints = append(hints, fmt.Sprintf("Retry `nem catalog bump --package %s@%s` once its upstream assets are uploaded", row.Name, v))
		}
		for _, v := range row.Unpublished {
			hints = append(hints, fmt.Sprintf("Run `nem catalog build --package %s@%s --push` to publish the backfilled archive", row.Name, v))
		}
	}
	return hints
}
