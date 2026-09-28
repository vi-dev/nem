package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

const (
	outputText = "text"
	outputJSON = "json"
)

func addOutputFlag(cmd *cobra.Command, output *string) {
	cmd.Flags().StringVar(output, "output", outputText, "output format: text or json")
	_ = cmd.RegisterFlagCompletionFunc("output",
		cobra.FixedCompletions([]string{outputText, outputJSON}, cobra.ShellCompDirectiveNoFileComp))
}

func validateOutput(output string) error {
	if output != outputText && output != outputJSON {
		return fmt.Errorf("unknown output format %q (want %s or %s)", output, outputText, outputJSON)
	}
	return nil
}
