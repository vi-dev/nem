package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vi-dev/nem/internal/build"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/ocix/ocixtest"
	"github.com/vi-dev/nem/internal/report"
	"github.com/vi-dev/nem/internal/spec"
	"github.com/vi-dev/nem/internal/testx"
)

func batchRecipe(name, sourceURL, extra, step string) string {
	return "schema: 2\nname: " + name + "\n" +
		"artifact: {oci: \":{{.Version}}\"}\ninstall: [{extract: {}}]\n" +
		"versions: [v1.0.0]\nbuild:\n  source: {url: \"" + sourceURL + "\"}\n" +
		extra +
		"  output: out\n  steps:\n    - run: " + step + "\n"
}

func markerStep(dir, name string) string {
	return `mkdir -p "$NEM_OUTPUT" && echo x > "$NEM_OUTPUT/marker" && touch ` +
		filepath.Join(dir, name)
}

func tableRow(out string, cell int, want string) []string {
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) > cell && f[cell] == want {
			return f
		}
	}
	return nil
}

func stageOverlayArchive(t *testing.T, dir, name, version string, platforms map[string][]byte) {
	t.Helper()
	layout, err := ocix.NewArchiveStore(dir).Open(name)
	if err != nil {
		t.Fatalf("open overlay layout for %s: %v", name, err)
	}
	ocixtest.PushFakeArchive(t, layout, version, platforms)
}

func TestCatalogBuildBatchDryRunRendersWavePlan(t *testing.T) {
	nemHome := t.TempDir()
	dir := writeLintFixture(t, map[string]string{
		"blib": batchRecipe("blib", "https://example.com/blib.tar.gz", "", "make"),
		"bapp": batchRecipe("bapp", "https://example.com/bapp.tar.gz",
			"  deps: [{name: blib}]\n", "make"),
	})

	out, errb, err := runNem(t, nemHome, "catalog", "build", dir,
		"--missing", "--push", "--dry-run")
	if err != nil {
		t.Fatalf("catalog build --dry-run: %v\nstderr: %s", err, errb)
	}
	if strings.Contains(errb, "Pulling catalog") || strings.Contains(errb, "Pulled catalog") {
		t.Fatalf("a directory target must open silently, with no pull narration:\n%s", errb)
	}
	for _, h := range []string{"WAVE", "PACKAGE", "VERSION", "REASON", "NEEDS", "PLATFORMS"} {
		if !strings.Contains(out, h) {
			t.Fatalf("plan table is missing the %s column:\n%s", h, out)
		}
	}

	lib := tableRow(out, 1, "blib")
	if lib == nil {
		t.Fatalf("no blib row in the plan:\n%s", out)
	}
	if lib[0] != "1" || lib[len(lib)-2] != "-" {
		t.Fatalf("blib must be wave 1 with no in-batch needs, got %v", lib)
	}
	if !strings.Contains(strings.Join(lib, " "), "missing archive") {
		t.Fatalf("blib REASON must be the missing-archive reason, got %v", lib)
	}
	if !strings.Contains(lib[len(lib)-1], "linux/amd64") {
		t.Fatalf("blib PLATFORMS must list the absent platforms, got %v", lib)
	}
	app := tableRow(out, 1, "bapp")
	if app == nil {
		t.Fatalf("no bapp row in the plan:\n%s", out)
	}
	if app[0] != "2" {
		t.Fatalf("bapp must land in wave 2 behind its dep, got %v", app)
	}
	if app[len(app)-2] != "blib" {
		t.Fatalf("bapp NEEDS must name blib, got %v", app)
	}
	if !strings.Contains(errb, "would push") {
		t.Fatalf("--push --dry-run must narrate the pushes it would make:\n%s", errb)
	}
}

func twoVersionRecipe(name string) string {
	return "schema: 2\nname: " + name + "\n" +
		"artifact: {oci: \":{{.Version}}\"}\ninstall: [{extract: {}}]\n" +
		"versions: [2.0.0, 1.0.0]\n" +
		"build:\n  source: {url: \"https://example.com/" + name + "-{{.Version}}.tar.gz\"}\n" +
		"  output: out\n  steps:\n    - run: make\n"
}

func planVersions(out, name string) []string {
	var versions []string
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) > 2 && f[1] == name {
			versions = append(versions, f[2])
		}
	}
	return versions
}

func TestCatalogBuildMissingPlansAllIncompleteVersions(t *testing.T) {
	nemHome := t.TempDir()
	dir := writeLintFixture(t, map[string]string{"zdup": twoVersionRecipe("zdup")})

	elsewhere := otherPlatform(t)
	complete := map[string][]byte{}
	for _, p := range spec.SupportedPlatforms {
		if p != elsewhere {
			complete[p.String()] = []byte(p.String())
		}
	}
	stageOverlayArchive(t, dir, "zdup", "2.0.0", complete)

	out, errb, err := runNem(t, nemHome, "catalog", "build", dir, "--missing", "--dry-run")
	if err != nil {
		t.Fatalf("catalog build --missing --dry-run: %v\nstderr: %s", err, errb)
	}
	got := planVersions(out, "zdup")
	if len(got) != 2 || got[0] != "2.0.0" || got[1] != "1.0.0" {
		t.Fatalf("zdup plan versions = %v, want both incomplete versions newest-first:\n%s", got, out)
	}

	if row := tableRow(out, 2, "2.0.0"); row == nil || row[len(row)-1] != elsewhere.String() {
		t.Fatalf("zdup@2.0.0 row = %v, want %s as its only absent platform:\n%s", row, elsewhere, out)
	}
}

func TestRenderSummaryFailsAndHintsOnPushFailure(t *testing.T) {
	var out, errb bytes.Buffer
	testx.Swap(t, &console, report.New(&out, &errb, report.Options{Color: report.ColorNever}))

	err := renderSummary([]build.SummaryRow{
		{Name: "alpha", Version: "1.0.0", Result: "pushed", Detail: "-"},
		{Name: "beta", Version: "2.0.0", Result: "built", Detail: "push failed: boom", PushFailed: true},
	})
	if err == nil {
		t.Fatal("a requested push that failed must exit non-zero: the archive outlived nothing")
	}
	if !strings.Contains(err.Error(), "failed to push") {
		t.Fatalf("err = %v, want the push-failure count", err)
	}

	if !strings.Contains(out.String(), "PACKAGE") || !strings.Contains(out.String(), "push failed: boom") {
		t.Fatalf("the summary table belongs on stdout:\n%s", out.String())
	}
	if strings.Contains(out.String(), "built (") || strings.Contains(out.String(), "hint:") {
		t.Fatalf("stdout must carry the table and nothing else:\n%s", out.String())
	}
	if !strings.Contains(errb.String(), "WARN 2 built (1 pushed), 0 failed, 0 skipped") {
		t.Fatalf("a push failure must warn, not succeed:\n%s", errb.String())
	}
	if !strings.Contains(errb.String(), "hint: Re-run with --missing to retry failed pushes") {
		t.Fatalf("stderr must carry the converge hint:\n%s", errb.String())
	}
	if strings.Contains(errb.String(), "PACKAGE") {
		t.Fatalf("the table must not be duplicated on stderr:\n%s", errb.String())
	}
}

func TestCatalogBuildWithDepsComposesWithMissing(t *testing.T) {
	nemHome := t.TempDir()
	dir := writeLintFixture(t, map[string]string{
		"blib": batchRecipe("blib", "https://example.com/blib.tar.gz", "", "make"),
		"bapp": batchRecipe("bapp", "https://example.com/bapp.tar.gz",
			"  deps: [{name: blib}]\n", "make"),
		"bsolo": batchRecipe("bsolo", "https://example.com/bsolo.tar.gz", "", "make"),
	})

	out, errb, err := runNem(t, nemHome, "catalog", "build", dir,
		"--package", "bapp@v1.0.0", "--with-deps", "--missing", "--dry-run")
	if err != nil {
		t.Fatalf("--with-deps must compose with --missing: %v\nstderr: %s", err, errb)
	}
	if row := tableRow(out, 1, "bapp"); row == nil ||
		!strings.Contains(strings.Join(row, " "), "forced") {
		t.Fatalf("bapp row = %v, want the forced selection to win over the missing scan:\n%s", row, out)
	}
	if row := tableRow(out, 1, "blib"); row == nil ||
		!strings.Contains(strings.Join(row, " "), "dep of bapp") {
		t.Fatalf("blib row = %v, want the dep selection to win over the missing scan:\n%s", row, out)
	}
	if row := tableRow(out, 1, "bsolo"); row == nil ||
		!strings.Contains(strings.Join(row, " "), "missing archive") {
		t.Fatalf("bsolo row = %v, want the missing scan to still contribute:\n%s", row, out)
	}
	if !strings.Contains(errb, "Checked 3 oci packages:") {
		t.Fatalf("the missing scan must narrate what it checked:\n%s", errb)
	}
}

func TestCatalogBuildPushStagesIntoTheDirectoryTarget(t *testing.T) {
	nemHome := t.TempDir()
	srv := helloTarball(t)
	defer srv.Close()

	markers := t.TempDir()
	dir := writeLintFixture(t, map[string]string{
		"alpha": batchRecipe("alpha", srv.URL, "", markerStep(markers, "alpha")),
	})

	out, errb, err := runNem(t, nemHome, "catalog", "build", dir,
		"--package", "alpha@v1.0.0", "--push")
	if err != nil {
		t.Fatalf("catalog build --push: %v\nstderr: %s", err, errb)
	}
	if row := tableRow(out, 0, "alpha"); row == nil || row[2] != "pushed" {
		t.Fatalf("summary row for alpha = %v, want a pushed row:\n%s", row, out)
	}
	if !ocix.NewArchiveStore(dir).HasTag("alpha", "v1.0.0") {
		t.Fatalf("alpha@v1.0.0 is not staged in %s", filepath.Join(dir, "archives", "alpha"))
	}
}

func TestCatalogBuildBatchSkipsDependentsOfFailedPackages(t *testing.T) {
	nemHome := t.TempDir()
	srv := helloTarball(t)
	defer srv.Close()

	dir := writeLintFixture(t, map[string]string{
		"dfail": batchRecipe("dfail", srv.URL, "", "exit 1"),
		"dapp": batchRecipe("dapp", srv.URL, "  deps: [{name: dfail}]\n",
			`mkdir -p "$NEM_OUTPUT" && echo x > "$NEM_OUTPUT/marker"`),
	})

	out, errb, err := runNem(t, nemHome, "catalog", "build", dir,
		"--package", "dfail@v1.0.0", "--package", "dapp@v1.0.0")
	if err == nil {
		t.Fatalf("a failed package must fail the batch\nstdout: %s\nstderr: %s", out, errb)
	}
	if row := tableRow(out, 0, "dfail"); row == nil || row[2] != "failed" {
		t.Fatalf("dfail row = %v, want failed:\n%s", row, out)
	}
	row := tableRow(out, 0, "dapp")
	if row == nil || row[2] != "skipped" {
		t.Fatalf("dapp row = %v, want skipped:\n%s", row, out)
	}
	if !strings.Contains(strings.Join(row, " "), "needs dfail") {
		t.Fatalf("the skipped row must name the failed dep, got %v", row)
	}
	if !strings.Contains(err.Error(), "failed or were skipped") {
		t.Fatalf("error = %v, want the failed-or-skipped count", err)
	}
}

func TestCatalogBuildBatchResolvesDepsFromTheTargetCatalog(t *testing.T) {
	nemHome := t.TempDir()
	srv := helloTarball(t)
	defer srv.Close()

	markers := t.TempDir()
	dir := writeLintFixture(t, map[string]string{
		"ilib": batchRecipe("ilib", srv.URL, "", markerStep(markers, "ilib")),
		"iapp": batchRecipe("iapp", srv.URL, "  deps: [{name: ilib}]\n", markerStep(markers, "iapp")),
	})

	out, errb, err := runNem(t, nemHome, "catalog", "build", dir,
		"--package", "ilib@v1.0.0", "--package", "iapp@v1.0.0")
	if err != nil {
		t.Fatalf("catalog build of two selections: %v\nstdout: %s\nstderr: %s", err, out, errb)
	}
	for _, name := range []string{"ilib", "iapp"} {
		if _, err := os.Stat(filepath.Join(markers, name)); err != nil {
			t.Fatalf("%s never built: %v\nstderr: %s", name, err, errb)
		}
		if row := tableRow(out, 0, name); row == nil || row[2] != "built" {
			t.Fatalf("summary row for %s = %v, want a built row:\n%s", name, row, out)
		}
	}
	if !strings.Contains(errb, "2 built (0 pushed), 0 failed, 0 skipped") {
		t.Fatalf("stderr must close with the verdict line:\n%s", errb)
	}
}

func TestCatalogBuildRejectsAnEmptySelectionBeforeOpeningTheTarget(t *testing.T) {
	nemHome := t.TempDir()
	_, _, err := runNem(t, nemHome, "catalog", "build", "ghcr.io/org/cat:v2")
	if err == nil || !strings.Contains(err.Error(), "select packages with --missing or --package") {
		t.Fatalf("err = %v, want the no-selection rejection", err)
	}
}

func TestCatalogBuildFlagValidation(t *testing.T) {
	nemHome := t.TempDir()
	dir := writeLintFixture(t, map[string]string{
		"blib": batchRecipe("blib", "https://example.com/blib.tar.gz", "", "make"),
	})
	recipe := filepath.Join(dir, "pkgs", "blib", "pkg.yaml")

	t.Run("no selection", func(t *testing.T) {
		_, _, err := runNem(t, nemHome, "catalog", "build", dir)
		if err == nil || !strings.Contains(err.Error(), "select packages with --missing or --package") {
			t.Fatalf("err = %v, want the no-selection rejection", err)
		}
	})
	t.Run("with-deps without a root", func(t *testing.T) {
		_, _, err := runNem(t, nemHome, "catalog", "build", dir, "--with-deps", "--missing")
		if err == nil || !strings.Contains(err.Error(), "--with-deps needs at least one --package") {
			t.Fatalf("err = %v, want the with-deps rejection", err)
		}
	})
	t.Run("two positionals", func(t *testing.T) {
		_, _, err := runNem(t, nemHome, "catalog", "build", dir, dir, "--missing")
		if err == nil || !strings.Contains(err.Error(), "accepts 1 arg") {
			t.Fatalf("err = %v, want exactly one catalog positional", err)
		}
	})
	t.Run("two versions of one package", func(t *testing.T) {
		out, errb, err := runNem(t, nemHome, "catalog", "build", dir,
			"--package", "blib@v1.0.0", "--package", "blib@v2.0.0", "--dry-run")
		if err != nil {
			t.Fatalf("two versions of one package must plan: %v\nstderr: %s", err, errb)
		}
		if got := planVersions(out, "blib"); len(got) != 2 ||
			got[0] != "v1.0.0" || got[1] != "v2.0.0" {
			t.Fatalf("blib plan versions = %v, want both selections:\n%s", got, out)
		}
	})
	t.Run("same version of one package twice", func(t *testing.T) {
		_, _, err := runNem(t, nemHome, "catalog", "build", dir,
			"--package", "blib@v1.0.0", "--package", "blib@v1.0.0", "--dry-run")
		if err == nil || !strings.Contains(err.Error(), "duplicate selection blib@v1.0.0") {
			t.Fatalf("err = %v, want the exact-duplicate rejection", err)
		}
	})
	t.Run("recipe path is not a catalog ref", func(t *testing.T) {
		_, errb, err := runNem(t, nemHome, "catalog", "build", recipe, "--package", "blib@v1.0.0")
		if err == nil {
			t.Fatal("a pkg.yaml path is not a catalog ref any more")
		}
		if !strings.Contains(errb, "catalog directory or OCI ref") {
			t.Fatalf("the hint must redirect a recipe path to a catalog ref:\n%s", errb)
		}
	})
}
