package catalog

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/project"
	"github.com/vi-dev/nem/internal/spec"
)

func TestSetFindAndOrder(t *testing.T) {
	a := Entry{Name: "a", Ref: "example.com/a:v1"}
	b := Entry{Name: "b"}
	s := NewSet(a, b)

	if got := s.Entries(); len(got) != 2 || got[0].Name != "a" || got[1].Name != "b" {
		t.Fatalf("Entries: %+v", got)
	}
	e, ok := s.Find("a")
	if !ok || e.Ref != "example.com/a:v1" {
		t.Fatalf("Find(a): %+v, %v", e, ok)
	}
	if _, ok := s.Find("zzz"); ok {
		t.Fatal("Find(zzz) should miss")
	}
}

func TestSetPrependAppendNonMutating(t *testing.T) {
	base := NewSet(Entry{Name: "mid"})
	front := base.Prepend(Entry{Name: "first"})
	back := base.Append(Entry{Name: "last"})

	if got := base.Entries(); len(got) != 1 || got[0].Name != "mid" {
		t.Fatalf("base mutated: %+v", got)
	}
	if got := front.Entries(); len(got) != 2 || got[0].Name != "first" || got[1].Name != "mid" {
		t.Fatalf("Prepend: %+v", got)
	}
	if got := back.Entries(); len(got) != 2 || got[0].Name != "mid" || got[1].Name != "last" {
		t.Fatalf("Append: %+v", got)
	}
}

func TestSetLookupFirstMatchWins(t *testing.T) {
	a := writeDirCatalog(t, "shared", "only-a")
	b := writeDirCatalog(t, "shared", "only-b")
	s := NewSet(Entry{Name: "a", Catalog: NewDir(a)}, Entry{Name: "b", Catalog: NewDir(b)})
	ctx := context.Background()

	hit, err := s.Lookup(ctx, project.ToolKey{Name: "shared"})
	if err != nil || hit.Entry.Name != "a" {
		t.Fatalf("first-match: %+v, %v", hit.Entry, err)
	}
	hit, err = s.Lookup(ctx, project.ToolKey{Name: "only-b"})
	if err != nil || hit.Entry.Name != "b" {
		t.Fatalf("fallthrough: %+v, %v", hit.Entry, err)
	}
	var nf *PackageNotFoundError
	if _, err = s.Lookup(ctx, project.ToolKey{Name: "nowhere"}); !errors.As(err, &nf) || len(nf.Catalogs) != 2 {
		t.Fatalf("exhausted: %v", err)
	}
}

func TestSetLookupPinned(t *testing.T) {
	a := writeDirCatalog(t, "shared")
	b := writeDirCatalog(t, "shared", "only-b")
	s := NewSet(Entry{Name: "a", Catalog: NewDir(a)}, Entry{Name: "b", Ref: "example.com/b:v1", Catalog: NewDir(b)})
	ctx := context.Background()

	hit, err := s.Lookup(ctx, project.ToolKey{Catalog: "b", Name: "shared"})
	if err != nil || hit.Entry.Name != "b" || hit.Entry.Ref != "example.com/b:v1" {
		t.Fatalf("pin honored with provenance: %+v, %v", hit.Entry, err)
	}
	var cnf *NotFoundError
	if _, err := s.Lookup(ctx, project.ToolKey{Catalog: "zzz", Name: "shared"}); !errors.As(err, &cnf) {
		t.Fatalf("want NotFoundError, got %v", err)
	}
	var nf *PackageNotFoundError
	if _, err := s.Lookup(ctx, project.ToolKey{Catalog: "a", Name: "only-b"}); !errors.As(err, &nf) {
		t.Fatalf("pinned miss must not fall through: %v", err)
	}
}

type stubCatalog struct{ err error }

func (s stubCatalog) Summaries(context.Context) ([]Summary, error)       { return nil, s.err }
func (s stubCatalog) Versions(context.Context, string) ([]string, error) { return nil, s.err }
func (s stubCatalog) Package(context.Context, string) (*spec.Package, string, error) {
	return nil, "", s.err
}
func (s stubCatalog) PackageNames(context.Context) ([]string, error)       { return nil, s.err }
func (s stubCatalog) ReadManifest(context.Context, string) ([]byte, error) { return nil, s.err }

func TestSetSummariesDedupAndUnsynced(t *testing.T) {
	a := writeDirCatalog(t, "shared", "only-a")
	b := writeDirCatalog(t, "shared", "only-b")
	s := NewSet(
		Entry{Name: "a", Catalog: NewDir(a)},
		Entry{Name: "stale", Catalog: stubCatalog{err: ocix.ErrNotSynced}},
		Entry{Name: "b", Catalog: NewDir(b)},
	)
	hits, unsynced, err := s.Summaries(context.Background())
	if err != nil {
		t.Fatalf("Summaries: %v", err)
	}
	if !slices.Equal(unsynced, []string{"stale"}) {
		t.Fatalf("unsynced: %v", unsynced)
	}
	got := map[string]string{}
	for _, h := range hits {
		got[h.Name] = h.Catalog
	}
	want := map[string]string{"shared": "a", "only-a": "a", "only-b": "b"}
	if !maps.Equal(got, want) {
		t.Fatalf("hits: %v, want %v", got, want)
	}
}

func TestSetSummariesOtherErrorAborts(t *testing.T) {
	boom := errors.New("boom")
	s := NewSet(Entry{Name: "bad", Catalog: stubCatalog{err: boom}})
	if _, _, err := s.Summaries(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
}

func TestSetPackageNamesBestEffortUnion(t *testing.T) {
	a := writeDirCatalog(t, "shared", "only-a")
	b := writeDirCatalog(t, "shared", "only-b")
	s := NewSet(
		Entry{Name: "a", Catalog: NewDir(a)},
		Entry{Name: "bad", Catalog: stubCatalog{err: errors.New("boom")}},
		Entry{Name: "b", Catalog: NewDir(b)},
	)
	got := s.PackageNames(context.Background())
	slices.Sort(got)
	if !slices.Equal(got, []string{"only-a", "only-b", "shared"}) {
		t.Fatalf("names: %v", got)
	}
}
