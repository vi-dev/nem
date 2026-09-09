package build

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/oci"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/home"
	"github.com/vi-dev/nem/internal/install"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/report"
	"github.com/vi-dev/nem/internal/spec"
	"github.com/vi-dev/nem/internal/testx"
	"github.com/vi-dev/nem/internal/usage"
)

func buildPkg(version string, steps ...spec.BuildStep) *spec.Package {
	return &spec.Package{Schema: 2, Name: "tool",
		Artifact: spec.Artifact{OCI: ":{{.Version}}"},
		Install:  []spec.Action{{Extract: &spec.ExtractAction{}}},
		Versions: []spec.VersionEntry{{Version: version}},
		Build:    &spec.Build{Output: "out", Steps: steps},
	}
}

func buildFixture(t *testing.T, srcFiles map[string]string, version string, steps ...spec.BuildStep) (home.Home, *spec.Package) {
	t.Helper()
	h := testx.HomeAt(t.TempDir())
	srv, _ := serve(t, makeTarGz(t, srcFiles))
	pkg := buildPkg(version, steps...)
	pkg.Build.Source.URL = srv.URL
	return h, pkg
}

func runBuild(t *testing.T, h home.Home, pkg *spec.Package, opts Options) (Result, string, error) {
	t.Helper()
	var b bytes.Buffer
	res, err := Build(context.Background(), h, nil, nil, pkg, opts,
		report.New(&b, &b, report.Options{}), &b, &b)
	return res, b.String(), err
}

func stubArchiveStore(t *testing.T) *oci.Store {
	t.Helper()
	store, err := oci.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	testx.Swap(t, &archivesOpener, func(catalogRef, name string) (oras.Target, error) { return store, nil })
	return store
}

func otherPlatform() spec.Platform {
	if spec.Current().OS == "linux" {
		return spec.Platform{OS: "darwin"}
	}
	return spec.Platform{OS: "linux"}
}

func TestBuildRunsStepsAndVerifies(t *testing.T) {
	h, pkg := buildFixture(t, map[string]string{"src/README": "hi"}, "v1.0.0",
		spec.BuildStep{Run: "mkdir -p \"$NEM_OUTPUT/bin\" && echo \"$NEM_VERSION\" > \"$NEM_OUTPUT/bin/ver\""})

	res, out, err := runBuild(t, h, pkg, Options{Version: "v1.0.0"})
	if err != nil {
		t.Fatalf("Build: %v\n%s", err, out)
	}
	got, _ := os.ReadFile(filepath.Join(res.OutputDir, "bin", "ver"))
	if string(got) != "v1.0.0\n" {
		t.Fatalf("step did not run against NEM_* env; ver=%q", got)
	}
	if res.SourceVerified {
		t.Fatal("no sourceSha256 pinned → SourceVerified must be false (TOFU)")
	}
	if res.SourceSha256 == "" {
		t.Fatal("TOFU must report the computed sourceSha256")
	}
}

func TestBuildStepPWDMatchesTheNemPaths(t *testing.T) {
	linked := filepath.Join(t.TempDir(), "home")
	if err := os.Symlink(t.TempDir(), linked); err != nil {
		t.Fatal(err)
	}
	h := testx.HomeAt(linked)
	srv, _ := serve(t, makeTarGz(t, map[string]string{"src/x": "y"}))
	pkg := buildPkg("v1", spec.BuildStep{Run: `
mkdir -p "$NEM_OUTPUT"
printf '%s\n%s\n' "$PWD" "$(dirname "$NEM_OUTPUT")" > "$NEM_OUTPUT/paths"
`})
	pkg.Build.Source.URL = srv.URL

	res, out, err := runBuild(t, h, pkg, Options{Version: "v1"})
	if err != nil {
		t.Fatalf("Build: %v\n%s", err, out)
	}
	paths, err := os.ReadFile(filepath.Join(res.OutputDir, "paths"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(paths)), "\n")
	if len(lines) != 2 || lines[0] != lines[1] {
		t.Fatalf("step saw $PWD = %q, but nem names the same dir %q", lines[0], lines[len(lines)-1])
	}
}

func TestBuildFailsOnStepError(t *testing.T) {
	h, pkg := buildFixture(t, map[string]string{"src/x": "y"}, "v1", spec.BuildStep{Run: "exit 3"})
	if _, _, err := runBuild(t, h, pkg, Options{Version: "v1"}); err == nil {
		t.Fatal("want error when a build step exits non-zero")
	}
}

func TestBuildPlatformStepFiltering(t *testing.T) {
	nonMatching := spec.BuildStep{Run: "exit 7", Platforms: []spec.Platform{otherPlatform()}}
	matching := spec.BuildStep{Run: `mkdir -p "$NEM_OUTPUT" && echo ran > "$NEM_OUTPUT/marker"`,
		Platforms: []spec.Platform{{OS: spec.Current().OS}}}

	tests := []struct {
		name       string
		steps      []spec.BuildStep
		wantMarker bool
	}{
		{name: "skips non-matching steps", steps: []spec.BuildStep{nonMatching, matching}, wantMarker: true},
		{name: "fails when all steps filtered", steps: []spec.BuildStep{nonMatching}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, pkg := buildFixture(t, map[string]string{"src/x": "y"}, "v1", tt.steps...)
			res, out, err := runBuild(t, h, pkg, Options{Version: "v1"})
			if !tt.wantMarker {
				if err == nil || !strings.Contains(err.Error(), "no build step applies to "+spec.Current().String()) {
					t.Fatalf("want no-applicable-step error naming the platform, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Build: %v\n%s", err, out)
			}
			got, _ := os.ReadFile(filepath.Join(res.OutputDir, "marker"))
			if string(got) != "ran\n" {
				t.Fatalf("matching step did not run; marker=%q", got)
			}
		})
	}
}

func TestBuildPushRoundTripsThroughArchive(t *testing.T) {
	store := stubArchiveStore(t)
	h, pkg := buildFixture(t, map[string]string{"src/README": "hi"}, "v1.0.0",
		spec.BuildStep{Run: `mkdir -p "$NEM_OUTPUT/bin" && echo hello > "$NEM_OUTPUT/bin/tool"`})

	res, out, err := runBuild(t, h, pkg, Options{Version: "v1.0.0", Push: "ghcr.io/x/cat:v2"})
	if err != nil {
		t.Fatalf("build --push: %v\n%s", err, out)
	}
	if !res.Pushed {
		t.Fatal("Result.Pushed should be true")
	}

	pulled, err := ocix.PullArchiveFrom(context.Background(), store, "v1.0.0", spec.Current(), t.TempDir())
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if err := install.Install(context.Background(), h, pkg, "v1.0.0", "cat", pulled); err != nil {
		t.Fatalf("install pulled archive: %v", err)
	}
	dir, _ := h.PackageDir("tool", "v1.0.0")
	if got, _ := os.ReadFile(filepath.Join(dir, "bin", "tool")); string(got) != "hello\n" {
		t.Fatalf("installed tool = %q, want hello", got)
	}
}

func TestBuildPushesNothing(t *testing.T) {
	tests := []struct {
		name    string
		opts    Options
		wantErr string
	}{
		{name: "dry run",
			opts: Options{Version: "v1.0.0", Push: "ghcr.io/x/cat:v2", DryRun: true}},
		{name: "failing test hook",
			opts: Options{Version: "v1.0.0", Push: "ghcr.io/x/cat:v2",
				Test: func(context.Context, *spec.Package, string, string) error {
					return errors.New("boom")
				}},
			wantErr: "boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := stubArchiveStore(t)
			h, pkg := buildFixture(t, map[string]string{"src/README": "hi"}, "v1.0.0",
				spec.BuildStep{Run: `mkdir -p "$NEM_OUTPUT/bin" && echo hello > "$NEM_OUTPUT/bin/tool"`})

			res, out, err := runBuild(t, h, pkg, tt.opts)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Build: %v\n%s", err, out)
				}
				if res.Pushed {
					t.Fatal("Result.Pushed should be false on dry-run")
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("want the hook's error, got %v", err)
			}

			if _, err := ocix.PullArchiveFrom(context.Background(), store, "v1.0.0", spec.Current(), t.TempDir()); !errors.Is(err, ocix.ErrArchiveNotFound) {
				t.Fatalf("%s pushed something: pull err = %v, want %v", tt.name, err, ocix.ErrArchiveNotFound)
			}
		})
	}
}

func depDirCatalog(t *testing.T, name, version string, depArchive []byte) string {
	t.Helper()
	srv, sha := serve(t, depArchive)

	root := t.TempDir()
	dir := filepath.Join(root, "pkgs", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	yaml := `
schema: 2
name: ` + name + `
artifact:
  url: "` + srv.URL + `"
install:
  - extract: {}
libs: ["lib"]
versions:
  - version: "` + version + `"
    sha256:
      darwin/arm64: "` + sha + `"
      darwin/amd64: "` + sha + `"
      linux/arm64: "` + sha + `"
      linux/amd64: "` + sha + `"
`
	if err := os.WriteFile(filepath.Join(dir, "pkg.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatalf("write dep pkg.yaml: %v", err)
	}
	return root
}

func TestBuildRestampsAlreadyInstalledBuildDep(t *testing.T) {
	h := testx.HomeAt(t.TempDir())

	depArchive := makeTarGz(t, map[string]string{"lib/libdep.so": "dep bytes"})
	catalogRoot := depDirCatalog(t, "dep", "9.9.9", depArchive)
	sources := []catalog.Named{{Name: "cat", Source: catalog.NewDir(catalogRoot)}}

	depPkg, _, err := catalog.NewDir(catalogRoot).Load(context.Background(), "dep")
	if err != nil {
		t.Fatalf("load dep pkg: %v", err)
	}
	artifactPath := filepath.Join(t.TempDir(), "dep.tar.gz")
	if err := os.WriteFile(artifactPath, depArchive, 0o644); err != nil {
		t.Fatalf("write dep artifact: %v", err)
	}
	if err := install.Install(context.Background(), h, depPkg, "9.9.9", "cat", artifactPath); err != nil {
		t.Fatalf("pre-install dep: %v", err)
	}

	old := time.Now().Add(-2 * time.Hour)
	idx := usage.Load(h)
	idx[usage.Key("dep", "9.9.9")] = old
	if err := usage.Save(h, idx); err != nil {
		t.Fatalf("backdate dep stamp: %v", err)
	}

	srv, _ := serve(t, makeTarGz(t, map[string]string{"src/README": "hi"}))
	pkg := &spec.Package{Schema: 2, Name: "consumer",
		Artifact: spec.Artifact{OCI: ":{{.Version}}"},
		Install:  []spec.Action{{Extract: &spec.ExtractAction{}}},
		Versions: []spec.VersionEntry{{Version: "v1.0.0"}},
		Build: &spec.Build{
			Output: "out",
			Deps:   []spec.Dep{{Name: "dep", Version: "9.9.9"}},
			Steps: []spec.BuildStep{
				{Run: `mkdir -p "$NEM_OUTPUT/bin" && echo hi > "$NEM_OUTPUT/bin/consumer"`},
			},
		}}
	pkg.Build.Source.URL = srv.URL

	var b bytes.Buffer
	if _, err := Build(context.Background(), h, nil, sources, pkg, Options{Version: "v1.0.0"},
		report.New(&b, &b, report.Options{}), &b, &b); err != nil {
		t.Fatalf("Build: %v\n%s", err, b.String())
	}

	got := usage.Load(h)
	if _, ok := got.LastUsed("consumer", "v1.0.0"); !ok {
		t.Error("built package was not stamped")
	}
	depStamp, ok := got.LastUsed("dep", "9.9.9")
	if !ok {
		t.Fatal("dep stamp missing after build")
	}
	if !depStamp.After(old) {
		t.Fatalf("build did not refresh an already-installed dep's stamp: got %v, want after %v", depStamp, old)
	}
}

func TestBuildTestHookGetsAnInstallableArchive(t *testing.T) {
	h, pkg := buildFixture(t, map[string]string{"src/README": "hi"}, "v1",
		spec.BuildStep{Run: `mkdir -p "$NEM_OUTPUT/bin" && echo hi > "$NEM_OUTPUT/bin/tool"`})

	var gotArtifact string
	res, out, err := runBuild(t, h, pkg,
		Options{Version: "v1", Test: func(_ context.Context, _ *spec.Package, _, artifactPath string) error {
			gotArtifact = artifactPath
			info, statErr := os.Stat(artifactPath)
			if statErr != nil {
				return statErr
			}
			if info.Size() == 0 {
				return errors.New("archive is empty")
			}
			return nil
		}})
	if err != nil {
		t.Fatalf("Build: %v\n%s", err, out)
	}
	if gotArtifact == "" {
		t.Fatal("the test hook was never called")
	}
	if _, err := os.Stat(filepath.Join(res.OutputDir, "bin", "tool")); err != nil {
		t.Fatalf("output tree must be intact at Result.OutputDir: %v", err)
	}
	if _, err := os.Stat(gotArtifact); !os.IsNotExist(err) {
		t.Fatalf("the temporary archive must be removed, stat err = %v", err)
	}
}

func findDirNamed(t *testing.T, root, want string) string {
	t.Helper()
	var found string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == want {
			if found != "" {
				t.Fatalf("more than one %q directory under %s", want, root)
			}
			found = path
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if found == "" {
		t.Fatalf("no %q directory found under %s", want, root)
	}
	return found
}

func snapshotTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	got := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		got[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return got
}

func TestBuildFailsWhenTheTestHookFails(t *testing.T) {
	h, pkg := buildFixture(t, map[string]string{"src/README": "hi"}, "v1",
		spec.BuildStep{Run: `mkdir -p "$NEM_OUTPUT/bin" && echo hi > "$NEM_OUTPUT/bin/marker"`})

	var outputDir string
	var before map[string]string
	_, _, err := runBuild(t, h, pkg,
		Options{Version: "v1", Test: func(context.Context, *spec.Package, string, string) error {
			outputDir = findDirNamed(t, h.Tmp(), pkg.Build.Output)
			before = snapshotTree(t, outputDir)
			return errors.New("boom")
		}})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("want the hook's error, got %v", err)
	}
	if outputDir == "" {
		t.Fatal("the test hook was never called")
	}
	if _, statErr := os.Stat(outputDir); statErr != nil {
		t.Fatalf("output tree must still be present after a failing test: %v", statErr)
	}
	after := snapshotTree(t, outputDir)
	if !maps.Equal(before, after) {
		t.Fatalf("output tree changed after a failing test:\nbefore: %v\nafter:  %v", before, after)
	}
}

func TestBuildTestHookAndPushShareTheSameArchiveBytes(t *testing.T) {
	store := stubArchiveStore(t)
	h, pkg := buildFixture(t, map[string]string{"src/README": "hi"}, "v1.0.0",
		spec.BuildStep{Run: `mkdir -p "$NEM_OUTPUT/bin" && echo hello > "$NEM_OUTPUT/bin/tool"`})

	var hookBytes []byte
	res, out, err := runBuild(t, h, pkg,
		Options{Version: "v1.0.0", Push: "ghcr.io/x/cat:v2",
			Test: func(_ context.Context, _ *spec.Package, _, artifactPath string) error {
				data, readErr := os.ReadFile(artifactPath)
				if readErr != nil {
					return readErr
				}
				hookBytes = data
				return nil
			}})
	if err != nil {
		t.Fatalf("build with test hook and push: %v\n%s", err, out)
	}
	if !res.Pushed {
		t.Fatal("Result.Pushed should be true")
	}
	if len(hookBytes) == 0 {
		t.Fatal("the test hook never read the archive")
	}

	pulled, err := ocix.PullArchiveFrom(context.Background(), store, "v1.0.0", spec.Current(), t.TempDir())
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	pushedBytes, err := os.ReadFile(pulled)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(hookBytes, pushedBytes) {
		t.Fatal("bytes handed to the test hook differ from the bytes pushed")
	}
}
