package main

import (
	"bytes"
	"context"
	"fmt"
	"os"

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
			target := "."
			if len(args) == 1 {
				target = args[0]
			}
			info, err := os.Stat(target)
			if err != nil {
				return err
			}
			if !info.IsDir() {
				return fmtFile(cmd.Context(), target, packages)
			}

			d := catalog.NewDir(target)
			names := packages
			if len(names) == 0 {
				if names, err = d.PackageNames(cmd.Context()); err != nil {
					return err
				}
				if len(names) == 0 {
					return fmt.Errorf("no package manifests under %s", target)
				}
			}
			seen := make(map[string]bool, len(names))
			for _, name := range names {
				if seen[name] {
					continue
				}
				seen[name] = true
				data, err := d.ReadManifest(cmd.Context(), name)
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
				if err := d.UpdateManifest(cmd.Context(), name, formatted); err != nil {
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

func fmtFile(ctx context.Context, path string, packages []string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	declared := ""
	if pkg, err := spec.Parse(data); err == nil {
		declared = pkg.Name
	}
	for _, name := range packages {
		if name != declared {
			return &catalog.PackageNotFoundError{Name: name}
		}
	}
	formatted, err := spec.Format(data)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if bytes.Equal(data, formatted) {
		return nil
	}
	if err := catalog.NewFile(path).UpdateManifest(ctx, declared, formatted); err != nil {
		return err
	}
	display := declared
	if display == "" {
		display = path
	}
	console.Success("Formatted %s", display)
	return nil
}
