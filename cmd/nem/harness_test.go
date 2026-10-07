package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/vi-dev/nem/internal/home"
	"github.com/vi-dev/nem/internal/project"
	"github.com/vi-dev/nem/internal/testx"
)

func execNem(t *testing.T, nemHomeDir string, in io.Reader, out, errOut io.Writer, args ...string) error {
	t.Helper()
	t.Setenv("NEM_HOME", nemHomeDir)
	root := newRoot()
	if in != nil {
		root.SetIn(in)
	}
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetArgs(args)
	return root.Execute()
}

func runNem(t *testing.T, nemHomeDir string, args ...string) (string, string, error) {
	t.Helper()
	var out, errb bytes.Buffer
	err := execNem(t, nemHomeDir, nil, &out, &errb, append(args, "--color", "never")...)
	if err != nil && ranHook && console != nil {
		if _, exit := errors.AsType[*ExitError](err); !exit {
			console.Error(err, hintFor(err))
		}
	}
	return out.String(), errb.String(), err
}

func execNemMerged(t *testing.T, nemHomeDir, in string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := execNem(t, nemHomeDir, strings.NewReader(in), &out, &out, args...)
	return out.String(), err
}

func runNemComplete(t *testing.T, nemHomeDir string, words ...string) string {
	t.Helper()
	var out, errb bytes.Buffer
	if err := execNem(t, nemHomeDir, nil, &out, &errb, append([]string{"__complete"}, words...)...); err != nil {
		t.Fatalf("__complete %v: %v\nstderr: %s", words, err, errb.String())
	}
	return out.String()
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(cwd) })
}

func testNemHome(nemHomeDir string) home.Home {
	return testx.HomeAt(nemHomeDir)
}

func writeLockFile(t *testing.T, path string, pkgs ...project.LockEntry) {
	t.Helper()
	if err := project.WriteLock(&project.Lockfile{Path: path, Packages: pkgs}); err != nil {
		t.Fatalf("WriteLock: %v", err)
	}
}
