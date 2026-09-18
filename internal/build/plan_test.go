package build_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/vi-dev/nem/internal/build"
	"github.com/vi-dev/nem/internal/spec"
)

var (
	linuxAmd = spec.Platform{OS: "linux", Arch: "amd64"}
	darwin   = spec.Platform{OS: "darwin", Arch: "arm64"}
)

func sel(name string, plats []spec.Platform, deps ...spec.Dep) build.Selection {
	return build.Selection{
		Pkg:       &spec.Package{Schema: 2, Name: name, Deps: deps},
		Version:   "v1",
		Reason:    "missing archive",
		Platforms: plats,
	}
}

func selV(pkg *spec.Package, version string, plats []spec.Platform) build.Selection {
	return build.Selection{
		Pkg:       pkg,
		Version:   version,
		Reason:    "missing archive",
		Platforms: plats,
	}
}

func waves(t *testing.T, p build.Plan) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, e := range p.Entries {
		out[e.Pkg.Name] = e.Wave
	}
	return out
}

func TestComputePlanOrdersDeps(t *testing.T) {
	p, err := build.ComputePlan([]build.Selection{
		sel("app", []spec.Platform{linuxAmd}, spec.Dep{Name: "lib"}),
		sel("lib", []spec.Platform{linuxAmd}),
		sel("solo", []spec.Platform{linuxAmd}),
	})
	if err != nil {
		t.Fatalf("ComputePlan: %v", err)
	}
	w := waves(t, p)
	if w["lib"] != 1 || w["solo"] != 1 || w["app"] != 2 {
		t.Fatalf("waves: %+v", w)
	}
	for _, e := range p.Entries {
		if e.Pkg.Name == "app" && (len(e.Needs) != 1 || e.Needs[0] != "lib") {
			t.Fatalf("app.Needs = %v", e.Needs)
		}
	}
}

func TestComputePlanPlatformFilteredEdgeIsNoEdge(t *testing.T) {

	p, err := build.ComputePlan([]build.Selection{
		sel("app", []spec.Platform{darwin},
			spec.Dep{Name: "lib", Platforms: []spec.Platform{{OS: "linux", Arch: "amd64"}}}),
		sel("lib", []spec.Platform{linuxAmd}),
	})
	if err != nil {
		t.Fatalf("ComputePlan: %v", err)
	}
	w := waves(t, p)
	if w["app"] != 1 || w["lib"] != 1 {
		t.Fatalf("expected both in wave 1, got %+v", w)
	}
}

func TestComputePlanCycle(t *testing.T) {
	_, err := build.ComputePlan([]build.Selection{
		sel("a", []spec.Platform{linuxAmd}, spec.Dep{Name: "b"}),
		sel("b", []spec.Platform{linuxAmd}, spec.Dep{Name: "a"}),
	})
	var ce *build.CycleError
	if !errors.As(err, &ce) {
		t.Fatalf("want CycleError, got %v", err)
	}
	if len(ce.Names) != 2 {
		t.Fatalf("cycle names: %v", ce.Names)
	}
}

func TestComputePlanExpandsVersionsPerName(t *testing.T) {
	aPkg := &spec.Package{Schema: 2, Name: "a"}
	p, err := build.ComputePlan([]build.Selection{
		selV(aPkg, "v2", []spec.Platform{linuxAmd}),
		selV(aPkg, "v1", []spec.Platform{linuxAmd}),
		sel("b", []spec.Platform{linuxAmd}, spec.Dep{Name: "a"}),
	})
	if err != nil {
		t.Fatalf("ComputePlan: %v", err)
	}
	if len(p.Entries) != 3 {
		t.Fatalf("entries = %+v", p.Entries)
	}
	got := [3]struct {
		Name    string
		Version string
	}{}
	for i, e := range p.Entries {
		got[i] = struct {
			Name    string
			Version string
		}{e.Pkg.Name, e.Version}
	}
	want := [3]struct {
		Name    string
		Version string
	}{{"a", "v2"}, {"a", "v1"}, {"b", "v1"}}
	if got != want {
		t.Fatalf("entries order = %+v, want %+v", got, want)
	}
	if p.Entries[0].Wave != 1 || p.Entries[1].Wave != 1 {
		t.Fatalf("both a entries must share wave 1, got %+v", p.Entries[:2])
	}
	if p.Entries[2].Wave != 2 {
		t.Fatalf("b wave = %d, want 2", p.Entries[2].Wave)
	}
	if len(p.Entries[2].Needs) != 1 || p.Entries[2].Needs[0] != "a" {
		t.Fatalf("b.Needs = %v", p.Entries[2].Needs)
	}
}

func TestComputePlanRejectsDuplicateNameVersion(t *testing.T) {
	aPkg := &spec.Package{Schema: 2, Name: "a"}
	_, err := build.ComputePlan([]build.Selection{
		selV(aPkg, "v1", []spec.Platform{linuxAmd}),
		selV(aPkg, "v1", []spec.Platform{darwin}),
	})
	if err == nil {
		t.Fatal("duplicate (name, version) selections must error")
	}
	if !strings.Contains(err.Error(), "a@v1") {
		t.Fatalf("error = %v, want to contain %q", err, "a@v1")
	}
}

func TestComputePlanUnionsPlatformsForEdges(t *testing.T) {
	aPkg := &spec.Package{Schema: 2, Name: "a"}
	p, err := build.ComputePlan([]build.Selection{
		selV(aPkg, "v2", []spec.Platform{darwin}),
		selV(aPkg, "v1", []spec.Platform{linuxAmd}),
		sel("b", []spec.Platform{linuxAmd}, spec.Dep{Name: "a"}),
	})
	if err != nil {
		t.Fatalf("ComputePlan: %v", err)
	}
	w := waves(t, p)
	if w["a"] != 1 {
		t.Fatalf("a wave = %d, want 1", w["a"])
	}
	if w["b"] != 2 {
		t.Fatalf("b wave = %d, want 2 (edge must exist via unioned platforms)", w["b"])
	}
}
