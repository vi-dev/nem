package build

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

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

func runBuild(t *testing.T, h home.Home, pkg *spec.Package, opts Options) (string, error) {
	t.Helper()
	var b bytes.Buffer
	ctx := report.NewContext(context.Background(), report.New(&b, &b, report.Options{}))
	err := Build(ctx, h, nil, pkg, opts)
	return b.String(), err
}

func runBuildCapture(t *testing.T, h home.Home, pkg *spec.Package, opts Options) ([]byte, string, error) {
	t.Helper()
	var data []byte
	userTest := opts.Test
	opts.Test = func(ctx context.Context, p *spec.Package, version, artifactPath string) error {
		b, err := os.ReadFile(artifactPath)
		if err != nil {
			return err
		}
		data = b
		if userTest != nil {
			return userTest(ctx, p, version, artifactPath)
		}
		return nil
	}
	out, err := runBuild(t, h, pkg, opts)
	return data, out, err
}

func readArchiveFile(t *testing.T, data []byte, name string) string {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("open archive gzip stream: %v", err)
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			t.Fatalf("%s not found in archive", name)
		}
		if err != nil {
			t.Fatalf("read archive: %v", err)
		}
		if hdr.Name != name {
			continue
		}
		content, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read %s from archive: %v", name, err)
		}
		return string(content)
	}
}

func otherPlatform() spec.Platform {
	if spec.Current().OS == "linux" {
		return spec.Platform{OS: "darwin"}
	}
	return spec.Platform{OS: "linux"}
}

func TestBuildRunsStepsAndVerifies(t *testing.T) {
	body := makeTarGz(t, map[string]string{"src/README": "hi"})
	srv, sha := serve(t, body)
	h := testx.HomeAt(t.TempDir())
	pkg := buildPkg("v1.0.0", spec.BuildStep{Run: "mkdir -p \"$NEM_OUTPUT/bin\" && echo \"$NEM_VERSION\" > \"$NEM_OUTPUT/bin/ver\""})
	pkg.Build.Source.URL = srv.URL

	data, out, err := runBuildCapture(t, h, pkg, Options{Version: "v1.0.0"})
	if err != nil {
		t.Fatalf("Build: %v\n%s", err, out)
	}
	if got := readArchiveFile(t, data, "bin/ver"); got != "v1.0.0\n" {
		t.Fatalf("step did not run against NEM_* env; ver=%q", got)
	}
	want := fmt.Sprintf("Record for reproducibility: sourceSha256: %s", sha)
	if !strings.Contains(out, want) {
		t.Fatalf("no sourceSha256 pinned → TOFU must narrate the computed sourceSha256 %q, got %q", want, out)
	}
}

func TestBuildSkipsTOFURecordForVerifiedSource(t *testing.T) {
	body := makeTarGz(t, map[string]string{"src/README": "hi"})
	srv, sha := serve(t, body)
	h := testx.HomeAt(t.TempDir())
	pkg := buildPkg("v1.0.0", spec.BuildStep{Run: `mkdir -p "$NEM_OUTPUT"`})
	pkg.Build.Source.URL = srv.URL
	pkg.Versions[0].SourceSha256 = sha

	out, err := runBuild(t, h, pkg, Options{Version: "v1.0.0"})
	if err != nil {
		t.Fatalf("Build: %v\n%s", err, out)
	}
	if strings.Contains(out, "Record for reproducibility") {
		t.Fatalf("pinned sourceSha256 must be verified, not TOFU-recorded: %q", out)
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

	data, out, err := runBuildCapture(t, h, pkg, Options{Version: "v1"})
	if err != nil {
		t.Fatalf("Build: %v\n%s", err, out)
	}
	paths := readArchiveFile(t, data, "paths")
	lines := strings.Split(strings.TrimSpace(paths), "\n")
	if len(lines) != 2 || lines[0] != lines[1] {
		t.Fatalf("step saw $PWD = %q, but nem names the same dir %q", lines[0], lines[len(lines)-1])
	}
}

func TestBuildFailsOnStepError(t *testing.T) {
	h, pkg := buildFixture(t, map[string]string{"src/x": "y"}, "v1", spec.BuildStep{Run: "exit 3"})
	if _, err := runBuild(t, h, pkg, Options{Version: "v1"}); err == nil {
		t.Fatal("want error when a build step exits non-zero")
	}
}

func TestBuildNarratesSourceDownload(t *testing.T) {
	h, pkg := buildFixture(t, map[string]string{"src/README": "hi"}, "v1.0.0",
		spec.BuildStep{Run: `mkdir -p "$NEM_OUTPUT"`})

	out, err := runBuild(t, h, pkg, Options{Version: "v1.0.0"})
	if err != nil {
		t.Fatalf("Build: %v\n%s", err, out)
	}
	want := fmt.Sprintf("Downloaded source for %s %s", pkg.Name, "v1.0.0")
	if !strings.Contains(out, want) {
		t.Fatalf("narration missing source download done line %q, got %q", want, out)
	}
}

func TestBuildQuietSuppressesSourceDownloadDone(t *testing.T) {
	h, pkg := buildFixture(t, map[string]string{"src/README": "hi"}, "v1.0.0",
		spec.BuildStep{Run: `mkdir -p "$NEM_OUTPUT"`})

	var b bytes.Buffer
	ctx := report.NewContext(context.Background(), report.New(&b, &b, report.Options{Quiet: true}))
	err := Build(ctx, h, nil, pkg, Options{Version: "v1.0.0"})
	if err != nil {
		t.Fatalf("Build: %v\n%s", err, b.String())
	}
	if strings.Contains(b.String(), "Downloaded source for") {
		t.Fatalf("quiet did not suppress source download done line: %q", b.String())
	}
}

func TestBuildSourceDownloadReportsLiveByteProgress(t *testing.T) {

	rnd := rand.New(rand.NewSource(1))
	content := make([]byte, 700*1024)
	if _, err := rnd.Read(content); err != nil {
		t.Fatalf("fill random content: %v", err)
	}
	body := makeTarGz(t, map[string]string{"src/big": string(content)})
	total := len(body)
	chunk := total * 3 / 5
	if chunk < 256*1024 {
		t.Fatalf("archive too small to cross the progress chunk threshold: %d bytes", total)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(total))
		w.WriteHeader(http.StatusOK)
		w.Write(body[:chunk])
		w.(http.Flusher).Flush()
		time.Sleep(300 * time.Millisecond)
		w.Write(body[chunk:])
	}))
	t.Cleanup(srv.Close)

	h := testx.HomeAt(t.TempDir())
	pkg := buildPkg("v1.0.0", spec.BuildStep{Run: `mkdir -p "$NEM_OUTPUT"`})
	pkg.Build.Source.URL = srv.URL

	var b bytes.Buffer
	ctx := report.NewContext(context.Background(), report.New(&b, &b, report.Options{IsTTY: true, Color: report.ColorNever}))
	err := Build(ctx, h, nil, pkg, Options{Version: "v1.0.0"})
	if err != nil {
		t.Fatalf("Build: %v\n%s", err, b.String())
	}

	if !regexp.MustCompile(`downloading \d+%`).MatchString(b.String()) {
		t.Fatalf("no live byte-progress line seen during source download: %q", b.String())
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
			data, out, err := runBuildCapture(t, h, pkg, Options{Version: "v1"})
			if !tt.wantMarker {
				if err == nil || !strings.Contains(err.Error(), "no build step applies to "+spec.Current().String()) {
					t.Fatalf("want no-applicable-step error naming the platform, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Build: %v\n%s", err, out)
			}
			if got := readArchiveFile(t, data, "marker"); got != "ran\n" {
				t.Fatalf("matching step did not run; marker=%q", got)
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
	sources := catalog.NewSet(catalog.Entry{Name: "cat", Catalog: catalog.NewDir(catalogRoot)})

	depPkg, _, err := catalog.NewDir(catalogRoot).Package(context.Background(), "dep")
	if err != nil {
		t.Fatalf("load dep pkg: %v", err)
	}
	artifactPath := filepath.Join(t.TempDir(), "dep.tar.gz")
	if err := os.WriteFile(artifactPath, depArchive, 0o644); err != nil {
		t.Fatalf("write dep artifact: %v", err)
	}
	if err := install.Install(context.Background(), h, depPkg, "9.9.9", "cat", artifactPath, false); err != nil {
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
	ctx := report.NewContext(context.Background(), report.New(&b, &b, report.Options{}))
	if err := Build(ctx, h, sources, pkg, Options{Version: "v1.0.0"}); err != nil {
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
	data, out, err := runBuildCapture(t, h, pkg,
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
	if got := readArchiveFile(t, data, "bin/tool"); got != "hi\n" {
		t.Fatalf("archive handed to the test hook does not contain the build output; bin/tool=%q", got)
	}
	if _, err := os.Stat(gotArtifact); !os.IsNotExist(err) {
		t.Fatalf("the temporary archive must be removed, stat err = %v", err)
	}
}

func TestBuildFailsWhenTheTestHookFails(t *testing.T) {
	h, pkg := buildFixture(t, map[string]string{"src/README": "hi"}, "v1",
		spec.BuildStep{Run: `mkdir -p "$NEM_OUTPUT/bin" && echo hi > "$NEM_OUTPUT/bin/marker"`})

	store := ocix.NewArchiveStore(t.TempDir())
	var hookCalled bool
	_, err := runBuild(t, h, pkg,
		Options{Version: "v1", LocalStore: store, Test: func(context.Context, *spec.Package, string, string) error {
			hookCalled = true
			return errors.New("boom")
		}})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("want the hook's error, got %v", err)
	}
	if !hookCalled {
		t.Fatal("the test hook was never called")
	}
	if store.Has(pkg.Name) {
		t.Fatal("a failing test must leave nothing staged in the local store")
	}
}

func TestBuildTestHookAndLocalStoreShareTheSameArchiveBytes(t *testing.T) {
	h, pkg := buildFixture(t, map[string]string{"src/README": "hi"}, "v1.0.0",
		spec.BuildStep{Run: `mkdir -p "$NEM_OUTPUT/bin" && echo hello > "$NEM_OUTPUT/bin/tool"`})

	store := ocix.NewArchiveStore(t.TempDir())
	var hookBytes []byte
	out, err := runBuild(t, h, pkg,
		Options{Version: "v1.0.0", LocalStore: store,
			Test: func(_ context.Context, _ *spec.Package, _, artifactPath string) error {
				data, readErr := os.ReadFile(artifactPath)
				if readErr != nil {
					return readErr
				}
				hookBytes = data
				return nil
			}})
	if err != nil {
		t.Fatalf("build with test hook and local store: %v\n%s", err, out)
	}
	if len(hookBytes) == 0 {
		t.Fatal("the test hook never read the archive")
	}

	target, err := store.Open(pkg.Name)
	if err != nil {
		t.Fatalf("open local store target: %v", err)
	}
	staged, err := ocix.ReadArchive(context.Background(), target, "v1.0.0", spec.Current())
	if err != nil {
		t.Fatalf("read staged archive: %v", err)
	}
	if !bytes.Equal(hookBytes, staged) {
		t.Fatal("bytes handed to the test hook differ from the bytes staged")
	}
}

func TestBuildStagesArchiveInLocalStore(t *testing.T) {
	h, pkg := buildFixture(t, map[string]string{"src/README": "hi"}, "v1.0.0",
		spec.BuildStep{Run: "mkdir -p \"$NEM_OUTPUT/bin\" && echo \"$NEM_VERSION\" > \"$NEM_OUTPUT/bin/ver\""})

	store := ocix.NewArchiveStore(t.TempDir())
	out, err := runBuild(t, h, pkg, Options{Version: "v1.0.0", LocalStore: store})
	if err != nil {
		t.Fatalf("Build: %v\n%s", err, out)
	}

	matches, err := filepath.Glob(filepath.Join(h.Tmp(), "*"+home.BuildStagingInfix+"*"))
	if err != nil {
		t.Fatalf("glob staging dirs: %v", err)
	}
	if len(matches) > 0 {
		t.Fatalf("staging dir(s) left behind after a successful build: %v", matches)
	}

	if !store.Has(pkg.Name) {
		t.Fatalf("built archive was not staged in the local store for %s", pkg.Name)
	}
	target, err := store.Open(pkg.Name)
	if err != nil {
		t.Fatalf("open local store target: %v", err)
	}
	data, err := ocix.ReadArchive(context.Background(), target, "v1.0.0", spec.Current())
	if err != nil {
		t.Fatalf("read staged archive: %v", err)
	}
	if got := readArchiveFile(t, data, "bin/ver"); got != "v1.0.0\n" {
		t.Fatalf("step did not run against NEM_* env; ver=%q", got)
	}
}

func TestBuildContinuesWhenStagingSweepFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unremovable directories via chmod are unix-specific")
	}

	h, pkg := buildFixture(t, map[string]string{"src/README": "hi"}, "v1.0.0",
		spec.BuildStep{Run: `mkdir -p "$NEM_OUTPUT/bin" && echo hi > "$NEM_OUTPUT/bin/tool"` + "\n" +
			`mkdir -p "$NEM_STAGING_DIR/blocked" && touch "$NEM_STAGING_DIR/blocked/x" && chmod 000 "$NEM_STAGING_DIR/blocked"`})

	store := ocix.NewArchiveStore(t.TempDir())
	out, err := runBuild(t, h, pkg, Options{Version: "v1.0.0", LocalStore: store})

	matches, globErr := filepath.Glob(filepath.Join(h.Tmp(), pkg.Name+home.BuildStagingInfix+"*"))
	if globErr != nil {
		t.Fatalf("glob staging dirs: %v", globErr)
	}
	for _, m := range matches {
		blocked := filepath.Join(m, "blocked")
		t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) })
	}
	if len(matches) == 0 {
		t.Skip("this environment does not enforce the permission needed to force a sweep failure (e.g. running as root)")
	}

	if err != nil {
		t.Fatalf("a staging sweep failure must not fail an already-built, verified package: %v\n%s", err, out)
	}
	if !strings.Contains(out, "sweep build staging") {
		t.Fatalf("the sweep failure was not narrated:\n%s", out)
	}
	if !store.Has(pkg.Name) {
		t.Fatal("built archive was not staged in the local store despite the sweep failure")
	}
}
