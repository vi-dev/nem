package main

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/vi-dev/nem/internal/diff"
	"github.com/vi-dev/nem/internal/publish"
)

func newCatalogDiffCmd() *cobra.Command {
	var packages []string
	var output string
	cmd := &cobra.Command{
		Use:               "diff <base> <target>",
		Short:             "Compare a base catalog's package manifests against a target catalog",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: argsUpTo(2, completeYAMLOrDir),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateOutput(output); err != nil {
				return err
			}
			findings, err := publish.Lint(cmd.Context(), args[0], packages...)
			if err != nil {
				return err
			}
			if len(findings) > 0 {
				for _, f := range findings {
					console.Warn("%s", f.String())
				}
				return &ExitError{Code: 1}
			}
			res, err := diff.Compare(cmd.Context(), args[0], args[1], packages)
			if err != nil {
				return err
			}
			if output == outputJSON {
				if err := console.JSON(res.Changes); err != nil {
					return err
				}
			} else {
				renderDiffTable(res.Changes)
			}
			console.Success("%d changed (%d new, %d updated, %d removed), %d unchanged",
				len(res.Changes), res.Count(diff.StatusNew), res.Count(diff.StatusUpdated),
				res.Count(diff.StatusRemoved), res.Unchanged)
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&packages, "package", nil,
		"compare this package (repeatable; default: every package)")
	_ = cmd.RegisterFlagCompletionFunc("package", completeCatalogDirPackages)
	addOutputFlag(cmd, &output)
	return cmd
}

func renderDiffTable(changes []diff.Change) {
	if len(changes) == 0 {
		return
	}
	table := make([][]string, 0, len(changes))
	for _, r := range changes {
		table = append(table, []string{r.Name, r.Status, orDash(r.Base), orDash(r.Target),
			orDash(strings.Join(r.Diff, ", "))})
	}
	console.Table([]string{"PACKAGE", "STATUS", "BASE", "TARGET", "DIFF"}, table)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
