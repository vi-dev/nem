package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/vi-dev/nem/internal/envx"
	"github.com/vi-dev/nem/internal/project"
)

type ExitError struct{ Code int }

func (e *ExitError) Error() string { return fmt.Sprintf("exit status %d", e.Code) }

func newExecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "exec [-- <cmd> [args...]]",
		Aliases: []string{"x"},
		Short:   "Run a command in the composed environment",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExec(cmd, args)
		},
	}

	cmd.Flags().SetInterspersed(false)
	return cmd
}

func composedPath() (envx.Result, string, error) {
	projManifest, projLock, err := loadProjectLayer()
	if err != nil {
		return envx.Result{}, "", err
	}
	globalManifest, err := project.LoadManifest(nemHome.GlobalManifest())
	if err != nil {
		return envx.Result{}, "", err
	}
	globalLock, err := project.LoadLock(nemHome.GlobalLock())
	if err != nil {
		return envx.Result{}, "", err
	}

	result := envx.Compose(projManifest, globalManifest, projLock, globalLock, nemHome, installMetaLookup, os.LookupEnv)
	for _, w := range result.Warnings {
		console.Warn("%s", w)
	}

	pathValue := prependPath(result.Path, envValue(os.Environ(), "PATH"))
	return result, pathValue, nil
}

func runExec(cmd *cobra.Command, args []string) error {
	result, pathValue, err := composedPath()
	if err != nil {
		return err
	}

	base := os.Environ()
	childEnv := buildChildEnv(base, result, pathValue)

	resolved, lookErr := lookPath(args[0], pathValue)
	if lookErr != nil {
		return &ExitError{Code: exitCodeFor(lookErr)}
	}

	child := exec.CommandContext(cmd.Context(), resolved, args[1:]...)
	child.Args[0] = args[0]
	child.Env = childEnv
	child.Stdin = cmd.InOrStdin()
	child.Stdout = cmd.OutOrStdout()
	child.Stderr = cmd.ErrOrStderr()

	if err := child.Run(); err != nil {
		return &ExitError{Code: exitCodeFor(err)}
	}
	return nil
}

func buildChildEnv(base []string, res envx.Result, pathValue string) []string {
	overlay := make(map[string]string, len(res.Vars))
	for _, v := range res.Vars {
		overlay[v.Name] = v.Value
	}

	loaderValue := ""
	if res.LoaderVar != "" && len(res.LoaderPath) > 0 {
		loaderValue = prependPath(res.LoaderPath, envValue(base, res.LoaderVar))
	}

	env := make([]string, 0, len(base)+len(res.Vars)+2)
	applied := make(map[string]bool, len(overlay))
	for _, kv := range base {
		name, _, ok := strings.Cut(kv, "=")
		if !ok {
			env = append(env, kv)
			continue
		}
		if name == "PATH" {
			continue
		}
		if loaderValue != "" && name == res.LoaderVar {
			continue
		}
		if nv, ok := overlay[name]; ok {
			env = append(env, name+"="+nv)
			applied[name] = true
			continue
		}
		env = append(env, kv)
	}
	for _, v := range res.Vars {
		if applied[v.Name] {
			continue
		}
		env = append(env, v.Name+"="+v.Value)
	}

	env = append(env, "PATH="+pathValue)
	if loaderValue != "" {
		env = append(env, res.LoaderVar+"="+loaderValue)
	}
	return env
}

func envValue(env []string, key string) string {
	for _, kv := range env {
		if name, value, ok := strings.Cut(kv, "="); ok && name == key {
			return value
		}
	}
	return ""
}

func prependPath(dirs []string, existing string) string {
	if len(dirs) == 0 {
		return existing
	}
	prefix := strings.Join(dirs, string(os.PathListSeparator))
	if existing == "" {
		return prefix
	}
	return prefix + string(os.PathListSeparator) + existing
}

func lookPath(name, pathValue string) (string, error) {
	if strings.ContainsRune(name, os.PathSeparator) {
		return name, nil
	}
	for _, dir := range filepath.SplitList(pathValue) {
		if dir == "" {
			dir = "."
		}
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		if info.Mode().Perm()&0o111 != 0 {
			return anchorPath(candidate), nil
		}
	}
	return "", &exec.Error{Name: name, Err: exec.ErrNotFound}
}

func anchorPath(path string) string {
	if strings.ContainsRune(path, os.PathSeparator) {
		return path
	}
	return "." + string(os.PathSeparator) + path
}

func exitCodeFor(err error) int {
	if errors.Is(err, context.Canceled) {
		return 130
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal())
		}
		return exitErr.ExitCode()
	}
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) ||
		strings.Contains(err.Error(), "executable file not found") {
		return 127
	}
	return 126
}
