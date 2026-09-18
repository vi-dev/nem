package install

import (
	"context"
	"testing"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/ocix"
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

	jobs := Jobs(result, set, nil)
	if len(jobs) != 2 {
		t.Fatalf("want 2 jobs (other platform filtered), got %+v", jobs)
	}
	if jobs[0].Pkg != pkg || jobs[0].Source.CatalogRef != "example.com/cat:v1" {
		t.Fatalf("ref must come from the set entry: %+v", jobs[0])
	}
	if jobs[1].Source.CatalogRef != "" {
		t.Fatalf("unknown catalog must yield empty ref: %+v", jobs[1])
	}
	if jobs[0].Reinstall || jobs[0].Source.LocalArchives != nil {
		t.Fatalf("nil local store must not inject archives: %+v", jobs[0])
	}
}

func TestJobsWiresLocalStore(t *testing.T) {
	store := ocix.NewArchiveStore(t.TempDir())
	target, _ := store.Open("libgomp")
	_, _, err := ocix.PushArchive(context.Background(), target, "v1", spec.Current(), []byte("x"), false)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	if _, err := store.Open("zlib"); err != nil {
		t.Fatalf("open zlib: %v", err)
	}

	curlTarget, _ := store.Open("curl")
	if _, _, err := ocix.PushArchive(context.Background(), curlTarget, "v1", spec.Current(), []byte("y"), false); err != nil {
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
	if !byName["libgomp"].Reinstall || byName["libgomp"].Source.LocalArchives == nil {
		t.Fatalf("libgomp job must reinstall from local store: %+v", byName["libgomp"])
	}
	if byName["openssl"].Reinstall || byName["openssl"].Source.LocalArchives != nil {
		t.Fatalf("openssl is not in the store; must not be served from it: %+v", byName["openssl"])
	}
	if byName["zlib"].Reinstall || byName["zlib"].Source.LocalArchives != nil {
		t.Fatalf("zlib was opened but never staged; must not be served from the store: %+v", byName["zlib"])
	}
	if byName["curl"].Reinstall || byName["curl"].Source.LocalArchives != nil {
		t.Fatalf("curl is staged at a different version; must not be served from the store: %+v", byName["curl"])
	}
}
