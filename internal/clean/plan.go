package clean

import (
	"fmt"
	"time"

	"github.com/vi-dev/nem/internal/usage"
)

type Item struct {
	Path   string
	Newest time.Time
	Size   int64
}

type Version struct {
	Name, Version, Path string
	Size                int64
}

type Store struct {
	Staging   []Item
	Downloads []Item
	Partials  []Item
	Versions  []Version

	TestInstalls []Item
}

type Options struct {
	Grace  time.Duration
	Unused time.Duration
	All    bool
}

type Entry struct {
	Path    string
	Reason  string
	Size    int64
	Key     string
	Stamp   time.Time
	Recheck bool
}

type Plan struct {
	Entries []Entry
	Confirm bool
}

func (p Plan) Total() int64 {
	var n int64
	for _, e := range p.Entries {
		n += e.Size
	}
	return n
}

func Build(s Store, idx usage.Index, opts Options, now time.Time) Plan {
	var p Plan

	for _, group := range []struct {
		items  []Item
		reason string
	}{
		{items: s.Staging, reason: "leaked staging"},
		{items: s.Downloads, reason: "leaked download"},
		{items: s.Partials, reason: "partial install"},
		{items: s.TestInstalls, reason: "leftover test install"},
	} {
		for _, it := range group.items {
			if now.Sub(it.Newest) < opts.Grace {
				continue
			}
			p.Entries = append(p.Entries, Entry{
				Path:   it.Path,
				Reason: group.reason,
				Size:   it.Size,
			})
		}
	}

	switch {
	case opts.All:
		for _, v := range s.Versions {
			p.Entries = append(p.Entries, Entry{
				Path:   v.Path,
				Reason: "--all",
				Size:   v.Size,
				Key:    usage.Key(v.Name, v.Version),
			})
			p.Confirm = true
		}
	case opts.Unused > 0:
		for _, v := range s.Versions {
			last, ok := idx.LastUsed(v.Name, v.Version)
			if !ok {
				continue
			}
			age := now.Sub(last)
			if age < opts.Unused {
				continue
			}
			p.Entries = append(p.Entries, Entry{
				Path:    v.Path,
				Reason:  unusedReason(age),
				Size:    v.Size,
				Key:     usage.Key(v.Name, v.Version),
				Stamp:   last,
				Recheck: true,
			})
			p.Confirm = true
		}
	}
	return p
}

func unusedReason(age time.Duration) string {
	if age >= 24*time.Hour {
		return fmt.Sprintf("unused %dd", int(age.Hours())/24)
	}
	return fmt.Sprintf("unused %dh", int(age.Hours()))
}
