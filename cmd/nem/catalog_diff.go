package main

import (
	"github.com/spf13/cobra"

	"github.com/vi-dev/nem/internal/diff"
)

func newCatalogDiffCmd() *cobra.Command {
	var opts diff.Options
	cmd := &cobra.Command{
		Use:   "diff <registry-ref> [dir|pkg.yaml]",
		Short: "Compare local package manifests against a published catalog",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "."
			if len(args) == 2 {
				target = args[1]
			}
			return diff.Run(cmd.Context(), args[0], target, opts, console)
		},
	}
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "emit every compared package as JSON")
	return cmd
}
