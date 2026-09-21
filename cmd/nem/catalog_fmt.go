package main

import (
	"bytes"
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/spec"
)

func newCatalogFmtCmd() *cobra.Command {
	var packages []string
	cmd := &cobra.Command{
		Use:               "fmt [catalog]",
		Short:             "Rewrite package manifests to canonical form",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: firstArgOnly(completeYAMLOrDir),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cat := "."
			if len(args) == 1 {
				cat = args[0]
			}
			entry, err := catalog.Open(ctx, cat)
			if err != nil {
				return err
			}
			editor, ok := entry.Catalog.(catalog.Editor)
			if !ok {
				return fmt.Errorf("%s: fmt writes a local catalog directory or pkg.yaml", cat)
			}
			names, err := entry.Catalog.PackageNames(ctx)
			if err != nil {
				return err
			}
			if len(packages) > 0 {
				var selected []string
				for _, name := range packages {
					if slices.Contains(selected, name) {
						continue
					}
					if !slices.Contains(names, name) {
						return &catalog.PackageNotFoundError{Name: name, Catalogs: []string{cat}}
					}
					selected = append(selected, name)
				}
				names = selected
			} else if len(names) == 0 {
				return fmt.Errorf("no package manifests under %s", cat)
			}
			for _, name := range names {
				data, err := editor.ReadManifest(ctx, name)
				if err != nil {
					return err
				}
				formatted, err := spec.Format(data)
				if err != nil {
					return fmt.Errorf("%s: %w", name, err)
				}
				if bytes.Equal(data, formatted) {
					continue
				}
				if err := editor.UpdateManifest(ctx, name, formatted); err != nil {
					return err
				}
				console.Success("Formatted %s", name)
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&packages, "package", nil,
		"format this package (repeatable; default: every package)")
	_ = cmd.RegisterFlagCompletionFunc("package", completeCatalogDirPackages)
	return cmd
}
