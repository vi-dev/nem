package install

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/vi-dev/nem/internal/fetch"
	"github.com/vi-dev/nem/internal/report"
	"github.com/vi-dev/nem/internal/spec"
	"github.com/vi-dev/nem/internal/testx"
)

func copyToolPkg(name string) *spec.Package {
	return &spec.Package{
		Name:     name,
		Artifact: spec.Artifact{OCI: ":{{.Version}}"},
		Install: []spec.Action{{
			Copy: &spec.CopyAction{Src: artifactToken, Dst: "bin/tool", Mode: 0o755},
		}},
		Bins: []string{"bin"},
	}
}

func withAcquire(t *testing.T, fn func(ctx context.Context, pkg *spec.Package, version string, plat spec.Platform, src fetch.Source, dir string, task report.Task) (string, error)) {
	t.Helper()
	testx.Swap(t, &acquire, fn)
}

func TestRunAllJobsInstallReportDiscard(t *testing.T) {
	h := testx.Home(t)
	const n = 4

	var mu sync.Mutex
	artifacts := map[string]string{}

	withAcquire(t, func(_ context.Context, pkg *spec.Package, version string, _ spec.Platform, _ fetch.Source, dir string, _ report.Task) (string, error) {
		f, err := os.CreateTemp(dir, pkg.Name+"-*.artifact")
		if err != nil {
			return "", err
		}
		content := "binary-" + pkg.Name
		if _, err := f.WriteString(content); err != nil {
			f.Close()
			return "", err
		}
		if err := f.Close(); err != nil {
			return "", err
		}
		mu.Lock()
		artifacts[pkg.Name] = f.Name()
		mu.Unlock()
		return f.Name(), nil
	})

	jobs := make([]Job, n)
	for i := range jobs {
		jobs[i] = Job{Pkg: copyToolPkg(fmt.Sprintf("tool%d", i)), Version: "v1.0.0", Catalog: "official"}
	}

	ctx, _ := testx.ReporterContext(context.Background())
	if err := Run(ctx, h, jobs); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if _, err := os.Stat(h.Tmp()); err != nil {
		t.Fatalf("h.Tmp() not created: %v", err)
	}

	for i := range jobs {
		name := fmt.Sprintf("tool%d", i)
		if !IsInstalled(h, name, "v1.0.0") {
			t.Fatalf("%s: not installed", name)
		}
		installDir, err := h.PackageDir(name, "v1.0.0")
		if err != nil {
			t.Fatalf("PackageDir: %v", err)
		}
		got, err := os.ReadFile(filepath.Join(installDir, "bin", "tool"))
		if err != nil || string(got) != "binary-"+name {
			t.Fatalf("%s: bin/tool = %q, %v, want %q", name, got, err, "binary-"+name)
		}

		mu.Lock()
		artifactPath := artifacts[name]
		mu.Unlock()
		if artifactPath == "" {
			t.Fatalf("%s: acquire was never called", name)
		}
		if _, err := os.Lstat(artifactPath); !os.IsNotExist(err) {
			t.Fatalf("%s: artifact %s not removed after install, stat err = %v", name, artifactPath, err)
		}
	}
}

func TestRunSkipsAlreadyInstalledAcquireNotCalled(t *testing.T) {
	h := testx.Home(t)

	installedPkg := copyToolPkg("already")
	tmp := t.TempDir()
	preArtifact := filepath.Join(tmp, "pre-artifact")
	if err := os.WriteFile(preArtifact, []byte("pre-existing"), 0o644); err != nil {
		t.Fatalf("write pre-artifact: %v", err)
	}
	if err := Install(context.Background(), h, installedPkg, "v1.0.0", "official", preArtifact, false, nil); err != nil {
		t.Fatalf("pre-install: %v", err)
	}

	freshPkg := copyToolPkg("fresh")

	var mu sync.Mutex
	var called []string
	withAcquire(t, func(_ context.Context, pkg *spec.Package, version string, _ spec.Platform, _ fetch.Source, dir string, _ report.Task) (string, error) {
		mu.Lock()
		called = append(called, pkg.Name)
		mu.Unlock()
		f, err := os.CreateTemp(dir, pkg.Name+"-*.artifact")
		if err != nil {
			return "", err
		}
		if _, err := f.WriteString("fresh-bytes"); err != nil {
			f.Close()
			return "", err
		}
		f.Close()
		return f.Name(), nil
	})

	ctx, rep := testx.ReporterContext(context.Background())
	jobs := []Job{
		{Pkg: installedPkg, Version: "v1.0.0", Catalog: "official"},
		{Pkg: freshPkg, Version: "v1.0.0", Catalog: "official"},
	}

	if err := Run(ctx, h, jobs); err != nil {
		t.Fatalf("Run: %v", err)
	}

	mu.Lock()
	gotCalled := append([]string(nil), called...)
	mu.Unlock()
	if len(gotCalled) != 1 || gotCalled[0] != "fresh" {
		t.Fatalf("acquire called for %v, want exactly [fresh]", gotCalled)
	}

	if nTasks := len(rep.Tasks()); nTasks != 1 {
		t.Fatalf("task count = %d, want 1 (already-installed job must produce no task line)", nTasks)
	}

	task := rep.TaskFor("Installing fresh v1.0.0")
	if task == nil {
		t.Fatal("no task for fresh v1.0.0")
	}
	done, failed, outcome := task.Snapshot()
	if !done || failed || outcome != "Installed fresh v1.0.0" {
		t.Fatalf("fresh task = done:%v failed:%v outcome:%q, want done outcome %q", done, failed, outcome, "Installed fresh v1.0.0")
	}

	if !IsInstalled(h, "already", "v1.0.0") || !IsInstalled(h, "fresh", "v1.0.0") {
		t.Fatal("both jobs should end up installed")
	}
}

func TestRunFailFastCancelsRestJobs(t *testing.T) {
	h := testx.Home(t)
	ctx, rep := testx.ReporterContext(context.Background())

	const n = 4
	jobs := make([]Job, n)
	for i := range jobs {
		jobs[i] = Job{Pkg: copyToolPkg(fmt.Sprintf("tool%d", i)), Version: "v1.0.0", Catalog: "official"}
	}

	bootErr := errors.New("boom")
	var winnerTaken atomic.Bool
	withAcquire(t, func(ctx context.Context, pkg *spec.Package, version string, _ spec.Platform, _ fetch.Source, dir string, _ report.Task) (string, error) {
		if winnerTaken.CompareAndSwap(false, true) {
			return "", bootErr
		}

		<-ctx.Done()
		return "", ctx.Err()
	})

	err := Run(ctx, h, jobs)
	if !errors.Is(err, bootErr) {
		t.Fatalf("Run error = %v, want wrapping %v", err, bootErr)
	}

	sawBoom := 0
	for i := range jobs {
		name := fmt.Sprintf("tool%d", i)
		label := "Installing " + name + " v1.0.0"
		task := rep.TaskFor(label)
		if task == nil {
			t.Fatalf("%s: no task recorded", name)
		}
		done, failed, outcome := task.Snapshot()
		if done {
			t.Fatalf("%s: task marked Done, want no successful install during fail-fast", name)
		}
		if !failed {
			t.Fatalf("%s: task not failed", name)
		}
		switch outcome {
		case "Failed to install " + name + " v1.0.0":
			sawBoom++
		case "Cancelled " + name + " v1.0.0":

		default:
			t.Fatalf("%s: unexpected outcome %q", name, outcome)
		}
		if IsInstalled(h, name, "v1.0.0") {
			t.Fatalf("%s: must not be installed after fail-fast", name)
		}
	}
	if sawBoom != 1 {
		t.Fatalf("saw %d boom outcomes, want exactly 1", sawBoom)
	}
}

func TestIsCancellationTrustsOnlyTheJobsOwnError(t *testing.T) {
	doneCtx, cancel := context.WithCancel(context.Background())
	cancel()

	realErr := errors.New("checksum mismatch")
	if isCancellation(doneCtx, realErr) {
		t.Fatal("a real unrelated error must not be classified as cancellation just because gctx is already done")
	}
	if !isCancellation(doneCtx, context.Canceled) {
		t.Fatal("context.Canceled itself must be classified as cancellation")
	}
	if !isCancellation(doneCtx, nil) {
		t.Fatal("with no error to inspect, an already-done gctx must be classified as cancellation")
	}

	freshCtx := context.Background()
	if isCancellation(freshCtx, nil) {
		t.Fatal("a live gctx with no error must not be classified as cancellation")
	}
}

func TestRunConcurrentRealErrorsAreNotMisclassifiedAsCancelled(t *testing.T) {
	if runtime.NumCPU() < 2 {
		t.Skip("needs at least two concurrent install slots to hold both jobs inside acquire at once")
	}

	h := testx.Home(t)
	ctx, rep := testx.ReporterContext(context.Background())

	errA := errors.New("checksum mismatch for joba")
	errB := errors.New("checksum mismatch for jobb")

	var bothStarted sync.WaitGroup
	bothStarted.Add(2)
	withAcquire(t, func(_ context.Context, pkg *spec.Package, _ string, _ spec.Platform, _ fetch.Source, _ string, _ report.Task) (string, error) {
		bothStarted.Done()
		bothStarted.Wait()
		switch pkg.Name {
		case "joba":
			return "", errA
		case "jobb":
			return "", errB
		default:
			t.Fatalf("unexpected job %q", pkg.Name)
			return "", nil
		}
	})

	jobs := []Job{
		{Pkg: copyToolPkg("joba"), Version: "v1.0.0", Catalog: "official"},
		{Pkg: copyToolPkg("jobb"), Version: "v1.0.0", Catalog: "official"},
	}

	err := Run(ctx, h, jobs)
	if !errors.Is(err, errA) && !errors.Is(err, errB) {
		t.Fatalf("Run error = %v, want either errA or errB", err)
	}

	for _, tc := range []struct{ name, want string }{
		{"joba", "Failed to install joba v1.0.0"},
		{"jobb", "Failed to install jobb v1.0.0"},
	} {
		task := rep.TaskFor("Installing " + tc.name + " v1.0.0")
		if task == nil {
			t.Fatalf("%s: no task recorded", tc.name)
		}
		done, failed, outcome := task.Snapshot()
		if done || !failed {
			t.Fatalf("%s: done=%v failed=%v, want failed only", tc.name, done, failed)
		}
		if outcome != tc.want {
			t.Fatalf("%s: outcome = %q, want its own failure %q (must not read Cancelled)", tc.name, outcome, tc.want)
		}
	}
}

func TestRunReturnsCtxErrorWhenAllJobsCancelledWithoutOwnError(t *testing.T) {
	h := testx.Home(t)

	baseCtx, cancel := context.WithCancel(context.Background())
	cancel()
	ctx, _ := testx.ReporterContext(baseCtx)

	withAcquire(t, func(context.Context, *spec.Package, string, spec.Platform, fetch.Source, string, report.Task) (string, error) {
		t.Fatal("acquire must not be called when ctx is already cancelled before Run starts")
		return "", nil
	})

	jobs := []Job{
		{Pkg: copyToolPkg("cancelled-a"), Version: "v1.0.0", Catalog: "official"},
		{Pkg: copyToolPkg("cancelled-b"), Version: "v1.0.0", Catalog: "official"},
	}

	err := Run(ctx, h, jobs)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}

	for _, name := range []string{"cancelled-a", "cancelled-b"} {
		if IsInstalled(h, name, "v1.0.0") {
			t.Fatalf("%s: must not be installed", name)
		}
	}
}

func TestRunInstallFailureRemovesArtifact(t *testing.T) {
	h := testx.Home(t)
	ctx, rep := testx.ReporterContext(context.Background())

	pkg := &spec.Package{
		Name:     "badinstall",
		Artifact: spec.Artifact{OCI: ":{{.Version}}"},
		Install: []spec.Action{{
			Copy: &spec.CopyAction{Src: artifactToken, Dst: "../escape"},
		}},
	}

	var artifactPath string
	withAcquire(t, func(_ context.Context, p *spec.Package, _ string, _ spec.Platform, _ fetch.Source, dir string, _ report.Task) (string, error) {
		f, err := os.CreateTemp(dir, p.Name+"-*.artifact")
		if err != nil {
			return "", err
		}
		if _, err := f.WriteString("bytes"); err != nil {
			f.Close()
			return "", err
		}
		if err := f.Close(); err != nil {
			return "", err
		}
		artifactPath = f.Name()
		return f.Name(), nil
	})

	jobs := []Job{{Pkg: pkg, Version: "v1.0.0", Catalog: "official"}}

	err := Run(ctx, h, jobs)
	if err == nil || !strings.Contains(err.Error(), "escapes staging dir") {
		t.Fatalf("Run error = %v, want containment error", err)
	}
	if artifactPath == "" {
		t.Fatal("acquire was never called")
	}
	if _, statErr := os.Lstat(artifactPath); !os.IsNotExist(statErr) {
		t.Fatalf("artifact %s not removed after failed install, stat err = %v", artifactPath, statErr)
	}

	task := rep.TaskFor("Installing badinstall v1.0.0")
	if task == nil {
		t.Fatal("no task recorded")
	}
	done, failed, outcome := task.Snapshot()
	if done || !failed {
		t.Fatalf("task done=%v failed=%v, want failed only", done, failed)
	}
	if outcome != "Failed to install badinstall v1.0.0" {
		t.Fatalf("outcome = %q, want short failure outcome", outcome)
	}
	if IsInstalled(h, "badinstall", "v1.0.0") {
		t.Fatal("must not be installed after a failed install action")
	}
}

func TestRunReinstallReplacesExistingVersion(t *testing.T) {
	h := testx.Home(t)
	pkg := copyToolPkg("dep")

	var acquireCalls int32
	serveBytes := func(content string) {
		withAcquire(t, func(_ context.Context, p *spec.Package, _ string, _ spec.Platform, _ fetch.Source, dir string, _ report.Task) (string, error) {
			atomic.AddInt32(&acquireCalls, 1)
			f, err := os.CreateTemp(dir, p.Name+"-*.artifact")
			if err != nil {
				return "", err
			}
			if _, err := f.WriteString(content); err != nil {
				f.Close()
				return "", err
			}
			if err := f.Close(); err != nil {
				return "", err
			}
			return f.Name(), nil
		})
	}

	installDir, err := h.PackageDir(pkg.Name, "v1.0.0")
	if err != nil {
		t.Fatalf("PackageDir: %v", err)
	}
	readBin := func() string {
		t.Helper()
		got, err := os.ReadFile(filepath.Join(installDir, "bin", "tool"))
		if err != nil {
			t.Fatalf("read bin/tool: %v", err)
		}
		return string(got)
	}

	serveBytes("binary-v1")
	if err := Run(context.Background(), h, []Job{
		{Pkg: pkg, Version: "v1.0.0", Catalog: "official"},
	}); err != nil {
		t.Fatalf("initial Run: %v", err)
	}
	if got := readBin(); got != "binary-v1" {
		t.Fatalf("bin/tool = %q, want %q", got, "binary-v1")
	}
	if atomic.LoadInt32(&acquireCalls) != 1 {
		t.Fatalf("acquireCalls = %d, want 1 after initial install", acquireCalls)
	}

	serveBytes("binary-v2")
	if err := Run(context.Background(), h, []Job{
		{Pkg: pkg, Version: "v1.0.0", Catalog: "official", Reinstall: true},
	}); err != nil {
		t.Fatalf("reinstall Run: %v", err)
	}
	if got := readBin(); got != "binary-v2" {
		t.Fatalf("bin/tool after reinstall = %q, want %q", got, "binary-v2")
	}
	if atomic.LoadInt32(&acquireCalls) != 2 {
		t.Fatalf("acquireCalls = %d, want 2 after reinstall", acquireCalls)
	}
	if !IsInstalled(h, pkg.Name, "v1.0.0") {
		t.Fatal("must still be installed after reinstall")
	}

	if err := Run(context.Background(), h, []Job{
		{Pkg: pkg, Version: "v1.0.0", Catalog: "official"},
	}); err != nil {
		t.Fatalf("skip Run: %v", err)
	}
	if got := readBin(); got != "binary-v2" {
		t.Fatalf("bin/tool after skip = %q, want unchanged %q", got, "binary-v2")
	}
	if atomic.LoadInt32(&acquireCalls) != 2 {
		t.Fatalf("acquireCalls = %d, want still 2 (acquire must not be called on skip)", acquireCalls)
	}
}

func TestRunReinstallsInstalledVersionWithOldLayout(t *testing.T) {
	h := testx.Home(t)
	pkg := copyToolPkg("tool")
	pkg.Deps = []spec.Dep{{Name: "zlib", Kind: spec.DepKindLink}}
	installDir, err := h.PackageDir("tool", "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(installDir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "bin", "tool"), []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installDir, metaFileName), []byte("package: tool\nversion: v1.0.0\ncatalog: official\nbins:\n- bin\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var acquireCalls int32
	withAcquire(t, func(_ context.Context, p *spec.Package, _ string, _ spec.Platform, _ fetch.Source, dir string, _ report.Task) (string, error) {
		atomic.AddInt32(&acquireCalls, 1)
		f, err := os.CreateTemp(dir, p.Name+"-*.artifact")
		if err != nil {
			return "", err
		}
		f.WriteString("new")
		return f.Name(), f.Close()
	})

	job := Job{Pkg: pkg, Version: "v1.0.0", Catalog: "official", Links: map[string]string{"zlib": "1.3.2"}}
	if err := Run(context.Background(), h, []Job{job}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if atomic.LoadInt32(&acquireCalls) != 1 {
		t.Fatalf("acquire calls = %d, want 1 (old layout must reinstall)", acquireCalls)
	}
	if got, _ := os.ReadFile(filepath.Join(installDir, "bin", "tool")); string(got) != "new" {
		t.Fatalf("bin/tool = %q, want the reinstalled binary", got)
	}
	links, err := ReadLinks(installDir)
	if err != nil || links["zlib"] != "1.3.2" {
		t.Fatalf("links after reinstall = %v, %v", links, err)
	}
	meta, err := ReadMeta(h, "tool", "v1.0.0")
	if err != nil || meta.Layout != LayoutVersion {
		t.Fatalf("meta layout = %+v, %v; want %d", meta, err, LayoutVersion)
	}

	if err := Run(context.Background(), h, []Job{job}); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if atomic.LoadInt32(&acquireCalls) != 1 {
		t.Fatalf("acquire calls = %d, want still 1 (current layout must not reinstall)", acquireCalls)
	}
}

func TestRunLeavesOldLayoutAloneWithoutLinkDeps(t *testing.T) {
	h := testx.Home(t)
	pkg := copyToolPkg("plain")
	installDir, _ := h.PackageDir("plain", "v1.0.0")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installDir, metaFileName), []byte("package: plain\nversion: v1.0.0\ncatalog: official\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withAcquire(t, func(context.Context, *spec.Package, string, spec.Platform, fetch.Source, string, report.Task) (string, error) {
		t.Fatal("acquire must not be called for an installed package without link deps")
		return "", nil
	})
	if err := Run(context.Background(), h, []Job{{Pkg: pkg, Version: "v1.0.0", Catalog: "official"}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRunRelinksInstalledDependents(t *testing.T) {
	h := testx.Home(t)
	app := copyToolPkg("app")
	app.Deps = []spec.Dep{{Name: "zlib", Kind: spec.DepKindLink}}
	artifact := filepath.Join(t.TempDir(), "artifact")
	if err := os.WriteFile(artifact, []byte("bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Install(context.Background(), h, app, "1.0.0", "official", artifact, false, map[string]string{"zlib": "1.3.1"}); err != nil {
		t.Fatalf("pre-install: %v", err)
	}
	withAcquire(t, func(context.Context, *spec.Package, string, spec.Platform, fetch.Source, string, report.Task) (string, error) {
		t.Fatal("acquire must not be called to relink an installed package")
		return "", nil
	})

	ctx, rep := testx.ReporterContext(context.Background())
	job := Job{Pkg: app, Version: "1.0.0", Catalog: "official", Links: map[string]string{"zlib": "1.3.2"}}
	if err := Run(ctx, h, []Job{job}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	appDir, _ := h.PackageDir("app", "1.0.0")
	links, err := ReadLinks(appDir)
	if err != nil || links["zlib"] != "1.3.2" {
		t.Fatalf("app links = %v, %v; want zlib 1.3.2", links, err)
	}
	if n := len(rep.Tasks()); n != 0 {
		t.Fatalf("task count = %d, want none for a relink", n)
	}
}

func TestRunLeavesLinksAloneWhenAnInstallFails(t *testing.T) {
	h := testx.Home(t)
	app := copyToolPkg("app")
	app.Deps = []spec.Dep{{Name: "zlib", Kind: spec.DepKindLink}}
	artifact := filepath.Join(t.TempDir(), "artifact")
	if err := os.WriteFile(artifact, []byte("bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Install(context.Background(), h, app, "1.0.0", "official", artifact, false, map[string]string{"zlib": "1.3.1"}); err != nil {
		t.Fatalf("pre-install: %v", err)
	}
	notFound := errors.New("archive not found")
	withAcquire(t, func(context.Context, *spec.Package, string, spec.Platform, fetch.Source, string, report.Task) (string, error) {
		return "", notFound
	})

	ctx, _ := testx.ReporterContext(context.Background())
	jobs := []Job{
		{Pkg: app, Version: "1.0.0", Catalog: "official", Links: map[string]string{"zlib": "1.3.2"}},
		{Pkg: copyToolPkg("zlib"), Version: "1.3.2", Catalog: "official"},
	}
	if err := Run(ctx, h, jobs); !errors.Is(err, notFound) {
		t.Fatalf("Run error = %v, want the failed install", err)
	}
	appDir, _ := h.PackageDir("app", "1.0.0")
	links, err := ReadLinks(appDir)
	if err != nil || links["zlib"] != "1.3.1" {
		t.Fatalf("app links = %v, %v; a dependent must keep its links when an install in the same run fails", links, err)
	}
}
