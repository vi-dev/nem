package main

import (
	"github.com/spf13/cobra"

	"github.com/vi-dev/nem/internal/clidoc"
	"github.com/vi-dev/nem/internal/report"
)

func guide(title, path string) map[string]string {
	return map[string]string{clidoc.AnnotationGuideTitle: title, clidoc.AnnotationGuidePath: path}
}

func newGendocsCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "gendocs <dir>",
		Short:  "Write the command reference as Markdown pages",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := cmd.Root()
			root.InitDefaultCompletionCmd()
			root.InitDefaultVersionFlag()
			pages := clidoc.Pages(root)
			if err := clidoc.Write(args[0], pages); err != nil {
				return err
			}
			console.Success("Wrote %s to %s", report.Plural(len(pages), "page"), args[0])
			return nil
		},
	}
}
