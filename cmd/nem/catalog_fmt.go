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
		Use:         "fmt [catalog]",
		Short:       "Rewrite package manifests to canonical form",
		Annotations: guide("Authoring workflow", "/docs/writing-packages/workflow"),
		Long: "Rewrite package manifests in canonical form: the field order, quoting, and " +
			"layout nem catalog bump itself writes. Only a local directory or a single " +
			"pkg.yaml can be formatted.",
		Example: "  nem catalog fmt                        # every manifest in the current directory\n" +
			"  nem catalog fmt . --package kubectl    # one package",
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
				files, err := editor.ReadFiles(ctx, name)
				if err != nil {
					return err
				}
				formatted := spec.Files{Recipes: map[string][]byte{}}
				if formatted.Pkg, err = spec.FormatPackage(files.Pkg); err != nil {
					return fmt.Errorf("%s: %w", name, err)
				}
				changed := !bytes.Equal(files.Pkg, formatted.Pkg)
				if !changed {
					formatted.Pkg = nil
				}
				for file, data := range files.Recipes {
					out, err := spec.Format(data)
					if err != nil {
						return fmt.Errorf("%s: %s: %w", name, file, err)
					}
					if !bytes.Equal(data, out) {
						formatted.Recipes[file] = out
						changed = true
					}
				}
				if !changed {
					continue
				}
				if err := editor.WriteFiles(ctx, name, formatted); err != nil {
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
