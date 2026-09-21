package main

import (
	"github.com/spf13/cobra"

	"github.com/vi-dev/nem/internal/mirror"
)

func newCatalogMirrorCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:               "mirror <src> <dst>",
		Short:             "Replicate a catalog and its archives to another registry",
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
				console.Success("%s %d packages, %d tag(s), %d tag(s) failed",
					verb, summary.Packages, summary.Copied, summary.Failed)
				return &ExitError{Code: 1}
			}
			console.Success("%s %d packages, %d tag(s)", verb, summary.Packages, summary.Copied)
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report the mirror plan without writing anything")
	return cmd
}
