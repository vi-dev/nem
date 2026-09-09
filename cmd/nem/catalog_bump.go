package main

import (
	"github.com/spf13/cobra"

	"github.com/vi-dev/nem/internal/bump"
)

func newCatalogBumpCmd() *cobra.Command {
	var opts bump.Options
	cmd := &cobra.Command{
		Use:   "bump [dir|pkg.yaml]",
		Short: "Add newer upstream versions to package manifests",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "."
			if len(args) == 1 {
				target = args[0]
			}
			return bump.Run(cmd.Context(), target, opts, console)
		},
	}
	cmd.Flags().StringVar(&opts.Version, "version", "", "version to add (default: every version newer than the current latest)")
	cmd.Flags().IntVar(&opts.Backfill, "backfill", 0, "also ensure the newest <n> discovered versions have entries")
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "emit every attempted package as JSON")
	return cmd
}
