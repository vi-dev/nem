package ocix_test

import (
	"fmt"
	"sync"

	"github.com/vi-dev/nem/internal/ocix/ocixtest"
	"github.com/vi-dev/nem/internal/report"
)

type progressCall struct{ done, total int64 }

type progressRecorder struct {
	mu    sync.Mutex
	calls []progressCall
}

func (r *progressRecorder) record(done, total int64, _ report.Unit) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, progressCall{done, total})
}

func (r *progressRecorder) snapshot() []progressCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]progressCall(nil), r.calls...)
}

func fakeCatalogEntries(n int) []ocixtest.FakeEntry {
	entries := make([]ocixtest.FakeEntry, n)
	for i := range entries {
		name := fmt.Sprintf("pkg%02d", i)
		entries[i] = ocixtest.FakeEntry{Name: name, YAML: []byte("name: " + name)}
	}
	return entries
}
