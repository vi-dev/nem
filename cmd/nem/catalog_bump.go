package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vi-dev/nem/internal/bump"
	"github.com/vi-dev/nem/internal/spec"
)

func newCatalogBumpCmd() *cobra.Command {
	var packages []string
	var backfill int
	var dryRun, jsonOut bool
	cmd := &cobra.Command{
		Use:               "bump [catalog]",
		Short:             "Add newer upstream versions to package manifests",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: firstArgOnly(completeYAMLOrDir),
		RunE: func(cmd *cobra.Command, args []string) error {
			cat := "."
			if len(args) == 1 {
				cat = args[0]
			}
			refs := make([]spec.Ref, 0, len(packages))
			for _, p := range packages {
				ref, err := spec.ParseRef(p)
				if err != nil {
					return err
				}
				refs = append(refs, ref)
			}
			res, err := bump.Run(cmd.Context(), cat, bump.Options{Packages: refs, Backfill: backfill, DryRun: dryRun})
			if err != nil {
				return err
			}
			if jsonOut {
				if err := console.JSON(res.Rows); err != nil {
					return err
				}
			}
			console.Success("%s", bumpSummary(res, dryRun))
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
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit every attempted package as JSON")
	return cmd
}

func bumpSummary(res *bump.Result, dryRun bool) string {
	var parts []string
	if n := res.Bumped(); n > 0 {
		verb := "Bumped"
		if dryRun {
			verb = "Would bump"
		}
		parts = append(parts, fmt.Sprintf("%s %d packages", verb, n))
	}
	if n := res.Failed(); n > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", n))
	}
	if n := res.UpToDate(); n > 0 {
		parts = append(parts, fmt.Sprintf("%d up to date", n))
	}
	return countSummary("Nothing to bump", parts, res.Skipped)
}
