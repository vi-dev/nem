package install

import (
	"testing"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/project"
	"github.com/vi-dev/nem/internal/resolve"
	"github.com/vi-dev/nem/internal/spec"
)

func TestJobsRefFromSet(t *testing.T) {
	current := spec.Current().String()
	pkg := &spec.Package{Name: "tool"}
	other := &spec.Package{Name: "other"}
	stray := &spec.Package{Name: "stray"}
	result := &resolve.Result{
		Entries: []project.LockEntry{
			{Name: "tool", Version: "1.0.0", Catalog: "extra", Platforms: []string{current}},
			{Name: "stray", Version: "2.0.0", Catalog: "unknown", Platforms: []string{current}},
			{Name: "other", Version: "3.0.0", Catalog: "extra", Platforms: []string{"plan9/mips"}},
		},
		Pkgs: map[string]*spec.Package{"tool": pkg, "stray": stray, "other": other},
	}
	set := catalog.NewSet(catalog.Entry{Name: "extra", Ref: "example.com/cat:v1"})

	jobs := Jobs(result, set)
	if len(jobs) != 2 {
		t.Fatalf("want 2 jobs (other platform filtered), got %+v", jobs)
	}
	if jobs[0].Pkg != pkg || jobs[0].Source.CatalogRef != "example.com/cat:v1" {
		t.Fatalf("ref must come from the set entry: %+v", jobs[0])
	}
	if jobs[1].Source.CatalogRef != "" {
		t.Fatalf("unknown catalog must yield empty ref: %+v", jobs[1])
	}
}
