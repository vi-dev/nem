package main

import (
	"github.com/spf13/cobra"
	"github.com/vi-dev/clix/console"

	"github.com/vi-dev/nem/internal/fill"
)

func newCatalogFillCmd() *cobra.Command {
	var packages []string
	var dryRun bool
	cmd := &cobra.Command{
		Use:         "fill <ref>",
		Short:       "Download a catalog's upstream artifacts and publish them as archives",
		Annotations: guide("Mirroring and filling", "/docs/managing-catalogs/mirror-and-fill"),
		Long: "For each package version the catalog's manifests pin by checksum, download the " +
			"upstream artifact, verify it, and publish it as an archive beside the index at " +
			"<ref>, so that consumers no longer reach upstream. It needs push access to <ref>; " +
			"run it against your own mirror. Re-running heals missing or stale archives and " +
			"skips the rest.",
		Example: "  nem catalog fill registry.corp.example/nem/catalog:v4                     # every package\n" +
			"  nem catalog fill registry.corp.example/nem/catalog:v4 --package kubectl   # one package\n" +
			"  nem catalog fill registry.corp.example/nem/catalog:v4 --dry-run           # the plan only",
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
			ui.Successf("%s %s, %s, %s, %d present, %s not fillable",
				verb, console.Plural(summary.Packages, "package"), console.Plural(summary.Filled, "fill"),
				console.Plural(summary.Healed, "heal"), summary.Present, console.Plural(summary.NotFillable, "package"))
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
