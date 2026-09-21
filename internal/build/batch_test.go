package build

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"

	"github.com/vi-dev/nem/internal/archive"
	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/home"
	"github.com/vi-dev/nem/internal/report"
	"github.com/vi-dev/nem/internal/spec"
	"github.com/vi-dev/nem/internal/testx"
)

type batchSpec struct {
	name    string
	version string
	step    string
	deps    []string
	plats   []spec.Platform
}

func (s batchSpec) platforms() []spec.Platform {
	if len(s.plats) > 0 {
		return s.plats
	}
	return []spec.Platform{spec.Current()}
}

func batchFixture(t *testing.T, specs ...batchSpec) (*catalog.Set, map[string]*spec.Package) {
	t.Helper()
	srv, _ := serve(t, makeTarGz(t, map[string]string{"src/README": "hi"}))

	var names []string
	groups := map[string][]batchSpec{}
	versions := map[string]string{}
	for _, s := range specs {
		if _, seen := groups[s.name]; !seen {
			names = append(names, s.name)
			versions[s.name] = s.version
		}
		groups[s.name] = append(groups[s.name], s)
	}

	root := t.TempDir()
	for _, name := range names {
		group := groups[name]
		dir := filepath.Join(root, "pkgs", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		var b strings.Builder
		fmt.Fprintf(&b, "schema: 2\nname: %s\n", name)
		b.WriteString("artifact:\n  oci: \":{{.Version}}\"\n")
		b.WriteString("install:\n  - extract: {}\n")
		b.WriteString("versions:\n")
		for _, s := range group {
			fmt.Fprintf(&b, "  - version: %q\n", s.version)
		}
		fmt.Fprintf(&b, "build:\n  source:\n    url: %q\n  output: out\n", srv.URL)
		if len(group[0].deps) > 0 {
			b.WriteString("  deps:\n")
			for _, d := range group[0].deps {
				fmt.Fprintf(&b, "    - name: %s\n      version: %q\n", d, versions[d])
			}
		}
		b.WriteString("  steps:\n    - run: |\n")
		for _, line := range strings.Split(strings.TrimRight(batchStep(group), "\n"), "\n") {
			fmt.Fprintf(&b, "        %s\n", line)
		}
		if err := os.WriteFile(filepath.Join(dir, "pkg.yaml"), []byte(b.String()), 0o644); err != nil {
			t.Fatalf("write %s pkg.yaml: %v", name, err)
		}
	}

	src := catalog.NewDir(root)
	pkgs := map[string]*spec.Package{}
	for _, name := range names {
		p, _, err := src.Package(context.Background(), name)
		if err != nil {
			t.Fatalf("load %s: %v", name, err)
		}
		pkgs[name] = p
	}
	return catalog.NewSet(catalog.Entry{Name: "cat", Catalog: src}), pkgs
}

func batchStep(group []batchSpec) string {
	if len(group) == 1 {
		return group[0].step
	}
	var b strings.Builder
	b.WriteString("case \"$NEM_VERSION\" in\n")
	for _, s := range group {
		fmt.Fprintf(&b, "%q)\n%s\n;;\n", s.version, strings.TrimRight(s.step, "\n"))
	}
	b.WriteString("esac\n")
	return b.String()
}

func batchPlan(t *testing.T, pkgs map[string]*spec.Package, specs ...batchSpec) Plan {
	t.Helper()
	sels := make([]Selection, 0, len(specs))
	for _, s := range specs {
		sels = append(sels, Selection{
			Pkg: pkgs[s.name], Version: s.version, Reason: "forced", Platforms: s.platforms(),
		})
	}
	plan, err := ComputePlan(sels)
	if err != nil {
		t.Fatalf("ComputePlan: %v", err)
	}
	return plan
}

func runBatch(t *testing.T, h home.Home, set *catalog.Set, plan Plan, opts BatchOptions) ([]SummaryRow, string) {
	t.Helper()
	var b bytes.Buffer
	ctx := report.NewContext(context.Background(), report.New(&b, &b, report.Options{}))
	rows, err := RunBatch(ctx, h, set, plan, opts)
	if err != nil {
		t.Fatalf("RunBatch: %v\n%s", err, b.String())
	}
	return rows, b.String()
}

type funcStore struct {
	openRW func(name string) (oras.Target, error)
}

func (s funcStore) Open(_ context.Context, name string) (oras.ReadOnlyTarget, error) {
	return s.openRW(name)
}
func (s funcStore) OpenRW(_ context.Context, name string) (oras.Target, error) {
	return s.openRW(name)
}

func ociTarget(ref string) *Target { return &Target{Ref: ref} }

func dirBatchTarget(t *testing.T) *Target {
	t.Helper()
	dir := t.TempDir()
	d := archive.NewDir(dir)
	return &Target{
		Ref:     dir,
		Dir:     dir,
		Entry:   catalog.Entry{Name: dir, Catalog: catalog.NewDir(dir), Archives: d},
		Overlay: d,
		Store:   d,
	}
}

func batchStoreDirs(t *testing.T, h home.Home) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(h.Tmp(), "batch"+home.BuildStagingInfix+"*"))
	if err != nil {
		t.Fatalf("glob batch stores: %v", err)
	}
	return matches
}

func rowStrings(rows []SummaryRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, fmt.Sprintf("%s@%s %s %s", r.Name, r.Version, r.Result, r.Detail))
	}
	return out
}

func assertRows(t *testing.T, rows []SummaryRow, want []string, out string) {
	t.Helper()
	got := rowStrings(rows)
	if len(got) != len(want) {
		t.Fatalf("rows = %v, want %v\n%s", got, want, out)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("row %d = %q, want %q (all: %v)\n%s", i, got[i], want[i], got, out)
		}
	}
}

func TestRunBatchHandsDepArchiveToDependentThroughLocalStore(t *testing.T) {
	h := testx.HomeAt(t.TempDir())

	lib := batchSpec{name: "lib", version: "1.0.0",
		step: `mkdir -p "$NEM_OUTPUT/lib"` + "\n" + `echo marker > "$NEM_OUTPUT/lib/marker"`}
	app := batchSpec{name: "app", version: "2.0.0", deps: []string{"lib"},
		step: `test -f "$NEM_DEP_LIB_PREFIX/lib/marker" || exit 9` + "\n" +
			`mkdir -p "$NEM_OUTPUT/bin"` + "\n" + `echo app > "$NEM_OUTPUT/bin/app"`}

	elsewhere := batchSpec{name: "elsewhere", version: "3.0.0", plats: []spec.Platform{otherPlatform()},
		step: `mkdir -p "$NEM_OUTPUT" && exit 5`}

	sources, pkgs := batchFixture(t, lib, app, elsewhere)
	plan := batchPlan(t, pkgs, lib, app, elsewhere)

	var tested []string
	rows, out := runBatch(t, h, sources, plan, BatchOptions{
		TestFor: func(p *spec.Package, store *archive.Dir) func(context.Context, *spec.Package, string, string) error {
			if store == nil {
				t.Error("TestFor must receive the batch's local store")
			}
			return func(_ context.Context, tp *spec.Package, version, artifactPath string) error {
				tested = append(tested, tp.Name+"@"+version)
				if _, err := os.Stat(artifactPath); err != nil {
					return err
				}
				return nil
			}
		},
	})

	assertRows(t, rows, []string{"lib@1.0.0 built -", "app@2.0.0 built -"}, out)

	dir, err := h.PackageDir("lib", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "lib", "marker")); string(got) != "marker\n" {
		t.Fatalf("lib was not installed from the local store; marker = %q\n%s", got, out)
	}

	if len(tested) != 2 || tested[0] != "lib@1.0.0" || tested[1] != "app@2.0.0" {
		t.Fatalf("test hooks ran for %v, want both packages in wave order", tested)
	}
}

func TestRunBatchSkipsDependentsOfAFailedBuild(t *testing.T) {
	h := testx.HomeAt(t.TempDir())

	lib := batchSpec{name: "lib", version: "1.0.0", step: `mkdir -p "$NEM_OUTPUT" && exit 1`}
	app := batchSpec{name: "app", version: "2.0.0", deps: []string{"lib"},
		step: `mkdir -p "$NEM_OUTPUT/bin" && echo app > "$NEM_OUTPUT/bin/app"`}
	web := batchSpec{name: "web", version: "3.0.0", deps: []string{"app"},
		step: `mkdir -p "$NEM_OUTPUT/bin" && echo web > "$NEM_OUTPUT/bin/web"`}

	sources, pkgs := batchFixture(t, lib, app, web)
	rows, out := runBatch(t, h, sources, batchPlan(t, pkgs, lib, app, web), BatchOptions{})

	if len(rows) != 3 {
		t.Fatalf("rows = %v, want three", rowStrings(rows))
	}
	if rows[0].Name != "lib" || rows[0].Result != "failed" || rows[0].Detail == "" {
		t.Fatalf("row 0 = %+v, want a failed lib row carrying the error", rows[0])
	}
	if strings.Contains(rows[0].Detail, "\n") {
		t.Fatalf("failure detail must be a single line, got %q", rows[0].Detail)
	}
	if got := rowStrings(rows)[1]; got != "app@2.0.0 skipped needs lib (all versions failed)" {
		t.Fatalf("row 1 = %q, want app skipped naming lib\n%s", got, out)
	}

	if got := rowStrings(rows)[2]; got != "web@3.0.0 skipped needs app (all versions failed)" {
		t.Fatalf("row 2 = %q, want web skipped naming app\n%s", got, out)
	}
	if !strings.Contains(out, "Skipping app@2.0.0") {
		t.Fatalf("skip was not narrated:\n%s", out)
	}
	if strings.Contains(out, "building app@") || strings.Contains(out, "building web@") {
		t.Fatalf("skipped entries must not be built:\n%s", out)
	}
}

func TestRunBatchBuildsAllVersionsOfAPackage(t *testing.T) {
	h := testx.HomeAt(t.TempDir())

	step := func(v string) string {
		return `mkdir -p "$NEM_OUTPUT/bin"` + "\n" + `echo ` + v + ` > "$NEM_OUTPUT/bin/tool"`
	}
	newer := batchSpec{name: "dual", version: "2.0.0", step: step("two")}
	older := batchSpec{name: "dual", version: "1.0.0", step: step("one")}

	sources, pkgs := batchFixture(t, newer, older)
	rows, out := runBatch(t, h, sources, batchPlan(t, pkgs, newer, older), BatchOptions{})

	assertRows(t, rows, []string{"dual@2.0.0 built -", "dual@1.0.0 built -"}, out)
	for i, want := range []string{
		"[1/2] wave 1: building dual@2.0.0",
		"[2/2] wave 1: building dual@1.0.0",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("narration %d: want %q newest-first in:\n%s", i, want, out)
		}
	}
}

func TestRunBatchSkipsOnlyWhenAllVersionsFail(t *testing.T) {
	ok := `mkdir -p "$NEM_OUTPUT/bin"` + "\n" + `echo hi > "$NEM_OUTPUT/bin/tool"`
	fails := `mkdir -p "$NEM_OUTPUT" && exit 1`

	app := batchSpec{name: "app", version: "3.0.0", deps: []string{"lib"},
		step: `test -f "$NEM_DEP_LIB_PREFIX/bin/tool" || exit 9` + "\n" + ok}

	t.Run("one version survives", func(t *testing.T) {
		h := testx.HomeAt(t.TempDir())
		good := batchSpec{name: "lib", version: "2.0.0", step: ok}
		bad := batchSpec{name: "lib", version: "1.0.0", step: fails}

		sources, pkgs := batchFixture(t, good, bad, app)
		rows, out := runBatch(t, h, sources, batchPlan(t, pkgs, good, bad, app), BatchOptions{})

		if len(rows) != 3 {
			t.Fatalf("rows = %v, want three", rowStrings(rows))
		}
		if got := rowStrings(rows)[0]; got != "lib@2.0.0 built -" {
			t.Fatalf("row 0 = %q, want the newest lib built\n%s", got, out)
		}
		if rows[1].Name != "lib" || rows[1].Version != "1.0.0" || rows[1].Result != "failed" {
			t.Fatalf("row 1 = %+v, want lib@1.0.0 failed", rows[1])
		}
		if got := rowStrings(rows)[2]; got != "app@3.0.0 built -" {
			t.Fatalf("row 2 = %q: one surviving version of a dep must keep its "+
				"dependents in the batch\n%s", got, out)
		}
	})

	t.Run("every version fails", func(t *testing.T) {
		h := testx.HomeAt(t.TempDir())
		newer := batchSpec{name: "lib", version: "2.0.0", step: fails}
		older := batchSpec{name: "lib", version: "1.0.0", step: fails}

		sources, pkgs := batchFixture(t, newer, older, app)
		rows, out := runBatch(t, h, sources, batchPlan(t, pkgs, newer, older, app), BatchOptions{})

		if len(rows) != 3 {
			t.Fatalf("rows = %v, want three", rowStrings(rows))
		}
		if rows[0].Result != "failed" || rows[1].Result != "failed" {
			t.Fatalf("both lib rows must fail: %v", rowStrings(rows))
		}
		if got := rowStrings(rows)[2]; got != "app@3.0.0 skipped needs lib (all versions failed)" {
			t.Fatalf("row 2 = %q, want app skipped naming lib\n%s", got, out)
		}
		if !strings.Contains(out, "Skipping app@3.0.0: dependency lib failed (all versions)") {
			t.Fatalf("the skip must say every version of the dep failed:\n%s", out)
		}
		if strings.Contains(out, "building app@") {
			t.Fatalf("a skipped entry must not be built:\n%s", out)
		}
	})
}

func TestRunBatchPushesEachSuccessAndIsolatesPushFailures(t *testing.T) {
	h := testx.HomeAt(t.TempDir())

	step := `mkdir -p "$NEM_OUTPUT/bin"` + "\n" + `echo hi > "$NEM_OUTPUT/bin/tool"`
	a := batchSpec{name: "alpha", version: "1.0.0", step: step}
	b := batchSpec{name: "bravo", version: "1.0.0", step: step}

	c := batchSpec{name: "charlie", version: "1.0.0", deps: []string{"bravo"},
		step: `test -f "$NEM_DEP_BRAVO_PREFIX/bin/tool" || exit 9` + "\n" + step}
	sources, pkgs := batchFixture(t, a, b, c)

	fixtures := testx.NewArchiveFixtures()
	fixtures.Set("bravo", &testx.RejectPushTarget{Target: memory.New()})
	target := ociTarget("ghcr.io/x/cat:v2")
	target.Store = funcStore{openRW: func(name string) (oras.Target, error) {
		return fixtures.Open(name), nil
	}}

	rows, out := runBatch(t, h, sources, batchPlan(t, pkgs, a, b, c),
		BatchOptions{Target: target, Push: true})

	if len(rows) != 3 {
		t.Fatalf("rows = %v, want three", rowStrings(rows))
	}
	if got := rowStrings(rows)[0]; got != "alpha@1.0.0 pushed -" {
		t.Fatalf("row 0 = %q, want alpha pushed\n%s", got, out)
	}
	if rows[1].Result != "built" || !strings.HasPrefix(rows[1].Detail, "push failed: ") || !rows[1].PushFailed {
		t.Fatalf("row 1 = %+v, want a built row with a push-failed detail and PushFailed set\n%s", rows[1], out)
	}
	if got := rowStrings(rows)[2]; got != "charlie@1.0.0 pushed -" {
		t.Fatalf("row 2 = %q: a dependent of an unpushed-but-built package must "+
			"still build from the local store\n%s", got, out)
	}

	for _, name := range []string{"alpha", "charlie"} {
		if !strings.Contains(out, "Pushed "+name+" 1.0.0") {
			t.Fatalf("expected narration of %s's successful push\n%s", name, out)
		}
	}
	if strings.Contains(out, "Pushed bravo") {
		t.Fatalf("bravo's push failed and must not be narrated as pushed\n%s", out)
	}

	for _, name := range []string{"alpha", "charlie"} {
		if _, err := archive.Pull(context.Background(), fixtures.Open(name),
			"1.0.0", spec.Current(), t.TempDir()); err != nil {
			t.Fatalf("%s's archive is not in the registry: %v", name, err)
		}
	}
}

func TestRunBatchReportsAnUnchangedPushAsPushed(t *testing.T) {
	h := testx.HomeAt(t.TempDir())

	a := batchSpec{name: "alpha", version: "1.0.0",
		step: `mkdir -p "$NEM_OUTPUT/bin"` + "\n" + `echo hi > "$NEM_OUTPUT/bin/tool"`}
	sources, pkgs := batchFixture(t, a)

	store := memory.New()
	target := ociTarget("ghcr.io/x/cat:v2")
	target.Store = funcStore{openRW: func(string) (oras.Target, error) { return store, nil }}
	plan := batchPlan(t, pkgs, a)

	opts := BatchOptions{Target: target, Push: true}
	firstRows, firstOut := runBatch(t, h, sources, plan, opts)
	if firstOut == "" {
		t.Fatal("expected narration from the first run")
	}
	assertRows(t, firstRows, []string{"alpha@1.0.0 pushed -"}, firstOut)
	if !strings.Contains(firstOut, "Pushed alpha 1.0.0") {
		t.Fatalf("expected narration of alpha's successful push\n%s", firstOut)
	}

	rows, out := runBatch(t, h, sources, plan, opts)

	assertRows(t, rows, []string{"alpha@1.0.0 pushed unchanged"}, out)
	if strings.Contains(out, "Pushed") {
		t.Fatalf("an unchanged push must stay silent\n%s", out)
	}
}

func TestRunBatchDirPushStagesIntoOverlay(t *testing.T) {
	h := testx.HomeAt(t.TempDir())

	step := `mkdir -p "$NEM_OUTPUT/bin"` + "\n" + `echo hi > "$NEM_OUTPUT/bin/tool"`
	a := batchSpec{name: "alpha", version: "1.0.0", step: step}
	b := batchSpec{name: "bravo", version: "2.0.0", deps: []string{"alpha"},
		step: `test -f "$NEM_DEP_ALPHA_PREFIX/bin/tool" || exit 9` + "\n" + step}
	sources, pkgs := batchFixture(t, a, b)

	t.Cleanup(archive.SetRepoOpener(func(ref string) (oras.Target, error) {
		t.Errorf("a dir target must not open registry archives (%s)", ref)
		return memory.New(), nil
	}))

	target := dirBatchTarget(t)
	rows, out := runBatch(t, h, sources, batchPlan(t, pkgs, a, b), BatchOptions{
		Target: target, Push: true,
		TestFor: func(_ *spec.Package, store *archive.Dir) func(context.Context, *spec.Package, string, string) error {
			if store != target.Overlay {
				t.Error("builds must stage into the target overlay store")
			}
			return nil
		},
	})

	assertRows(t, rows, []string{"alpha@1.0.0 pushed -", "bravo@2.0.0 pushed -"}, out)

	for _, s := range []batchSpec{a, b} {
		if !strings.Contains(out, "Pushed "+s.name+" "+s.version) {
			t.Fatalf("expected narration of %s's dir-overlay push\n%s", s.name, out)
		}
	}

	for _, s := range []batchSpec{a, b} {
		if !target.Overlay.HasVersion(s.name, s.version) {
			t.Fatalf("%s@%s is not staged in the target overlay %s\n%s",
				s.name, s.version, target.Dir, out)
		}
		index := filepath.Join(target.Dir, "archives", s.name, "index.json")
		if _, err := os.Stat(index); err != nil {
			t.Fatalf("expected a layout at %s: %v", index, err)
		}
	}

	if got := batchStoreDirs(t, h); len(got) > 0 {
		t.Fatalf("dir push must not create a tmp batch store, found %v", got)
	}
}

func TestRunBatchDirPushStagesNothingWhenTestsFail(t *testing.T) {
	h := testx.HomeAt(t.TempDir())

	step := `mkdir -p "$NEM_OUTPUT/bin"` + "\n" + `echo hi > "$NEM_OUTPUT/bin/tool"`
	good := batchSpec{name: "alpha", version: "1.0.0", step: step}
	bad := batchSpec{name: "bravo", version: "1.0.0", step: step}
	sources, pkgs := batchFixture(t, good, bad)

	target := dirBatchTarget(t)
	rows, out := runBatch(t, h, sources, batchPlan(t, pkgs, good, bad), BatchOptions{
		Target: target, Push: true,
		TestFor: func(pkg *spec.Package, _ *archive.Dir) func(context.Context, *spec.Package, string, string) error {
			if pkg.Name != "bravo" {
				return nil
			}
			return func(context.Context, *spec.Package, string, string) error {
				return errors.New("smoke test exploded")
			}
		},
	})

	if len(rows) != 2 {
		t.Fatalf("rows = %v, want two", rowStrings(rows))
	}
	if got := rowStrings(rows)[0]; got != "alpha@1.0.0 pushed -" {
		t.Fatalf("row 0 = %q, want the tested package published\n%s", got, out)
	}
	if rows[1].Result != "failed" || !strings.Contains(rows[1].Detail, "smoke test exploded") {
		t.Fatalf("row 1 = %+v, want a failed bravo row naming the test error\n%s", rows[1], out)
	}

	if target.Overlay.HasVersion("bravo", "1.0.0") || target.Overlay.Has("bravo") {
		t.Fatalf("bravo@1.0.0 was published despite failing its tests (%s)", target.Dir)
	}
	if !target.Overlay.HasVersion("alpha", "1.0.0") {
		t.Fatalf("alpha@1.0.0 passed its tests and must be published (%s)\n%s", target.Dir, out)
	}
}

func TestRunBatchNoPushStillUsesTmpStore(t *testing.T) {
	h := testx.HomeAt(t.TempDir())

	a := batchSpec{name: "alpha", version: "1.0.0",
		step: `mkdir -p "$NEM_OUTPUT/bin"` + "\n" + `echo hi > "$NEM_OUTPUT/bin/tool"`}
	sources, pkgs := batchFixture(t, a)

	target := dirBatchTarget(t)
	var duringWasOverlay bool
	var duringStores []string
	rows, out := runBatch(t, h, sources, batchPlan(t, pkgs, a), BatchOptions{
		Target: target,
		TestFor: func(_ *spec.Package, store *archive.Dir) func(context.Context, *spec.Package, string, string) error {
			duringWasOverlay = store == target.Overlay
			duringStores = batchStoreDirs(t, h)
			return nil
		},
	})

	assertRows(t, rows, []string{"alpha@1.0.0 built -"}, out)
	if strings.Contains(out, "Pushed") {
		t.Fatalf("no push happened, so nothing should be narrated as pushed\n%s", out)
	}

	if duringWasOverlay {
		t.Fatalf("builds staged into the target overlay %s without --push", target.Dir)
	}
	if len(duringStores) != 1 {
		t.Fatalf("batch stores during the run = %v, want exactly one under %s", duringStores, h.Tmp())
	}
	if got := batchStoreDirs(t, h); len(got) > 0 {
		t.Fatalf("the tmp batch store was not swept: %v", got)
	}
	if target.Overlay.Has("alpha") {
		t.Fatalf("the target overlay must be untouched without --push")
	}
	if _, err := os.Stat(filepath.Join(target.Dir, "archives")); !os.IsNotExist(err) {
		t.Fatalf("stat %s/archives = %v, want it never created", target.Dir, err)
	}
}

func TestVerdictCountsRowsAndDecidesTheExitCode(t *testing.T) {
	tests := []struct {
		name                                     string
		rows                                     []SummaryRow
		built, pushed, failed, skipped, unpushed int
		err                                      string
	}{
		{
			name: "every build lands",
			rows: []SummaryRow{
				{Result: "pushed"},
				{Result: "built"},
			},
			built: 2, pushed: 1,
		},
		{
			name: "failed and skipped rows sink the batch",
			rows: []SummaryRow{
				{Result: "built"},
				{Result: "failed"},
				{Result: "skipped"},
			},
			built: 1, failed: 1, skipped: 1,
			err: "2 of 3 builds failed or were skipped",
		},
		{
			name: "a push failure alone still sinks the batch",
			rows: []SummaryRow{
				{Result: "pushed"},
				{Result: "built", Detail: "push failed: boom", PushFailed: true},
			},
			built: 2, pushed: 1, unpushed: 1,
			err: "1 of 2 builds failed to push",
		},
		{
			name: "a build failure outranks a push failure",
			rows: []SummaryRow{
				{Result: "failed"},
				{Result: "built", Detail: "push failed: boom", PushFailed: true},
			},
			built: 1, failed: 1, unpushed: 1,
			err: "1 of 2 builds failed or were skipped",
		},
		{
			name: "the detail prefix alone is not a push failure",
			rows: []SummaryRow{
				{Result: "built", Detail: "push failed: boom"},
			},
			built: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			built, pushed, failed, skipped, unpushed, err := Verdict(tc.rows)
			got := []int{built, pushed, failed, skipped, unpushed}
			want := []int{tc.built, tc.pushed, tc.failed, tc.skipped, tc.unpushed}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("counts = %v, want %v", got, want)
				}
			}
			switch {
			case tc.err == "" && err != nil:
				t.Fatalf("err = %v, want nil", err)
			case tc.err != "" && (err == nil || err.Error() != tc.err):
				t.Fatalf("err = %v, want %q", err, tc.err)
			}
		})
	}
}
