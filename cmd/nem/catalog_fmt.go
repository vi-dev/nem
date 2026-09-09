package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/vi-dev/nem/internal/fsx"
	"github.com/vi-dev/nem/internal/publish"
	"github.com/vi-dev/nem/internal/spec"
)

func newCatalogFmtCmd() *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:   "fmt [dir|pkg.yaml]",
		Short: "Rewrite package manifests to canonical form",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "."
			if len(args) == 1 {
				target = args[0]
			}
			paths, err := publish.ManifestPaths(target)
			if err != nil {
				return err
			}
			dirty := 0
			for _, p := range paths {
				data, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				formatted, err := spec.Format(data)
				if err != nil {
					return fmt.Errorf("%s: %w", p, err)
				}
				if bytes.Equal(data, formatted) {
					continue
				}
				dirty++
				if check {
					console.Data("%s\n", p)
					continue
				}
				if err := fsx.WriteAtomic(p, formatted, 0o644); err != nil {
					return err
				}
				console.Success("Formatted %s", p)
			}
			if check && dirty > 0 {
				return &ExitError{Code: 1}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "list non-canonical manifests and exit 1 instead of rewriting")
	return cmd
}
