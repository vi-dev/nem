package main

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/vi-dev/clix"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/diff"
	"github.com/vi-dev/nem/internal/publish"
)

func newCatalogDiffCmd() *cobra.Command {
	var packages []string
	var output string
	cmd := &cobra.Command{
		Use:         "diff <base> <target>",
		Short:       "Compare a base catalog's package manifests against a target catalog",
		Annotations: guide("Authoring workflow", "/docs/writing-packages/workflow"),
		Long: "Lint <base> when it is a local catalog, then compare its package manifests against " +
			"<target> and print one " +
			"row per package with its status relative to target: new, updated, or removed, " +
			"plus a count of unchanged packages. Base is the catalog being worked on and " +
			"target the reference, typically the published one. --output json emits only " +
			"the changed rows together with the versions base adds.",
		Example: "  nem catalog diff . ghcr.io/vi-dev/nem-catalog:v4                 # a checkout against the published catalog\n" +
			"  nem catalog diff . ghcr.io/vi-dev/nem-catalog:v4 --output json   # for CI\n" +
			"  nem catalog diff . ghcr.io/vi-dev/nem-catalog:v4 --package jq    # one package",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: argsUpTo(2, completeYAMLOrDir),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := clix.ValidateOutput(output); err != nil {
				return err
			}
			if catalog.Local(args[0]) {
				findings, err := publish.Lint(cmd.Context(), args[0], packages...)
				if err != nil {
					return err
				}
				if len(findings) > 0 {
					for _, f := range findings {
						ui.Warn(f.String())
					}
					return &ExitError{Code: 1}
				}
			}
			res, err := diff.Compare(cmd.Context(), args[0], args[1], packages)
			if err != nil {
				return err
			}
			if output == "json" {
				if err := ui.JSON(res.Changes); err != nil {
					return err
				}
			} else {
				renderDiffTable(res.Changes)
			}
			ui.Successf("%d changed (%d new, %d updated, %d removed), %d unchanged",
				len(res.Changes), res.Count(diff.StatusNew), res.Count(diff.StatusUpdated),
				res.Count(diff.StatusRemoved), res.Unchanged)
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&packages, "package", nil,
		"compare this package (repeatable; default: every package)")
	_ = cmd.RegisterFlagCompletionFunc("package", completeCatalogDirPackages)
	clix.AddOutputFlag(cmd, &output)
	return cmd
}

func renderDiffTable(changes []diff.Change) {
	if len(changes) == 0 {
		return
	}
	table := make([][]string, 0, len(changes))
	for _, r := range changes {
		table = append(table, []string{r.Name, r.Status, orDash(r.Base), orDash(r.Target),
			orDash(strings.Join(r.Diff, ", ")), orDash(exclusionChanges(r))})
	}
	ui.Table([]string{"PACKAGE", "STATUS", "BASE", "TARGET", "DIFF", "EXCLUSIONS"}, table)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func exclusionChanges(c diff.Change) string {
	parts := make([]string, 0, len(c.Excluded)+len(c.Unexcluded))
	for _, v := range c.Excluded {
		parts = append(parts, v+" excluded")
	}
	for _, v := range c.Unexcluded {
		parts = append(parts, v+" lifted")
	}
	return strings.Join(parts, ", ")
}
