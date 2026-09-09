package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/vi-dev/nem/internal/envx"
	"github.com/vi-dev/nem/internal/home"
	"github.com/vi-dev/nem/internal/install"
	"github.com/vi-dev/nem/internal/project"
	"github.com/vi-dev/nem/internal/spec"

	"github.com/vi-dev/nem/internal/testx"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	testx.WriteFile(t, path, content)
}

func TestExecCapturesProjectEnvVar(t *testing.T) {
	nemHomeDir := t.TempDir()
	projDir := t.TempDir()
	chdir(t, projDir)
	writeFile(t, filepath.Join(projDir, "nem.toml"), "[env]\nFOO = \"bar\"\n")

	out, errb, err := runNem(t, nemHomeDir, "exec", "--", "/bin/sh", "-c", "echo $FOO")
	if err != nil {
		t.Fatalf("exec: %v\n%s", err, errb)
	}
	if out != "bar\n" {
		t.Fatalf("stdout = %q, want %q", out, "bar\n")
	}
}

func TestExecExitCodePropagation(t *testing.T) {
	nemHomeDir := t.TempDir()
	projDir := t.TempDir()
	chdir(t, projDir)

	_, _, err := runNem(t, nemHomeDir, "exec", "--", "/bin/sh", "-c", "exit 7")
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("exec error = %v, want *ExitError", err)
	}
	if exitErr.Code != 7 {
		t.Fatalf("exit code = %d, want 7", exitErr.Code)
	}
}

func TestExecNotFoundCommandExits127(t *testing.T) {
	nemHomeDir := t.TempDir()
	projDir := t.TempDir()
	chdir(t, projDir)

	_, _, err := runNem(t, nemHomeDir, "exec", "--", "definitely-not-a-real-command-xyz")
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("exec error = %v, want *ExitError", err)
	}
	if exitErr.Code != 127 {
		t.Fatalf("exit code = %d, want 127", exitErr.Code)
	}
}

func TestExecLeadingPersistentFlagIsParsedNotPassedToChild(t *testing.T) {
	nemHomeDir := t.TempDir()
	projDir := t.TempDir()
	chdir(t, projDir)

	var out, errb bytes.Buffer
	if err := execNem(t, nemHomeDir, nil, &out, &errb, "--verbose", "exec", "--", "/bin/echo", "hi"); err != nil {
		t.Fatalf("exec: %v\n%s", err, errb.String())
	}
	if out.String() != "hi\n" {
		t.Fatalf("stdout = %q, want %q", out.String(), "hi\n")
	}
	if !flagVerbose {
		t.Fatal("--verbose before exec was not parsed as a root persistent flag")
	}
}

func TestExecNoCommandGivenIsUsageError(t *testing.T) {
	nemHomeDir := t.TempDir()
	projDir := t.TempDir()
	chdir(t, projDir)

	var out, errb bytes.Buffer
	if err := execNem(t, nemHomeDir, nil, &out, &errb, "exec", "--"); err == nil {
		t.Fatal("want error when no command follows --")
	}
	if ranHook {
		t.Fatal("hook ran for a missing exec command; usage errors must exit 2")
	}
}

const fakeToolYAML = `
schema: 2
name: mytool
artifact:
  oci: ":{{.Version}}"
install:
  - extract: {}
bins: ["bin"]
versions:
  - v1.0.0
`

func installFakeTool(t *testing.T, h home.Home, marker string) project.LockEntry {
	t.Helper()
	pkg, err := spec.Parse([]byte(fakeToolYAML))
	if err != nil {
		t.Fatalf("parse fake tool package: %v", err)
	}
	archive := makeTarGz(t, map[string]string{"bin/mytool": "#!/bin/sh\necho " + marker + "\n"})
	artifact := filepath.Join(t.TempDir(), "artifact.tar.gz")
	writeBinary(t, artifact, archive)

	if err := install.Install(context.Background(), h, pkg, "v1.0.0", "test", artifact); err != nil {
		t.Fatalf("install fake tool: %v", err)
	}
	return project.LockEntry{
		Name: "mytool", Version: "v1.0.0", Catalog: "test", Direct: true, OnPath: true,
		Platforms: []string{spec.Current().String()},
	}
}

func writeBinary(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExecResolvesPathInjectedToolByBareName(t *testing.T) {
	nemHomeDir := t.TempDir()
	h := testNemHome(nemHomeDir)
	projDir := t.TempDir()
	chdir(t, projDir)
	writeFile(t, filepath.Join(projDir, "nem.toml"), "")

	entry := installFakeTool(t, h, "hello-from-mytool")
	lf := &project.Lockfile{Path: filepath.Join(projDir, "nem.lock"), Packages: []project.LockEntry{entry}}
	if err := project.WriteLock(lf); err != nil {
		t.Fatalf("WriteLock: %v", err)
	}

	out, errb, err := runNem(t, nemHomeDir, "exec", "--", "mytool")
	if err != nil {
		t.Fatalf("exec: %v\n%s", err, errb)
	}
	if out != "hello-from-mytool\n" {
		t.Fatalf("stdout = %q, want %q", out, "hello-from-mytool\n")
	}
}

func TestLookPathNeverReturnsBareNameForEmptyPathComponent(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	toolPath := filepath.Join(dir, "footool")
	writeFile(t, toolPath, "#!/bin/sh\necho footool-ran\n")
	if err := os.Chmod(toolPath, 0o755); err != nil {
		t.Fatal(err)
	}

	resolved, err := lookPath("footool", "/nonexistent-dir-xyz:")
	if err != nil {
		t.Fatalf("lookPath: %v", err)
	}
	if !strings.ContainsRune(resolved, os.PathSeparator) {
		t.Fatalf("lookPath returned a bare name %q; exec.Command would "+
			"re-resolve it against the real process PATH instead of the "+
			"composed one", resolved)
	}
	info, statErr := os.Stat(resolved)
	if statErr != nil || info.IsDir() {
		t.Fatalf("resolved path %q does not point at the installed tool: %v", resolved, statErr)
	}
}

func TestExitCodeForSignalKill(t *testing.T) {
	cmd := exec.Command("/bin/sleep", "5")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal sleep: %v", err)
	}

	err := cmd.Wait()
	if err == nil {
		t.Fatal("want error from a signal-killed process")
	}
	want := 128 + int(syscall.SIGTERM)
	if got := exitCodeFor(err); got != want {
		t.Fatalf("exitCodeFor = %d, want %d", got, want)
	}
}

func TestBuildChildEnvPrependsLoaderVar(t *testing.T) {
	base := []string{"PATH=/usr/bin", "DYLD_LIBRARY_PATH=/sys/lib"}
	res := envx.Result{LoaderVar: "DYLD_LIBRARY_PATH", LoaderPath: []string{"/n/openssl/lib"}}

	env := buildChildEnv(base, res, "/nem/bin:/usr/bin")

	count := 0
	got := ""
	for _, kv := range env {
		if name, value, ok := strings.Cut(kv, "="); ok && name == "DYLD_LIBRARY_PATH" {
			count++
			got = value
		}
	}
	if count != 1 {
		t.Fatalf("DYLD_LIBRARY_PATH must appear exactly once (replace, not duplicate), got %d entries: %v", count, env)
	}
	if got != "/n/openssl/lib:/sys/lib" {
		t.Fatalf("DYLD_LIBRARY_PATH = %q, want /n/openssl/lib:/sys/lib", got)
	}
}

func TestBuildChildEnvLeavesLoaderVarWhenNoLibraries(t *testing.T) {
	base := []string{"PATH=/usr/bin", "DYLD_LIBRARY_PATH=/sys/lib"}
	res := envx.Result{LoaderVar: "DYLD_LIBRARY_PATH"}

	env := buildChildEnv(base, res, "/nem/bin:/usr/bin")

	if slices.Contains(env, "DYLD_LIBRARY_PATH=/sys/lib") {
		return
	}
	t.Fatalf("inherited DYLD_LIBRARY_PATH must be left untouched, got %v", env)
}
