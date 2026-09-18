package catalog

import (
	"context"
	"errors"
	"slices"

	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/project"
	"github.com/vi-dev/nem/internal/spec"
)

type Entry struct {
	Name    string
	Ref     string
	Catalog Catalog
}

type Set struct{ entries []Entry }

func NewSet(entries ...Entry) *Set { return &Set{entries: entries} }

func (s *Set) Entries() []Entry { return slices.Clone(s.entries) }

func (s *Set) Find(name string) (Entry, bool) {
	for _, e := range s.entries {
		if e.Name == name {
			return e, true
		}
	}
	return Entry{}, false
}

func (s *Set) Prepend(e Entry) *Set {
	entries := make([]Entry, 0, len(s.entries)+1)
	entries = append(entries, e)
	return &Set{entries: append(entries, s.entries...)}
}

func (s *Set) Append(e Entry) *Set {
	entries := make([]Entry, 0, len(s.entries)+1)
	entries = append(entries, s.entries...)
	return &Set{entries: append(entries, e)}
}

type Hit struct {
	Pkg    *spec.Package
	Digest string
	Entry  Entry
}

func (s *Set) Lookup(ctx context.Context, key project.ToolKey) (Hit, error) {
	if key.Catalog != "" {
		e, ok := s.Find(key.Catalog)
		if !ok {
			return Hit{}, &NotFoundError{Name: key.Catalog}
		}
		pkg, dig, err := e.Catalog.Package(ctx, key.Name)
		if nf, ok := errors.AsType[*PackageNotFoundError](err); ok {
			nf.Catalogs = []string{e.Name}
			return Hit{}, nf
		}
		if err != nil {
			return Hit{}, err
		}
		return Hit{Pkg: pkg, Digest: dig, Entry: e}, nil
	}
	searched := make([]string, 0, len(s.entries))
	var notSynced []string
	for _, e := range s.entries {
		searched = append(searched, e.Name)
		pkg, dig, err := e.Catalog.Package(ctx, key.Name)
		if err != nil {
			if _, ok := errors.AsType[*PackageNotFoundError](err); ok {
				continue
			}
			if errors.Is(err, ocix.ErrNotSynced) {
				notSynced = append(notSynced, e.Name)
				continue
			}
			return Hit{}, err
		}
		return Hit{Pkg: pkg, Digest: dig, Entry: e}, nil
	}
	if len(notSynced) > 0 {
		return Hit{}, &NotSyncedError{Name: key.Name, Catalogs: notSynced}
	}
	return Hit{}, &PackageNotFoundError{Name: key.Name, Catalogs: searched}
}

type SetSummary struct {
	Summary
	Catalog string
}

func (s *Set) Summaries(ctx context.Context) (hits []SetSummary, unsynced []string, err error) {
	seen := map[string]bool{}
	for _, e := range s.entries {
		summaries, err := e.Catalog.Summaries(ctx)
		if err != nil {
			if errors.Is(err, ocix.ErrNotSynced) {
				unsynced = append(unsynced, e.Name)
				continue
			}
			return nil, nil, err
		}
		for _, sum := range summaries {
			if seen[sum.Name] {
				continue
			}
			seen[sum.Name] = true
			hits = append(hits, SetSummary{Summary: sum, Catalog: e.Name})
		}
	}
	return hits, unsynced, nil
}

func (s *Set) PackageNames(ctx context.Context) []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range s.entries {
		names, err := e.Catalog.PackageNames(ctx)
		if err != nil {
			continue
		}
		for _, n := range names {
			if seen[n] {
				continue
			}
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}
