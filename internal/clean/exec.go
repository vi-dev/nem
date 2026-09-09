package clean

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/vi-dev/nem/internal/fsx"
	"github.com/vi-dev/nem/internal/home"
	"github.com/vi-dev/nem/internal/usage"
)

type Observer struct {
	Removing func(entry Entry)
	Skipped  func(entry Entry, reason string)
}

func (o Observer) removing(e Entry) {
	if o.Removing != nil {
		o.Removing(e)
	}
}

func (o Observer) skipped(e Entry, reason string) {
	if o.Skipped != nil {
		o.Skipped(e, reason)
	}
}

func Execute(h home.Home, p Plan, obs Observer) (int64, error) {
	freed, deleted, unverifiable, err := removeEntries(h, p.Entries, obs)
	if len(deleted) > 0 && !unverifiable {
		pruneIndex(h, deleted)
	}
	return freed, err
}

func removeEntries(h home.Home, entries []Entry, obs Observer) (int64, map[string]bool, bool, error) {
	var freed int64
	deleted := map[string]bool{}
	unverifiable := false

	for _, e := range entries {
		if !withinRoot(h.Root(), e.Path) {
			obs.skipped(e, "outside NEM_HOME")
			continue
		}
		info, err := os.Lstat(e.Path)
		if os.IsNotExist(err) {
			if e.Key != "" {
				deleted[e.Key] = true
			}
			continue
		}
		if err != nil {
			obs.skipped(e, err.Error())
			return freed, deleted, unverifiable, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			obs.skipped(e, "symlink")
			continue
		}
		if e.Recheck {
			idx, err := usage.Read(h)
			if err != nil {
				unverifiable = true
				obs.skipped(e, "usage index unreadable")
				continue
			}
			if last, ok := idx.ByKey(e.Key); ok && !last.Equal(e.Stamp) {
				obs.skipped(e, "used since planning")
				continue
			}
		}
		obs.removing(e)
		if err := os.RemoveAll(e.Path); err != nil {
			return freed, deleted, unverifiable, err
		}
		freed += e.Size
		if e.Key != "" {
			deleted[e.Key] = true
		}
	}
	return freed, deleted, unverifiable, nil
}

func pruneIndex(h home.Home, deleted map[string]bool) {
	release, err := fsx.Lock(h.LockFile())
	if err != nil {
		return
	}
	defer release()

	idx := usage.Load(h)
	var surviving []string
	for k := range idx {
		if !deleted[k] {
			surviving = append(surviving, k)
		}
	}
	_ = usage.Save(h, idx.Prune(surviving))
}

func withinRoot(root, path string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	return path != root && strings.HasPrefix(path, root+string(filepath.Separator))
}
