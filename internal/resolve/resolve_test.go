package resolve_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/project"
	"github.com/vi-dev/nem/internal/resolve"
	"github.com/vi-dev/nem/internal/spec"
)

func writePkg(t *testing.T, root, yaml string) {
	t.Helper()
	pkg, err := spec.Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	dir := filepath.Join(root, "pkgs", pkg.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatalf("write pkg.yaml: %v", err)
	}
}

type pkgSpec struct {
	name      string
	platforms string
	deps      []string
	libs      string
	versions  string
}

func writeSpec(t *testing.T, root string, p pkgSpec) {
	t.Helper()
	var b strings.Builder
	b.WriteString("schema: 2\nname: " + p.name + "\n")
	if p.platforms != "" {
		b.WriteString("platforms: " + p.platforms + "\n")
	}
	if len(p.deps) > 0 {
		b.WriteString("deps: [" + strings.Join(p.deps, ", ") + "]\n")
	}
	if p.libs != "" {
		b.WriteString("libs: " + p.libs + "\n")
	}
	b.WriteString("artifact: {oci: \":{{.Version}}\"}\ninstall: [{extract: {}}]\nversions: " + p.versions + "\n")
	writePkg(t, root, b.String())
}

func namedSources(root string) []catalog.Named {
	return []catalog.Named{{Name: "cat", Source: catalog.NewDir(root)}}
}

func entry(t *testing.T, res *resolve.Result, name string) project.LockEntry {
	t.Helper()
	for _, e := range res.Entries {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("no entry %q in %+v", name, res.Entries)
	return project.LockEntry{}
}

func bothOrders(t *testing.T, firstName, secondName string, first, second resolve.Tool, fn func(t *testing.T, tools []resolve.Tool)) {
	t.Helper()
	orders := map[string][]resolve.Tool{
		firstName:  {first, second},
		secondName: {second, first},
	}
	for name, tools := range orders {
		t.Run(name, func(t *testing.T) { fn(t, tools) })
	}
}

func TestResolveSingleToolLatest(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, pkgSpec{name: "a", versions: "[v2.0.0, v1.0.0]"})
	tools := []resolve.Tool{{Key: project.ToolKey{Name: "a"}}}
	res, err := resolve.Resolve(context.Background(), tools, namedSources(root))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(res.Entries) != 1 {
		t.Fatalf("want 1 entry, got %+v", res.Entries)
	}
	e := res.Entries[0]
	if e.Name != "a" || e.Version != "v2.0.0" || e.Catalog != "cat" || !e.Direct || e.Digest != "" {
		t.Fatalf("entry: %+v", e)
	}
	if len(e.Platforms) != 4 {
		t.Fatalf("platforms: %v", e.Platforms)
	}
	if res.Pkgs["a"] == nil || res.Pkgs["a"].Name != "a" {
		t.Fatalf("Pkgs[a]: %+v", res.Pkgs["a"])
	}
}

func TestResolveRangePicksHighestMatch(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, pkgSpec{name: "lib", libs: "[lib]", versions: "[v2.0.0, v1.8.0, v1.9.0]"})
	writeSpec(t, root, pkgSpec{name: "app", deps: []string{`{name: lib, kind: link, compat: "1"}`}, versions: "[v1.0.0]"})
	tools := []resolve.Tool{{Key: project.ToolKey{Name: "app"}}}
	res, err := resolve.Resolve(context.Background(), tools, namedSources(root))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if e := entry(t, res, "lib"); e.Version != "v1.9.0" {
		t.Fatalf("the range must name its highest match: %+v", e)
	}
}

func TestResolveVersionlessKeepsListOrderLatest(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, pkgSpec{name: "a", versions: "[v1.2.3, v1.3.0-rc1]"})
	tools := []resolve.Tool{{Key: project.ToolKey{Name: "a"}}}
	res, err := resolve.Resolve(context.Background(), tools, namedSources(root))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if e := entry(t, res, "a"); e.Version != "v1.2.3" {
		t.Fatalf("latest must be the list's top satisfying entry: %+v", e)
	}
}

func TestResolveDepChain(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, pkgSpec{name: "a", deps: []string{"{name: b}"}, versions: "[v1.0.0]"})
	writeSpec(t, root, pkgSpec{name: "b", versions: "[v1.0.0]"})
	tools := []resolve.Tool{{Key: project.ToolKey{Name: "a"}}}
	res, err := resolve.Resolve(context.Background(), tools, namedSources(root))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(res.Entries) != 2 {
		t.Fatalf("want 2 entries, got %+v", res.Entries)
	}
	a := entry(t, res, "a")
	b := entry(t, res, "b")
	if !a.Direct {
		t.Fatalf("a should be direct: %+v", a)
	}
	if b.Direct {
		t.Fatalf("b should be indirect: %+v", b)
	}
	if len(b.Platforms) != 4 {
		t.Fatalf("b platforms should be the union (all 4): %v", b.Platforms)
	}
}

func TestResolvePlatformConstrainedDep(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, pkgSpec{name: "a", deps: []string{"{name: b, platforms: [linux]}"}, versions: "[v1.0.0]"})
	writeSpec(t, root, pkgSpec{name: "b", versions: "[v1.0.0]"})
	tools := []resolve.Tool{{Key: project.ToolKey{Name: "a"}}}
	res, err := resolve.Resolve(context.Background(), tools, namedSources(root))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	b := entry(t, res, "b")
	want := []string{"linux/arm64", "linux/amd64"}
	if len(b.Platforms) != len(want) {
		t.Fatalf("b platforms: got %v, want %v", b.Platforms, want)
	}
	for i, p := range want {
		if b.Platforms[i] != p {
			t.Fatalf("b platforms: got %v, want %v", b.Platforms, want)
		}
	}
}

func TestResolveExactDepsDivergeConflict(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, pkgSpec{name: "d", versions: "[v2.0.0, v1.0.0]"})
	writeSpec(t, root, pkgSpec{name: "p1", deps: []string{"{name: d, version: v1.0.0}"}, versions: "[v1.0.0]"})
	writeSpec(t, root, pkgSpec{name: "p2", deps: []string{"{name: d, version: v2.0.0}"}, versions: "[v1.0.0]"})
	p1 := resolve.Tool{Key: project.ToolKey{Name: "p1"}}
	p2 := resolve.Tool{Key: project.ToolKey{Name: "p2"}}
	bothOrders(t, "p1 first", "p2 first", p1, p2, func(t *testing.T, tools []resolve.Tool) {
		_, err := resolve.Resolve(context.Background(), tools, namedSources(root))
		var sce *resolve.CompatConflictError
		if !errors.As(err, &sce) {
			t.Fatalf("want CompatConflictError, got %v", err)
		}
		if sce.Name != "d" || strings.Join(sce.Compats, ",") != "v1.0.0,v2.0.0" {
			t.Fatalf("conflict fields should be canonical in any order: %+v", sce)
		}
		if !strings.Contains(err.Error(), "conflicting requirements v1.0.0, v2.0.0") {
			t.Fatalf("error should state the requirements without attribution: %v", err)
		}
	})
}

func TestResolveUnpinnedRootYieldsToExactDep(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, pkgSpec{name: "a", versions: "[v2.0.0, v1.0.0]"})
	writeSpec(t, root, pkgSpec{name: "b", deps: []string{"{name: a, version: v1.0.0}"}, versions: "[v1.0.0]"})
	a := resolve.Tool{Key: project.ToolKey{Name: "a"}}
	b := resolve.Tool{Key: project.ToolKey{Name: "b"}}
	bothOrders(t, "root first", "dep first", a, b, func(t *testing.T, tools []resolve.Tool) {
		res, err := resolve.Resolve(context.Background(), tools, namedSources(root))
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if e := entry(t, res, "a"); e.Version != "v1.0.0" || e.Catalog != "cat" || !e.Direct {
			t.Fatalf("the unpinned root must yield to the exact dep: %+v", e)
		}
	})
}

func TestResolveMissingExplicitVersion(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, pkgSpec{name: "a", versions: "[v1.0.0]"})
	tools := []resolve.Tool{{Key: project.ToolKey{Name: "a"}, Version: "v9.9.9"}}
	_, err := resolve.Resolve(context.Background(), tools, namedSources(root))
	var vnf *catalog.VersionNotFoundError
	if !errors.As(err, &vnf) {
		t.Fatalf("want VersionNotFoundError, got %v", err)
	}
	if vnf.Name != "a" || vnf.Version != "v9.9.9" || vnf.Catalog != "cat" {
		t.Fatalf("VersionNotFoundError: %+v", vnf)
	}
}

func TestResolvePinnedToolKey(t *testing.T) {
	rootA := t.TempDir()
	writeSpec(t, rootA, pkgSpec{name: "shared", versions: "[v1.0.0]"})
	rootB := t.TempDir()
	writeSpec(t, rootB, pkgSpec{name: "shared", versions: "[v2.0.0]"})
	sources := []catalog.Named{{Name: "a", Source: catalog.NewDir(rootA)}, {Name: "b", Source: catalog.NewDir(rootB)}}

	toolsB := []resolve.Tool{{Key: project.ToolKey{Catalog: "b", Name: "shared"}}}
	resB, err := resolve.Resolve(context.Background(), toolsB, sources)
	if err != nil {
		t.Fatalf("Resolve (pin b): %v", err)
	}
	eB := entry(t, resB, "shared")
	if eB.Catalog != "b" || eB.Version != "v2.0.0" {
		t.Fatalf("pinned to b: %+v", eB)
	}

	toolsA := []resolve.Tool{{Key: project.ToolKey{Catalog: "a", Name: "shared"}}}
	resA, err := resolve.Resolve(context.Background(), toolsA, sources)
	if err != nil {
		t.Fatalf("Resolve (pin a): %v", err)
	}
	eA := entry(t, resA, "shared")
	if eA.Catalog != "a" || eA.Version != "v1.0.0" {
		t.Fatalf("pinned to a: %+v", eA)
	}
}

func TestResolveEqualVersionPinKeepsAttribution(t *testing.T) {
	rootA := t.TempDir()
	writeSpec(t, rootA, pkgSpec{name: "shared", versions: "[v1.0.0]"})
	writeSpec(t, rootA, pkgSpec{name: "consumer", deps: []string{"{name: shared}"}, versions: "[v1.0.0]"})
	rootB := t.TempDir()
	writeSpec(t, rootB, pkgSpec{name: "shared", versions: "[v1.0.0]"})
	sources := []catalog.Named{
		{Name: "a", Source: catalog.NewDir(rootA)},
		{Name: "b", Source: catalog.NewDir(rootB)},
	}
	pin := resolve.Tool{Key: project.ToolKey{Catalog: "b", Name: "shared"}, Version: "v1.0.0"}
	consumer := resolve.Tool{Key: project.ToolKey{Name: "consumer"}}
	bothOrders(t, "pin first", "dep first", pin, consumer, func(t *testing.T, tools []resolve.Tool) {
		res, err := resolve.Resolve(context.Background(), tools, sources)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		shared := entry(t, res, "shared")
		if shared.Version != "v1.0.0" || shared.Catalog != "b" {
			t.Fatalf("the catalog-pinned direct tool (catalog b) must win the tie in either order, got %+v", shared)
		}
	})
}

func TestResolveUnpinnedQualifiedRootSelectsFromItsCatalog(t *testing.T) {
	rootA := t.TempDir()
	writeSpec(t, rootA, pkgSpec{name: "shared", versions: "[v3.0.0, v1.0.0]"})
	writeSpec(t, rootA, pkgSpec{name: "app", deps: []string{"{name: shared, version: v1.0.0}"}, versions: "[v1.0.0]"})
	rootB := t.TempDir()
	writeSpec(t, rootB, pkgSpec{name: "shared", versions: "[v2.0.0, v1.0.0]"})
	sources := []catalog.Named{
		{Name: "a", Source: catalog.NewDir(rootA)},
		{Name: "b", Source: catalog.NewDir(rootB)},
	}
	tools := []resolve.Tool{
		{Key: project.ToolKey{Catalog: "b", Name: "shared"}},
		{Key: project.ToolKey{Name: "app"}},
	}
	res, err := resolve.Resolve(context.Background(), tools, sources)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if e := entry(t, res, "shared"); e.Version != "v1.0.0" || e.Catalog != "b" {
		t.Fatalf("the yielded pick must come from the qualified catalog: %+v", e)
	}
}

func TestResolveUnpinnedQualifiedRootMissingBoundVersionErrors(t *testing.T) {
	rootA := t.TempDir()
	writeSpec(t, rootA, pkgSpec{name: "shared", versions: "[v3.0.0, v1.0.0]"})
	writeSpec(t, rootA, pkgSpec{name: "app", deps: []string{"{name: shared, version: v1.0.0}"}, versions: "[v1.0.0]"})
	rootB := t.TempDir()
	writeSpec(t, rootB, pkgSpec{name: "shared", versions: "[v2.0.0]"})
	sources := []catalog.Named{
		{Name: "a", Source: catalog.NewDir(rootA)},
		{Name: "b", Source: catalog.NewDir(rootB)},
	}
	tools := []resolve.Tool{
		{Key: project.ToolKey{Catalog: "b", Name: "shared"}},
		{Key: project.ToolKey{Name: "app"}},
	}
	_, err := resolve.Resolve(context.Background(), tools, sources)
	var vnf *catalog.VersionNotFoundError
	if !errors.As(err, &vnf) {
		t.Fatalf("want VersionNotFoundError when the bound version is absent from the qualified catalog, got %v", err)
	}
	if vnf.Catalog != "b" {
		t.Fatalf("error should name the qualified catalog: %+v", vnf)
	}
}

func TestResolveQualifiedRootMissingRangeErrors(t *testing.T) {
	rootA := t.TempDir()
	writeSpec(t, rootA, pkgSpec{name: "shared", libs: "[lib]", versions: "[v3.0.0, v1.9.5]"})
	writeSpec(t, rootA, pkgSpec{name: "app", deps: []string{`{name: shared, kind: link, compat: "1.9"}`}, versions: "[v1.0.0]"})
	rootB := t.TempDir()
	writeSpec(t, rootB, pkgSpec{name: "shared", libs: "[lib]", versions: "[v2.0.0]"})
	sources := []catalog.Named{
		{Name: "a", Source: catalog.NewDir(rootA)},
		{Name: "b", Source: catalog.NewDir(rootB)},
	}
	tools := []resolve.Tool{
		{Key: project.ToolKey{Catalog: "b", Name: "shared"}},
		{Key: project.ToolKey{Name: "app"}},
	}
	_, err := resolve.Resolve(context.Background(), tools, sources)
	var vnf *catalog.VersionNotFoundError
	if !errors.As(err, &vnf) {
		t.Fatalf("want VersionNotFoundError when the qualified catalog lists no version in range, got %v", err)
	}
	if vnf.Catalog != "b" {
		t.Fatalf("error should name the qualified catalog: %+v", vnf)
	}
}

func TestResolveDisjointRangesCanonicalConflict(t *testing.T) {
	root := t.TempDir()
	for _, p := range []struct{ name, compat string }{
		{"r1", "1"}, {"r2", "2"}, {"r3", "3"},
	} {
		writeSpec(t, root, pkgSpec{
			name:      p.name,
			platforms: "[darwin/arm64]",
			deps:      []string{`{name: openssl, kind: link, compat: "` + p.compat + `"}`},
			versions:  "[v1.0.0]",
		})
	}
	writeSpec(t, root, pkgSpec{name: "openssl", platforms: "[darwin/arm64]", libs: "[lib]", versions: "[v3.0.0, v2.0.0, v1.0.0]"})
	perms := [][]string{
		{"r1", "r2", "r3"}, {"r3", "r2", "r1"},
	}
	for _, order := range perms {
		t.Run(strings.Join(order, ","), func(t *testing.T) {
			tools := make([]resolve.Tool, len(order))
			for i, n := range order {
				tools[i] = resolve.Tool{Key: project.ToolKey{Name: n}}
			}
			_, err := resolve.Resolve(context.Background(), tools, namedSources(root))
			var sce *resolve.CompatConflictError
			if !errors.As(err, &sce) {
				t.Fatalf("want CompatConflictError, got %v", err)
			}
			if sce.Name != "openssl" || strings.Join(sce.Compats, ",") != "1,2,3" {
				t.Fatalf("conflict should list every range canonically in any order: %+v", sce)
			}
		})
	}
}

func TestResolveRolesRunVsLink(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, pkgSpec{name: "gpgme", deps: []string{"{name: gpg}", "{name: openssl, kind: link}"}, versions: "[v1.0.0]"})
	writeSpec(t, root, pkgSpec{name: "gpg", versions: "[v1.0.0]"})
	writeSpec(t, root, pkgSpec{name: "openssl", libs: "[lib]", versions: "[v3.4.0]"})
	tools := []resolve.Tool{{Key: project.ToolKey{Name: "gpgme"}}}
	res, err := resolve.Resolve(context.Background(), tools, namedSources(root))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	gpgme := entry(t, res, "gpgme")
	gpg := entry(t, res, "gpg")
	openssl := entry(t, res, "openssl")
	if !gpgme.OnPath {
		t.Fatalf("gpgme (direct) should be on_path: %+v", gpgme)
	}
	if !gpg.OnPath || gpg.OnLoaderPath {
		t.Fatalf("gpg (run dep) should be on_path only: %+v", gpg)
	}
	if openssl.OnPath || !openssl.OnLoaderPath {
		t.Fatalf("openssl (link dep with libs) should be on_loader_path only, never on_path: %+v", openssl)
	}
}

func TestResolveDepsBuildRolesRunVsLink(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, pkgSpec{name: "openssl", libs: "[lib]", versions: "[v1.0.0]"})
	writeSpec(t, root, pkgSpec{name: "make", versions: "[v1.0.0]"})
	pkg := &spec.Package{
		Schema: 2,
		Name:   "tool",
		Build: &spec.Build{
			Deps: []spec.Dep{
				{Name: "openssl", Kind: spec.DepKindLink},
				{Name: "make"},
			},
		},
	}
	res, err := resolve.Dependencies(context.Background(), pkg, pkg.Build.Deps, namedSources(root))
	if err != nil {
		t.Fatalf("ResolveDeps: %v", err)
	}
	openssl := entry(t, res, "openssl")
	makePkg := entry(t, res, "make")
	if !openssl.OnLoaderPath || openssl.OnPath {
		t.Fatalf("openssl (link build dep with libs) should be on_loader_path only: %+v", openssl)
	}
	if !makePkg.OnPath || makePkg.OnLoaderPath {
		t.Fatalf("make (run build dep) should be on_path only: %+v", makePkg)
	}
}

func TestResolveCompatTighterAndWiderReconcile(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, pkgSpec{name: "p1", deps: []string{`{name: openssl, kind: link, compat: "3.4"}`}, versions: "[v1.0.0]"})
	writeSpec(t, root, pkgSpec{name: "p2", deps: []string{`{name: openssl, kind: link, compat: "3"}`}, versions: "[v1.0.0]"})
	writeSpec(t, root, pkgSpec{name: "openssl", libs: "[lib]", versions: "[v3.5.1, v3.4.9, v3.4.2, v1.1.1]"})
	tools := []resolve.Tool{{Key: project.ToolKey{Name: "p1"}}, {Key: project.ToolKey{Name: "p2"}}}
	res, err := resolve.Resolve(context.Background(), tools, namedSources(root))
	if err != nil {
		t.Fatalf("compatible ranges must reconcile, got error: %v", err)
	}
	if v := entry(t, res, "openssl").Version; v != "v3.4.9" {
		t.Fatalf("openssl = %s, want highest satisfying both 3 and 3.4 (v3.4.9)", v)
	}
}

func TestResolveCycleTerminates(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, pkgSpec{name: "a", deps: []string{"{name: b}"}, versions: "[v1.0.0]"})
	writeSpec(t, root, pkgSpec{name: "b", deps: []string{"{name: a}"}, versions: "[v1.0.0]"})
	tools := []resolve.Tool{{Key: project.ToolKey{Name: "a"}}}

	done := make(chan struct{})
	var res *resolve.Result
	var err error
	go func() {
		res, err = resolve.Resolve(context.Background(), tools, namedSources(root))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Resolve did not terminate on a dependency cycle")
	}
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(res.Entries) != 2 {
		t.Fatalf("want 2 entries (a, b), got %+v", res.Entries)
	}
}

func TestResolvePinnedRootDepOutcome(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, pkgSpec{name: "a", versions: "[v2.0.0, v1.0.0]"})
	writeSpec(t, root, pkgSpec{name: "b", deps: []string{"{name: a, version: v2.0.0}"}, versions: "[v1.0.0]"})
	cases := []struct {
		name         string
		pin          string
		wantPinned   string
		wantRequired string
		wantVersion  string
	}{
		{name: "conflict errors", pin: "v1.0.0", wantPinned: "v1.0.0", wantRequired: "v2.0.0"},
		{name: "agreement resolves", pin: "v2.0.0", wantVersion: "v2.0.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tools := []resolve.Tool{
				{Key: project.ToolKey{Name: "a"}, Version: tc.pin},
				{Key: project.ToolKey{Name: "b"}},
			}
			res, err := resolve.Resolve(context.Background(), tools, namedSources(root))
			if tc.wantPinned != "" {
				var pce *resolve.PinConflictError
				if !errors.As(err, &pce) {
					t.Fatalf("want PinConflictError, got %v", err)
				}
				if pce.Name != "a" || pce.Pinned != tc.wantPinned || pce.Required != tc.wantRequired {
					t.Fatalf("PinConflictError: %+v", pce)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if a := entry(t, res, "a"); a.Version != tc.wantVersion || !a.Direct {
				t.Fatalf("entry a: %+v", a)
			}
		})
	}
}

func TestResolveBareDepDefersToPinnedRoot(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, pkgSpec{name: "a", versions: "[v2.0.0, v1.0.0]"})
	writeSpec(t, root, pkgSpec{name: "b", deps: []string{"{name: a}"}, versions: "[v1.0.0]"})
	pin := resolve.Tool{Key: project.ToolKey{Name: "a"}, Version: "v1.0.0"}
	dep := resolve.Tool{Key: project.ToolKey{Name: "b"}}
	bothOrders(t, "pin first", "dep first", pin, dep, func(t *testing.T, tools []resolve.Tool) {
		res, err := resolve.Resolve(context.Background(), tools, namedSources(root))
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if a := entry(t, res, "a"); a.Version != "v1.0.0" || !a.Direct {
			t.Fatalf("entry a: %+v, want pinned v1.0.0", a)
		}
	})
}

func TestResolveBareDepAloneFloatsToLatest(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, pkgSpec{name: "a", versions: "[v2.0.0, v1.0.0]"})
	writeSpec(t, root, pkgSpec{name: "b", deps: []string{"{name: a}"}, versions: "[v1.0.0]"})
	tools := []resolve.Tool{{Key: project.ToolKey{Name: "b"}}}
	res, err := resolve.Resolve(context.Background(), tools, namedSources(root))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if a := entry(t, res, "a"); a.Version != "v2.0.0" {
		t.Fatalf("unconstrained dep alone should float to latest, got %+v", a)
	}
}

func TestResolveDepsBareBuildDepDefersToExactDep(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, pkgSpec{name: "a", versions: "[v2.0.0, v1.0.0]"})
	writeSpec(t, root, pkgSpec{name: "b", deps: []string{"{name: a, version: v1.0.0}"}, versions: "[v1.0.0]"})
	pkg := &spec.Package{
		Schema: 2,
		Name:   "tool",
		Build: &spec.Build{
			Deps: []spec.Dep{
				{Name: "a"},
				{Name: "b"},
			},
		},
	}
	res, err := resolve.Dependencies(context.Background(), pkg, pkg.Build.Deps, namedSources(root))
	if err != nil {
		t.Fatalf("ResolveDeps: %v", err)
	}
	if a := entry(t, res, "a"); a.Version != "v1.0.0" {
		t.Fatalf("bare build dep floated a to %s over b's exact v1.0.0", a.Version)
	}
}

func TestResolvePinnedRootWithinCompatRangeKeepsPin(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, pkgSpec{name: "app", deps: []string{`{name: gpgme, kind: link, compat: "1"}`}, versions: "[v1.0.0]"})
	writeSpec(t, root, pkgSpec{name: "gpgme", libs: "[lib]", versions: "[2.0.0, 1.24.3, 1.24.2]"})
	pin := resolve.Tool{Key: project.ToolKey{Name: "gpgme"}, Version: "1.24.2"}
	app := resolve.Tool{Key: project.ToolKey{Name: "app"}}
	bothOrders(t, "pin first", "dep first", pin, app, func(t *testing.T, tools []resolve.Tool) {
		res, err := resolve.Resolve(context.Background(), tools, namedSources(root))
		if err != nil {
			t.Fatalf("in-range pin must satisfy the compat edge, got error: %v", err)
		}
		if g := entry(t, res, "gpgme"); g.Version != "1.24.2" || !g.Direct {
			t.Fatalf("entry gpgme: %+v, want pinned 1.24.2", g)
		}
	})
}

func TestResolveExactDepOutsideCompatRangeConflicts(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, pkgSpec{name: "p1", platforms: "[darwin/arm64]", deps: []string{"{name: openssl, version: v1.1.1}"}, versions: "[v1.0.0]"})
	writeSpec(t, root, pkgSpec{name: "p2", platforms: "[darwin/arm64]", deps: []string{`{name: openssl, kind: link, compat: "3"}`}, versions: "[v1.0.0]"})
	writeSpec(t, root, pkgSpec{name: "openssl", platforms: "[darwin/arm64]", libs: "[lib]", versions: "[v3.5.1, v1.1.1]"})
	p1 := resolve.Tool{Key: project.ToolKey{Name: "p1"}}
	p2 := resolve.Tool{Key: project.ToolKey{Name: "p2"}}
	bothOrders(t, "exact first", "compat first", p1, p2, func(t *testing.T, tools []resolve.Tool) {
		_, err := resolve.Resolve(context.Background(), tools, namedSources(root))
		var sce *resolve.CompatConflictError
		if !errors.As(err, &sce) {
			t.Fatalf("want CompatConflictError, got %v", err)
		}
		if sce.Name != "openssl" {
			t.Fatalf("conflict should name openssl: %+v", sce)
		}
	})
}

func TestResolveUnpinnedRootYieldsToCompatRange(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, pkgSpec{name: "app", platforms: "[darwin/arm64]", deps: []string{`{name: openssl, kind: link, compat: "3"}`}, versions: "[v1.0.0]"})
	writeSpec(t, root, pkgSpec{name: "openssl", platforms: "[darwin/arm64]", libs: "[lib]", versions: "[v4.0.0, v3.5.1]"})
	openssl := resolve.Tool{Key: project.ToolKey{Name: "openssl"}}
	app := resolve.Tool{Key: project.ToolKey{Name: "app"}}
	bothOrders(t, "root first", "dep first", openssl, app, func(t *testing.T, tools []resolve.Tool) {
		res, err := resolve.Resolve(context.Background(), tools, namedSources(root))
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if e := entry(t, res, "openssl"); e.Version != "v3.5.1" {
			t.Fatalf("the unpinned root must settle on the range's highest: %+v", e)
		}
	})
}

func TestResolveOrderIndependentConflict(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, pkgSpec{name: "p1", platforms: "[darwin/arm64]", deps: []string{"{name: openssl, version: v1.8.0}"}, versions: "[v1.0.0]"})
	writeSpec(t, root, pkgSpec{name: "p2", platforms: "[darwin/arm64]", deps: []string{"{name: openssl, version: v1.9.0}"}, versions: "[v1.0.0]"})
	writeSpec(t, root, pkgSpec{name: "p3", platforms: "[darwin/arm64]", deps: []string{`{name: openssl, kind: link, compat: "1.9"}`}, versions: "[v1.0.0]"})
	writeSpec(t, root, pkgSpec{name: "openssl", platforms: "[darwin/arm64]", libs: "[lib]", versions: "[v1.9.0, v1.8.0]"})
	perms := [][]string{
		{"p1", "p2", "p3"}, {"p3", "p2", "p1"},
	}
	for _, order := range perms {
		t.Run(strings.Join(order, ","), func(t *testing.T) {
			tools := make([]resolve.Tool, len(order))
			for i, n := range order {
				tools[i] = resolve.Tool{Key: project.ToolKey{Name: n}}
			}
			_, err := resolve.Resolve(context.Background(), tools, namedSources(root))
			var sce *resolve.CompatConflictError
			if !errors.As(err, &sce) {
				t.Fatalf("want CompatConflictError, got %v", err)
			}
			if sce.Name != "openssl" || strings.Join(sce.Compats, ",") != "v1.8.0,1.9,v1.9.0" {
				t.Fatalf("conflict should list every requirement canonically: %+v", sce)
			}
		})
	}
}

func TestResolvePinnedRootOutsideCompatRangeConflicts(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, pkgSpec{name: "app", deps: []string{`{name: openssl, kind: link, compat: "3"}`}, versions: "[v1.0.0]"})
	writeSpec(t, root, pkgSpec{name: "openssl", libs: "[lib]", versions: "[v3.5.1, v1.1.1]"})
	tools := []resolve.Tool{
		{Key: project.ToolKey{Name: "openssl"}, Version: "v1.1.1"},
		{Key: project.ToolKey{Name: "app"}},
	}
	_, err := resolve.Resolve(context.Background(), tools, namedSources(root))
	var pce *resolve.PinConflictError
	if !errors.As(err, &pce) {
		t.Fatalf("want PinConflictError, got %v", err)
	}
	if pce.Name != "openssl" || pce.Pinned != "v1.1.1" || pce.Required != "v3.5.1" {
		t.Fatalf("PinConflictError: %+v", pce)
	}
}

func TestResolveDepsWalksTheGivenList(t *testing.T) {
	root := t.TempDir()
	writeSpec(t, root, pkgSpec{name: "alpha", versions: "[v1.0.0]"})
	writeSpec(t, root, pkgSpec{name: "beta", versions: "[v2.0.0]"})
	pkg := &spec.Package{
		Schema: 2,
		Name:   "tool",
		Deps:   []spec.Dep{{Name: "alpha"}},
		Build:  &spec.Build{Deps: []spec.Dep{{Name: "beta"}}},
	}

	res, err := resolve.Dependencies(context.Background(), pkg, pkg.Deps, namedSources(root))
	if err != nil {
		t.Fatalf("ResolveDeps: %v", err)
	}
	if len(res.Entries) != 1 {
		t.Fatalf("want exactly the runtime dep, got %+v", res.Entries)
	}
	if got := entry(t, res, "alpha"); got.Version != "v1.0.0" {
		t.Fatalf("alpha version = %q", got.Version)
	}
}
