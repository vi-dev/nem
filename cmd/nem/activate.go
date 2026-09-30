package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/vi-dev/nem/internal/shell"
)

var stdoutIsTTY = func() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func newActivateCmd() *cobra.Command {
	var printOnly bool
	cmd := &cobra.Command{
		Use:   "activate [zsh|bash]",
		Short: "Activate nem for the current shell",
		Long: "Install nem's hook block into the shell's startup file, .zshrc or .bashrc, between " +
			"# >>> nem >>> and # <<< nem <<< markers. The hook re-applies the composed environment " +
			"on every directory change and right after nem use, unuse, lock, and sync, and " +
			"sources nem's completions. Without a shell name, $SHELL decides. Restart the " +
			"shell afterwards.",
		Example: "  nem activate                 # the current $SHELL\n" +
			"  nem activate zsh             # a specific shell\n" +
			"  nem activate --print         # print the block instead of installing it",
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: []string{"bash", "zsh"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runActivate(args, printOnly)
		},
	}
	cmd.Flags().BoolVar(&printOnly, "print", false, "print the hook block to stdout instead of installing it into the rc file")
	return cmd
}

func newDeactivateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "deactivate [zsh|bash]",
		Short: "Deactivate nem for the current shell",
		Long: "Remove the hook block that nem activate installed from the shell's startup file. " +
			"The current shell keeps its environment until it restarts.",
		Example: "  nem deactivate               # the current $SHELL\n" +
			"  nem deactivate bash          # a specific shell",
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: []string{"bash", "zsh"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeactivate(args)
		},
	}
}

func shellArg(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return defaultShellName()
}

func activationDialect(name string) (shell.Dialect, error) {
	switch name {
	case "bash":
		return shell.Bash, nil
	case "zsh":
		return shell.Zsh, nil
	default:
		return 0, fmt.Errorf("unsupported shell %q (want bash or zsh)", name)
	}
}

func rcPathFor(d shell.Dialect) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch d {
	case shell.Bash:
		return filepath.Join(homeDir, ".bashrc"), nil
	case shell.Zsh:
		return filepath.Join(homeDir, ".zshrc"), nil
	default:
		return "", fmt.Errorf("no rc file for shell dialect %d", int(d))
	}
}

func runActivate(args []string, printOnly bool) error {
	name := shellArg(args)
	dialect, err := activationDialect(name)
	if err != nil {
		return err
	}

	if printOnly || !stdoutIsTTY() {
		console.Data("%s", shell.HookBlock(dialect))
		return nil
	}

	rcPath, err := rcPathFor(dialect)
	if err != nil {
		return err
	}
	if err := shell.InstallBlock(rcPath, dialect); err != nil {
		return err
	}
	console.Success("Activated nem for %s", name)
	console.Hint(fmt.Sprintf("Restart your shell or run: source %s", rcPath))
	return nil
}

func runDeactivate(args []string) error {
	name := shellArg(args)
	dialect, err := activationDialect(name)
	if err != nil {
		return err
	}

	rcPath, err := rcPathFor(dialect)
	if err != nil {
		return err
	}
	if err := shell.RemoveBlock(rcPath); err != nil {
		return err
	}
	console.Success("Deactivated nem for %s", name)
	return nil
}
