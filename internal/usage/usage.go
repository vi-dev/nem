package usage

import (
	"encoding/json"
	"os"
	"time"

	"github.com/vi-dev/nem/internal/fsx"
	"github.com/vi-dev/nem/internal/home"
)

const Debounce = time.Hour

type Index map[string]time.Time

func Key(name, version string) string { return name + "@" + version }

func Load(h home.Home) Index {
	idx, err := Read(h)
	if err != nil {
		return Index{}
	}
	return idx
}

func Read(h home.Home) (Index, error) {
	data, err := os.ReadFile(h.Usage())
	if err != nil {
		if os.IsNotExist(err) {
			return Index{}, nil
		}
		return Index{}, err
	}
	idx := Index{}
	if err := json.Unmarshal(data, &idx); err != nil {
		return Index{}, err
	}
	return idx, nil
}

func (i Index) LastUsed(name, version string) (time.Time, bool) {
	t, ok := i[Key(name, version)]
	return t, ok
}

func (i Index) ByKey(key string) (time.Time, bool) {
	t, ok := i[key]
	return t, ok
}

func (i Index) Prune(existing []string) Index {
	keep := make(map[string]bool, len(existing))
	for _, k := range existing {
		keep[k] = true
	}
	out := make(Index, len(existing))
	for k, v := range i {
		if keep[k] {
			out[k] = v
		}
	}
	return out
}

func Save(h home.Home, idx Index) error {
	data, err := json.Marshal(idx)
	if err != nil {
		return err
	}
	return fsx.WriteAtomic(h.Usage(), data, 0o644)
}

func Stamp(h home.Home, now time.Time, keys []string) {
	if len(keys) == 0 {
		return
	}
	idx := Load(h)
	changed := false
	for _, k := range keys {
		if last, ok := idx[k]; ok && now.Sub(last) < Debounce {
			continue
		}
		idx[k] = now
		changed = true
	}
	if !changed {
		return
	}
	_ = Save(h, idx)
}
