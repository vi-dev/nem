package main

import (
	"github.com/spf13/cobra"

	"github.com/vi-dev/nem/internal/mirror"
	"github.com/vi-dev/nem/internal/report"
)

func newCatalogMirrorCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:         "mirror <src> <dst>",
		Short:       "Replicate a catalog and its archives to another registry",
		Annotations: guide("Mirroring and filling", "/docs/managing-catalogs/mirror-and-fill"),
		Long: "Copy a catalog's index and every archive it references from <src> to <dst>, " +
			"byte for byte, so that <dst> can be consumed as a catalog on its own. Re-running " +
			"skips what is already present. Archives the source never published are not " +
			"created; nem catalog fill adds them.",
		Example: "  nem catalog mirror ghcr.io/vi-dev/nem-catalog:v2 registry.corp.example/nem/catalog:v2             # copy\n" +
			"  nem catalog mirror ghcr.io/vi-dev/nem-catalog:v2 registry.corp.example/nem/catalog:v2 --dry-run   # the plan only",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			summary, err := mirror.Run(cmd.Context(), mirror.Options{SrcRef: args[0], DstRef: args[1], DryRun: dryRun})
			if err != nil {
				return err
			}
			verb := "Mirrored"
			if dryRun {
				verb = "Would mirror"
			}
			if summary.Failed > 0 {
				console.Success("%s %s, %s, %s failed",
					verb, report.Plural(summary.Packages, "package"), report.Plural(summary.Copied, "tag"), report.Plural(summary.Failed, "tag"))
				return &ExitError{Code: 1}
			}
			console.Success("%s %s, %s", verb, report.Plural(summary.Packages, "package"), report.Plural(summary.Copied, "tag"))
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report the mirror plan without writing anything")
	return cmd
}
