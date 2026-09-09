package pkgtest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vi-dev/nem/internal/build"
	"github.com/vi-dev/nem/internal/clean"
	"github.com/vi-dev/nem/internal/envx"
	"github.com/vi-dev/nem/internal/home"
	"github.com/vi-dev/nem/internal/report"
	"github.com/vi-dev/nem/internal/spec"
	"github.com/vi-dev/nem/internal/testx"
	"github.com/vi-dev/nem/internal/usage"
)

func tarFixture(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "artifact.tar.gz")
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte("#!/bin/sh\necho hi\n")
	if err := tw.WriteHeader(&tar.Header{Name: "bin/tool", Mode: 0o755, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func installable(t *testing.T, steps ...spec.TestStep) (home.Home, *spec.Package, string) {
	t.Helper()
	root := t.TempDir()
	h := testx.HomeAt(root)
	pkg := &spec.Package{
		Name:    "tool",
		Bins:    []string{"bin"},
		Install: []spec.Action{{Extract: &spec.ExtractAction{}}},
		Test:    steps,
	}
	return h, pkg, tarFixture(t, t.TempDir())
}

func installableWithLibs(t *testing.T, steps ...spec.TestStep) (home.Home, *spec.Package, string) {
	t.Helper()
	root := t.TempDir()
	h := testx.HomeAt(root)
	pkg := &spec.Package{
		Name:    "tool",
		Bins:    []string{"bin"},
		Libs:    []string{"lib"},
		Install: []spec.Action{{Extract: &spec.ExtractAction{}}},
		Test:    steps,
	}
	return h, pkg, tarFixture(t, t.TempDir())
}

func runInstall(t *testing.T, h home.Home, pkg *spec.Package, artifact string) error {
	t.Helper()
	_, err := runInstallOut(t, h, pkg, artifact)
	return err
}

func runInstallOut(t *testing.T, h home.Home, pkg *spec.Package, artifact string) (string, error) {
	t.Helper()
	var b bytes.Buffer
	err := InstallAndRun(context.Background(), h, nil, pkg, "v1", "", artifact,
		report.New(&b, &b, report.Options{}), &b, &b)
	return b.String(), err
}

func runInstallWithDeps(t *testing.T, h home.Home, deps []build.ResolvedDep, pkg *spec.Package, artifact string) (string, error) {
	t.Helper()
	var b bytes.Buffer
	err := InstallAndRun(context.Background(), h, deps, pkg, "v1", "", artifact,
		report.New(&b, &b, report.Options{}), &b, &b)
	return b.String(), err
}

func aliasDirs(t *testing.T, h home.Home) []string {
	t.Helper()
	entries, err := os.ReadDir(h.Packages())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if strings.Contains(e.Name(), home.TestInstallInfix) {
			out = append(out, e.Name())
		}
	}
	return out
}

func TestInstallAndRunNoStepsSucceeds(t *testing.T) {
	h, pkg, artifact := installable(t)
	if err := runInstall(t, h, pkg, artifact); err != nil {
		t.Fatalf("InstallAndRun with no steps: %v", err)
	}
}

func otherPlatform() spec.Platform {
	other := spec.Platform{OS: "linux", Arch: "amd64"}
	if spec.Current() == other {
		other = spec.Platform{OS: "darwin", Arch: "arm64"}
	}
	return other
}

func TestInstallAndRunSkipsAPackageUnsupportedHere(t *testing.T) {
	h, pkg, artifact := installable(t, spec.TestStep{Run: "exit 1"})
	pkg.Platforms = []spec.Platform{otherPlatform()}
	if err := runInstall(t, h, pkg, artifact); err != nil {
		t.Fatalf("a package unsupported here must not be tested: %v", err)
	}
	if got := aliasDirs(t, h); len(got) != 0 {
		t.Fatalf("nothing should have been installed, found %v", got)
	}
}

func TestInstallAndRunTestsARealInstall(t *testing.T) {
	h, pkg, artifact := installable(t, spec.TestStep{Run: `
set -e
[ -x "$NEM_PREFIX/bin/tool" ]
[ -f "$NEM_PREFIX/.nem-meta.yaml" ]
case "$NEM_PREFIX" in *-NEMTEST-*) ;; *) exit 1 ;; esac
`})
	if err := runInstall(t, h, pkg, artifact); err != nil {
		t.Fatalf("InstallAndRun: %v", err)
	}
	if got := aliasDirs(t, h); len(got) != 0 {
		t.Fatalf("alias must be removed after success, found %v", got)
	}
}

func TestInstallAndRunRemovesTheAliasAfterFailure(t *testing.T) {
	h, pkg, artifact := installable(t, spec.TestStep{Run: "exit 1"})
	if err := runInstall(t, h, pkg, artifact); err == nil {
		t.Fatal("want an error from the failing step")
	}
	if got := aliasDirs(t, h); len(got) != 0 {
		t.Fatalf("alias must be removed after failure, found %v", got)
	}
}

func TestInstallAndRunReportsARemovalFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses the permission that induces the removal failure")
	}
	h, pkg, artifact := installable(t, spec.TestStep{Run: `chmod 0500 "$(dirname "$NEM_PREFIX")"`})
	err := runInstall(t, h, pkg, artifact)
	dirs := aliasDirs(t, h)
	if len(dirs) != 1 {
		t.Fatalf("want the un-removable alias left behind, found %v", dirs)
	}
	defer os.Chmod(filepath.Join(h.Packages(), dirs[0]), 0o755)
	if err == nil {
		t.Fatal("want an error reporting the failed removal")
	}
	if !strings.Contains(err.Error(), dirs[0]) {
		t.Fatalf("error must name the leaked alias dir %q, got: %v", dirs[0], err)
	}
}

func TestInstallAndRunJoinsAStepFailureAndARemovalFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses the permission that induces the removal failure")
	}
	h, pkg, artifact := installable(t, spec.TestStep{Run: `chmod 0500 "$(dirname "$NEM_PREFIX")"; exit 3`})
	err := runInstall(t, h, pkg, artifact)
	dirs := aliasDirs(t, h)
	if len(dirs) != 1 {
		t.Fatalf("want the un-removable alias left behind, found %v", dirs)
	}
	defer os.Chmod(filepath.Join(h.Packages(), dirs[0]), 0o755)
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "test step 1") {
		t.Fatalf("error must still report the step failure, got: %v", err)
	}
	if !strings.Contains(err.Error(), dirs[0]) {
		t.Fatalf("error must also report the removal failure naming %q, got: %v", dirs[0], err)
	}
}

func TestInstallAndRunLeavesNoUsageRow(t *testing.T) {
	h, pkg, artifact := installable(t, spec.TestStep{Run: "true"})
	if err := runInstall(t, h, pkg, artifact); err != nil {
		t.Fatalf("InstallAndRun: %v", err)
	}
	idx := usage.Load(h)
	for k := range idx {
		if strings.Contains(k, home.TestInstallInfix) {
			t.Fatalf("usage index still has an alias row %q: %v", k, idx)
		}
	}
}

func TestInstallAndRunLeavesTheRealInstallAlone(t *testing.T) {
	h, pkg, artifact := installable(t, spec.TestStep{Run: "true"})
	packageDir, err := h.PackageDir("tool", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageDir, "marker"), []byte("installed"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runInstall(t, h, pkg, artifact); err != nil {
		t.Fatalf("InstallAndRun: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(packageDir, "marker"))
	if err != nil {
		t.Fatalf("the real install must be untouched: %v", err)
	}
	if string(b) != "installed" {
		t.Fatalf("real install marker = %q, want %q", string(b), "installed")
	}
}

func TestInstallAndRunReportsTheOutcomeOnce(t *testing.T) {
	h, pkg, artifact := installable(t, spec.TestStep{Run: "true"})
	out, err := runInstallOut(t, h, pkg, artifact)
	if err != nil {
		t.Fatalf("InstallAndRun: %v", err)
	}
	if !strings.Contains(out, "Tested tool v1 (1 step)") {
		t.Fatalf("want a singular one-step report, got:\n%s", out)
	}
	if n := strings.Count(out, "Tested"); n != 1 {
		t.Fatalf("want exactly one completion line, got %d:\n%s", n, out)
	}

	h, pkg, artifact = installable(t, spec.TestStep{Run: "true"}, spec.TestStep{Run: "true"})
	out, err = runInstallOut(t, h, pkg, artifact)
	if err != nil {
		t.Fatalf("InstallAndRun: %v", err)
	}
	if !strings.Contains(out, "Tested tool v1 (2 steps)") {
		t.Fatalf("want a plural two-step report, got:\n%s", out)
	}
}

func TestInstallAndRunDoesNotReportAPassWhenNothingApplied(t *testing.T) {
	h, pkg, artifact := installable(t, spec.TestStep{Run: "exit 1", Platforms: []spec.Platform{otherPlatform()}})
	out, err := runInstallOut(t, h, pkg, artifact)
	if err != nil {
		t.Fatalf("InstallAndRun: %v", err)
	}
	if strings.Contains(out, "Tested") {
		t.Fatalf("a run that asserted nothing must not report a pass, got:\n%s", out)
	}
	if !strings.Contains(out, "nothing asserted") {
		t.Fatalf("want a notice that no test applied, got:\n%s", out)
	}
}

func TestInstallAndRunFailingStepReportsIndexAndCommand(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "second-ran")
	h, pkg, artifact := installable(t,
		spec.TestStep{Run: "true"},
		spec.TestStep{Run: "exit 3"},
		spec.TestStep{Run: "touch " + marker},
	)
	err := runInstall(t, h, pkg, artifact)
	if err == nil {
		t.Fatal("want an error from the failing step")
	}
	if !strings.Contains(err.Error(), "test step 2") || !strings.Contains(err.Error(), "exit 3") {
		t.Fatalf("error must name step 2 and its command, got %v", err)
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatal("steps after a failure must not run")
	}
}

func TestInstallAndRunUsesAScratchDirAndRemovesIt(t *testing.T) {
	record := filepath.Join(t.TempDir(), "cwd")
	t.Setenv("PKGTEST_RECORD", record)
	h, pkg, artifact := installable(t, spec.TestStep{Run: `pwd > "$PKGTEST_RECORD"`})
	if err := runInstall(t, h, pkg, artifact); err != nil {
		t.Fatalf("InstallAndRun: %v", err)
	}
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("step did not record its cwd: %v", err)
	}
	cwd := strings.TrimSpace(string(data))
	if !strings.HasPrefix(cwd, h.Tmp()) {
		t.Fatalf("step ran in %q, want a dir under %q", cwd, h.Tmp())
	}
	if _, err := os.Stat(cwd); !os.IsNotExist(err) {
		t.Fatalf("scratch dir %q must be removed, stat err = %v", cwd, err)
	}
}

func TestInstallAndRunScratchDirIsSweptByClean(t *testing.T) {
	record := filepath.Join(t.TempDir(), "cwd")
	t.Setenv("PKGTEST_RECORD", record)
	h, pkg, artifact := installable(t, spec.TestStep{Run: `pwd > "$PKGTEST_RECORD"`})
	if err := runInstall(t, h, pkg, artifact); err != nil {
		t.Fatalf("InstallAndRun: %v", err)
	}
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatalf("step did not record its cwd: %v", err)
	}

	leaked := strings.TrimSpace(string(data))
	if err := os.MkdirAll(leaked, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := clean.Scan(h, false)
	if err != nil {
		t.Fatalf("clean.Scan: %v", err)
	}
	for _, item := range store.Staging {
		if item.Path == leaked {
			return
		}
	}
	t.Fatalf("nem clean does not sweep the scratch dir %q; it found %+v", leaked, store.Staging)
}

func TestInstallAndRunDropsInheritedLoaderPath(t *testing.T) {
	t.Setenv("LD_LIBRARY_PATH", "sentinel")
	t.Setenv("DYLD_LIBRARY_PATH", "sentinel")

	dyldCheck := `[ -z "$DYLD_LIBRARY_PATH" ]`
	if runtime.GOOS == "darwin" {
		dyldCheck = "true"
	}
	h, pkg, artifact := installable(t, spec.TestStep{Run: `
set -e
[ -n "$NEM_PREFIX" ]
[ "$NEM_VERSION" = v1 ]
[ -z "$LD_LIBRARY_PATH" ]
` + dyldCheck + `
case "$PATH" in "$NEM_PREFIX/bin":*) ;; *) exit 1 ;; esac
`})
	if err := runInstall(t, h, pkg, artifact); err != nil {
		t.Fatalf("environment assertions failed: %v", err)
	}
}

func TestInstallAndRunAppliesPackageEnvAgainstTheAlias(t *testing.T) {
	h, pkg, artifact := installable(t, spec.TestStep{Run: `
set -e
[ "$TOOL_HOME" = "$NEM_PREFIX" ]
case "$TOOL_HOME" in *` + home.TestInstallInfix + `*) ;; *) exit 1 ;; esac
`})
	pkg.Env = []spec.EnvExport{{Name: "TOOL_HOME", Value: "{{.InstallDir}}"}}
	if err := runInstall(t, h, pkg, artifact); err != nil {
		t.Fatalf("environment assertions failed: %v", err)
	}
}

func TestInstallAndRunSkipsAReservedPackageEnvExport(t *testing.T) {
	h, pkg, artifact := installable(t, spec.TestStep{Run: `
set -e
case "$PATH" in "$NEM_PREFIX/bin":*) ;; *) exit 1 ;; esac
`})
	pkg.Env = []spec.EnvExport{{Name: "PATH", Value: "clobbered"}}
	out, err := runInstallOut(t, h, pkg, artifact)
	if err != nil {
		t.Fatalf("a reserved package env export must be skipped, not applied: %v", err)
	}
	if !strings.Contains(out, `reserved env var "PATH"`) {
		t.Fatalf("want a reported warning naming the skipped export, got:\n%s", out)
	}
}

func TestInstallAndRunScrubsInheritedStagingAndOutputDir(t *testing.T) {
	t.Setenv("NEM_STAGING_DIR", "/leftover/staging")
	t.Setenv("NEM_OUTPUT", "/leftover/output")
	h, pkg, artifact := installable(t, spec.TestStep{Run: `
set -e
[ -z "$NEM_STAGING_DIR" ]
[ -z "$NEM_OUTPUT" ]
`})
	if err := runInstall(t, h, pkg, artifact); err != nil {
		t.Fatalf("a caller's NEM_STAGING_DIR/NEM_OUTPUT must not reach a test step: %v", err)
	}
}

func TestInstallAndRunLoaderPathExcludesInheritedValue(t *testing.T) {
	t.Setenv("LD_LIBRARY_PATH", "sentinel")
	t.Setenv("DYLD_LIBRARY_PATH", "sentinel")
	loaderVar := envx.LoaderPathVar()
	h, pkg, artifact := installableWithLibs(t, spec.TestStep{Run: `
set -e
[ "$` + loaderVar + `" = "$NEM_PREFIX/lib" ]
`})
	if err := runInstall(t, h, pkg, artifact); err != nil {
		t.Fatalf("environment assertions failed: %v", err)
	}
}

func TestInstallAndRunSetsLoaderPathForOnLoaderPathDep(t *testing.T) {
	loaderVar := envx.LoaderPathVar()
	depPrefix := t.TempDir()
	dep := build.ResolvedDep{
		Name: "brotli", Version: "v1", Prefix: depPrefix,
		OnLoaderPath: true, Libs: []string{"lib"},
	}
	h, pkg, artifact := installableWithLibs(t, spec.TestStep{Run: `
set -e
[ "$` + loaderVar + `" = "$NEM_PREFIX/lib:` + filepath.Join(depPrefix, "lib") + `" ]
`})
	out, err := runInstallWithDeps(t, h, []build.ResolvedDep{dep}, pkg, artifact)
	if err != nil {
		t.Fatalf("environment assertions failed: %v\n%s", err, out)
	}
}

func TestInstallAndRunLoaderPathExcludesDepsNotOnLoaderPath(t *testing.T) {
	loaderVar := envx.LoaderPathVar()
	dep := build.ResolvedDep{
		Name: "linkonly", Version: "v1", Prefix: t.TempDir(),
		Libs: []string{"lib"},
	}
	h, pkg, artifact := installableWithLibs(t, spec.TestStep{Run: `
set -e
[ "$` + loaderVar + `" = "$NEM_PREFIX/lib" ]
`})
	out, err := runInstallWithDeps(t, h, []build.ResolvedDep{dep}, pkg, artifact)
	if err != nil {
		t.Fatalf("environment assertions failed: %v\n%s", err, out)
	}
}

func TestInstallAndRunSkipsLoaderPathWhenNothingHasLibs(t *testing.T) {
	loaderVar := envx.LoaderPathVar()
	dep := build.ResolvedDep{Name: "dep", Version: "v1", Prefix: t.TempDir(), OnLoaderPath: true}
	h, pkg, artifact := installable(t, spec.TestStep{Run: `
set -e
[ -z "${` + loaderVar + `+x}" ]
`})
	out, err := runInstallWithDeps(t, h, []build.ResolvedDep{dep}, pkg, artifact)
	if err != nil {
		t.Fatalf("environment assertions failed: %v\n%s", err, out)
	}
}

func TestInstallAndRunFailingStepQuotesTheAuthorsScript(t *testing.T) {
	h, pkg, artifact := installableWithLibs(t, spec.TestStep{Run: "exit 3"})
	err := runInstall(t, h, pkg, artifact)
	if err == nil {
		t.Fatal("want an error from the failing step")
	}
	if !strings.Contains(err.Error(), `"exit 3"`) {
		t.Fatalf("error must quote the author's own script, got: %v", err)
	}
	if strings.Contains(err.Error(), "export") {
		t.Fatalf("error must not quote the loader-path prologue, got: %v", err)
	}
}
