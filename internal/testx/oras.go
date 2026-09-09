package testx

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"
)

type CountingTarget struct {
	oras.ReadOnlyTarget
	Resolves atomic.Int64
	Fetches  atomic.Int64
}

func (c *CountingTarget) Resolve(ctx context.Context, ref string) (ocispec.Descriptor, error) {
	c.Resolves.Add(1)
	return c.ReadOnlyTarget.Resolve(ctx, ref)
}

func (c *CountingTarget) Fetch(ctx context.Context, d ocispec.Descriptor) (io.ReadCloser, error) {
	c.Fetches.Add(1)
	return c.ReadOnlyTarget.Fetch(ctx, d)
}

type RejectPushTarget struct {
	oras.Target
}

func (r *RejectPushTarget) Push(context.Context, ocispec.Descriptor, io.Reader) error {
	return fmt.Errorf("push rejected")
}

type CancelOnPushTarget struct {
	oras.Target
	Cancel context.CancelFunc
	fired  atomic.Bool
}

func (c *CancelOnPushTarget) Push(context.Context, ocispec.Descriptor, io.Reader) error {
	if c.fired.CompareAndSwap(false, true) {
		c.Cancel()
		return context.Canceled
	}
	return nil
}

type IndexPushCountingTarget struct {
	oras.Target
	IndexPushes atomic.Int64
	Tags        atomic.Int64
}

func (c *IndexPushCountingTarget) Push(ctx context.Context, d ocispec.Descriptor, r io.Reader) error {
	if d.MediaType == ocispec.MediaTypeImageIndex {
		c.IndexPushes.Add(1)
	}
	return c.Target.Push(ctx, d, r)
}

func (c *IndexPushCountingTarget) Tag(ctx context.Context, d ocispec.Descriptor, ref string) error {
	c.Tags.Add(1)
	return c.Target.Tag(ctx, d, ref)
}

type ArchiveFixtures struct {
	mu     sync.Mutex
	stores map[string]oras.Target
	opened []string
}

func NewArchiveFixtures() *ArchiveFixtures {
	return &ArchiveFixtures{stores: map[string]oras.Target{}}
}

func (f *ArchiveFixtures) Set(name string, target oras.Target) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stores[name] = target
}

func (f *ArchiveFixtures) Open(name string) oras.Target {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opened = append(f.opened, name)
	if s, ok := f.stores[name]; ok {
		return s
	}
	s := memory.New()
	f.stores[name] = s
	return s
}

func (f *ArchiveFixtures) OpenedNames() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.opened...)
}

type FileServer struct {
	*httptest.Server
	mu    sync.Mutex
	files map[string][]byte
	hits  atomic.Int64
}

func NewFileServer(t testing.TB) *FileServer {
	t.Helper()
	u := &FileServer{files: map[string][]byte{}}
	u.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.hits.Add(1)
		u.mu.Lock()
		body, ok := u.files[r.URL.Path]
		u.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(u.Close)
	return u
}

func (u *FileServer) Set(path string, body []byte) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.files[path] = body
}

func (u *FileServer) Hits() int64 { return u.hits.Load() }
