package build

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/vi-dev/nem/internal/fetch"

	"github.com/vi-dev/nem/internal/testx"
)

func serve(t *testing.T, body []byte) (*httptest.Server, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	t.Cleanup(srv.Close)
	sum := sha256.Sum256(body)
	return srv, hex.EncodeToString(sum[:])
}

func makeTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	return testx.TarGz(t, files, 0o644)
}

func TestFetchSourceVerifyAndTOFU(t *testing.T) {
	body := []byte("tarball-bytes")
	srv, want := serve(t, body)
	dir := t.TempDir()
	meta := fetch.Meta{Name: "a", Version: "v1"}

	p, sha, verified, err := fetchSource(context.Background(), http.DefaultClient, srv.URL, want, dir, meta)
	if err != nil || !verified || sha != want {
		t.Fatalf("verify path: p=%q sha=%q verified=%v err=%v", p, sha, verified, err)
	}

	_, sha2, verified2, err := fetchSource(context.Background(), http.DefaultClient, srv.URL, "", dir, meta)
	if err != nil || verified2 || sha2 != want {
		t.Fatalf("tofu path: sha=%q verified=%v err=%v", sha2, verified2, err)
	}

}

func TestUnpackSourceStripsSingleRoot(t *testing.T) {
	tgz := makeTarGz(t, map[string]string{"src-1.0/configure": "#!/bin/sh\n", "src-1.0/main.c": "x"})
	arc := filepath.Join(t.TempDir(), "s.tar.gz")
	os.WriteFile(arc, tgz, 0o644)
	root, err := unpackSource(arc, t.TempDir(), "")
	if err != nil {
		t.Fatalf("unpackSource: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "configure")); err != nil {
		t.Fatalf("expected configure at stripped root: %v", err)
	}
}

func TestUnpackSourceSingleFile(t *testing.T) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write([]byte("int main(){}\n")); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	arc := filepath.Join(t.TempDir(), "main.c.gz")
	if err := os.WriteFile(arc, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	dest := t.TempDir()
	root, err := unpackSource(arc, dest, "main.c")
	if err != nil {
		t.Fatalf("unpackSource: %v", err)
	}
	if root != dest {
		t.Fatalf("root = %q, want dest dir %q", root, dest)
	}
	got, err := os.ReadFile(filepath.Join(dest, "main.c"))
	if err != nil || string(got) != "int main(){}\n" {
		t.Fatalf("main.c = %q, %v", got, err)
	}
}
