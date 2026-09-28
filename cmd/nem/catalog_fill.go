package main

import (
	"github.com/spf13/cobra"

	"github.com/vi-dev/nem/internal/fill"
	"github.com/vi-dev/nem/internal/report"
)

func newCatalogFillCmd() *cobra.Command {
	var packages []string
	var dryRun bool
	cmd := &cobra.Command{
		Use:               "fill <ref>",
		Short:             "Download a catalog's upstream artifacts and publish them as archives",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			summary, err := fill.Run(cmd.Context(), nemHome, fill.Options{CatalogRef: args[0], Packages: packages, DryRun: dryRun})
			if err != nil {
				return err
			}
			verb := "Filled"
			if dryRun {
				verb = "Would fill"
			}
			console.Success("%s %s, %s, %s, %d present, %s not fillable",
				verb, report.Plural(summary.Packages, "package"), report.Plural(summary.Filled, "fill"),
				report.Plural(summary.Healed, "heal"), summary.Present, report.Plural(summary.NotFillable, "package"))
			if summary.Failed > 0 {
				return &ExitError{Code: 1}
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&packages, "package", nil,
		"fill this package (repeatable; default: every package)")
	_ = cmd.RegisterFlagCompletionFunc("package", completeCatalogDirPackages)
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report the fill plan without downloading or publishing anything")
	return cmd
}
