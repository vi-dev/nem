package main

import (
	"github.com/spf13/cobra"
)

func newWhichCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "which <tool>...",
		Short: "Show where a tool resolves in the composed environment",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWhich(args)
		},
	}
	return cmd
}

func runWhich(args []string) error {
	_, pathValue, err := composedPath()
	if err != nil {
		return err
	}

	anyUnresolved := false
	for _, name := range args {
		resolved, lookErr := lookPath(name, pathValue)
		if lookErr != nil {
			console.Warn("%s: not found", name)
			anyUnresolved = true
			continue
		}
		console.Data("%s\n", resolved)
	}
	if anyUnresolved {
		return &ExitError{Code: 1}
	}
	return nil
}
