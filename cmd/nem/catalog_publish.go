package main

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/vi-dev/nem/internal/publish"
)

func newCatalogPublishCmd() *cobra.Command {
	var tags []string
	var dryRun, force bool
	cmd := &cobra.Command{
		Use:     "publish <ref> [catalog]",
		Aliases: []string{"pub"},
		Short:   "Publish a catalog to an OCI registry",
		Args:    cobra.RangeArgs(1, 2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 1 {
				return completeYAMLOrDir(cmd, args, toComplete)
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := args[0]
			cat := "."
			if len(args) == 2 {
				cat = args[1]
			}
			findings, err := publish.Lint(cmd.Context(), cat)
			if err != nil {
				return err
			}
			if len(findings) > 0 {
				for _, f := range findings {
					console.Warn("%s", f.String())
				}
				return &ExitError{Code: 1}
			}
			res, err := publish.Publish(cmd.Context(), cat, ref, publish.Options{Tags: tags, DryRun: dryRun, Force: force})
			if err != nil {
				return err
			}
			if dryRun {
				renderPublishPlan(res)
				console.Success("Dry run: would publish %s (%d packages), tags %s",
					ref, len(res.Packages), strings.Join(res.Tags, ", "))
				return nil
			}
			console.Success("Published %s: %d pushed, %d unchanged, tags %s",
				ref, res.Pushed, res.Unchanged, strings.Join(res.Tags, ", "))
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&tags, "tag", nil, "tag to move to the published index (repeatable; default v2)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report the publish plan without writing anything")
	cmd.Flags().BoolVar(&force, "force", false, "push every package manifest even when unchanged")
	return cmd
}

func renderPublishPlan(res *publish.Result) {
	rows := make([][]string, 0, len(res.Packages))
	for _, p := range res.Packages {
		rows = append(rows, []string{p.Name, orDash(p.Version)})
	}
	console.Table([]string{"PACKAGE", "VERSION"}, rows)
}
