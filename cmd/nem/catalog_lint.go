package main

import (
	"github.com/spf13/cobra"

	"github.com/vi-dev/nem/internal/publish"
)

func newCatalogLintCmd() *cobra.Command {
	var packages []string
	cmd := &cobra.Command{
		Use:               "lint [catalog]",
		Short:             "Validate package manifests in a catalog",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: firstArgOnly(completeYAMLOrDir),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "."
			if len(args) == 1 {
				target = args[0]
			}
			findings, err := publish.Lint(cmd.Context(), target, packages...)
			if err != nil {
				return err
			}
			if len(findings) > 0 {
				for _, f := range findings {
					console.Warn("%s", f.String())
				}
				return &ExitError{Code: 1}
			}
			console.Success("Catalog is clean: no findings")
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&packages, "package", nil,
		"lint this package (repeatable; default: every package)")
	_ = cmd.RegisterFlagCompletionFunc("package", completeCatalogDirPackages)
	return cmd
}
