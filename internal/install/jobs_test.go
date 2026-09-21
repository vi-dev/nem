package install

import (
	"context"
	"testing"

	"github.com/vi-dev/nem/internal/archive"
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
	var store archive.Store = archive.Remote("example.com/cat:v1")
	set := catalog.NewSet(catalog.Entry{Name: "extra", Ref: "example.com/cat:v1", Archives: store})

	jobs := Jobs(result, set, nil)
	if len(jobs) != 2 {
		t.Fatalf("want 2 jobs (other platform filtered), got %+v", jobs)
	}
	if jobs[0].Pkg != pkg || len(jobs[0].Source.Archives) != 1 || jobs[0].Source.Archives[0] != store {
		t.Fatalf("archives must come from the set entry: %+v", jobs[0])
	}
	if len(jobs[1].Source.Archives) != 0 {
		t.Fatalf("unknown catalog must yield no archives: %+v", jobs[1])
	}
	if jobs[0].Reinstall {
		t.Fatalf("nil local store must not force reinstall: %+v", jobs[0])
	}
}

func TestJobsWiresLocalStore(t *testing.T) {
	store := archive.NewDir(t.TempDir())
	target, _ := store.OpenRW(context.Background(), "libgomp")
	_, _, err := archive.Push(context.Background(), target, "v1", spec.Current(), archive.BytesBlob([]byte("x")), false)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	if _, err := store.OpenRW(context.Background(), "zlib"); err != nil {
		t.Fatalf("open zlib: %v", err)
	}

	curlTarget, _ := store.OpenRW(context.Background(), "curl")
	if _, _, err := archive.Push(context.Background(), curlTarget, "v1", spec.Current(), archive.BytesBlob([]byte("y")), false); err != nil {
		t.Fatalf("seed curl: %v", err)
	}
	res := &resolve.Result{
		Entries: []project.LockEntry{
			{Name: "libgomp", Version: "v1", Platforms: []string{spec.Current().String()}},
			{Name: "openssl", Version: "v3", Platforms: []string{spec.Current().String()}},
			{Name: "zlib", Version: "v1.3", Platforms: []string{spec.Current().String()}},
			{Name: "curl", Version: "v2", Platforms: []string{spec.Current().String()}},
		},
		Pkgs: map[string]*spec.Package{
			"libgomp": {Schema: 2, Name: "libgomp"},
			"openssl": {Schema: 2, Name: "openssl"},
			"zlib":    {Schema: 2, Name: "zlib"},
			"curl":    {Schema: 2, Name: "curl"},
		},
	}
	jobs := Jobs(res, catalog.NewSet(), store)
	byName := map[string]Job{}
	for _, j := range jobs {
		byName[j.Pkg.Name] = j
	}
	if !byName["libgomp"].Reinstall || len(byName["libgomp"].Source.Archives) != 1 || byName["libgomp"].Source.Archives[0] != archive.Store(store) {
		t.Fatalf("libgomp job must reinstall from local store: %+v", byName["libgomp"])
	}
	if byName["openssl"].Reinstall || len(byName["openssl"].Source.Archives) != 0 {
		t.Fatalf("openssl is not in the store; must not be served from it: %+v", byName["openssl"])
	}
	if byName["zlib"].Reinstall || len(byName["zlib"].Source.Archives) != 0 {
		t.Fatalf("zlib was opened but never staged; must not be served from the store: %+v", byName["zlib"])
	}
	if byName["curl"].Reinstall || len(byName["curl"].Source.Archives) != 0 {
		t.Fatalf("curl is staged at a different version; must not be served from the store: %+v", byName["curl"])
	}
}

func TestJobsPrependsLocalStoreBeforeCatalogArchives(t *testing.T) {
	local := archive.NewDir(t.TempDir())
	target, err := local.OpenRW(context.Background(), "tool")
	if err != nil {
		t.Fatalf("open local: %v", err)
	}
	if _, _, err := archive.Push(context.Background(), target, "v1", spec.Current(), archive.BytesBlob([]byte("x")), false); err != nil {
		t.Fatalf("seed local: %v", err)
	}

	var catalogStore archive.Store = archive.Remote("example.com/cat:v1")
	set := catalog.NewSet(catalog.Entry{Name: "extra", Ref: "example.com/cat:v1", Archives: catalogStore})

	res := &resolve.Result{
		Entries: []project.LockEntry{
			{Name: "tool", Version: "v1", Catalog: "extra", Platforms: []string{spec.Current().String()}},
		},
		Pkgs: map[string]*spec.Package{"tool": {Schema: 2, Name: "tool"}},
	}

	jobs := Jobs(res, set, local)
	if len(jobs) != 1 {
		t.Fatalf("want 1 job, got %+v", jobs)
	}
	archives := jobs[0].Source.Archives
	if len(archives) != 2 || archives[0] != archive.Store(local) || archives[1] != catalogStore {
		t.Fatalf("local store must be prepended before catalog archives: %+v", archives)
	}
	if !jobs[0].Reinstall {
		t.Fatalf("local hit must force reinstall: %+v", jobs[0])
	}
}
