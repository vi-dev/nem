package archive_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/vi-dev/clix/console"
	"github.com/vi-dev/clix/console/consoletest"

	"github.com/vi-dev/nem/internal/archive"
)

func TestProgressBlobReportsBytesRead(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 100_000)
	_, rep := consoletest.Context(context.Background())
	task := rep.Start("Pushing x 1.0.0")
	task.Phase("uploading")

	blob := archive.ProgressBlob(archive.BytesBlob(data), task)
	if blob.Digest != archive.BytesBlob(data).Digest || blob.Size != int64(len(data)) {
		t.Fatalf("blob = %+v, want the wrapped blob's digest and size", blob)
	}
	rc, err := blob.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()
	if _, err := io.Copy(io.Discard, rc); err != nil {
		t.Fatalf("read: %v", err)
	}

	calls := rep.TaskFor("Pushing x 1.0.0").ProgressCalls()
	if len(calls) == 0 {
		t.Fatal("no progress calls")
	}
	for i := 1; i < len(calls); i++ {
		if calls[i].Done < calls[i-1].Done {
			t.Fatalf("progress went backwards: %d then %d", calls[i-1].Done, calls[i].Done)
		}
	}
	last := calls[len(calls)-1]
	if last.Unit != console.Bytes || last.Done != int64(len(data)) || last.Total != int64(len(data)) {
		t.Fatalf("last progress = %+v, want %d/%d bytes", last, len(data), len(data))
	}
}

type closeRecorder struct {
	io.Reader
	closed bool
}

func (c *closeRecorder) Close() error {
	c.closed = true
	return nil
}

func TestProgressBlobClosesTheWrappedReader(t *testing.T) {
	rec := &closeRecorder{Reader: bytes.NewReader([]byte("abc"))}
	blob := archive.ProgressBlob(archive.Blob{Size: 3, Open: func() (io.ReadCloser, error) { return rec, nil }}, nil)
	rc, err := blob.Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := io.Copy(io.Discard, rc); err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := rc.Close(); err != nil || !rec.closed {
		t.Fatalf("Close = %v, closed = %v, want the wrapped reader closed", err, rec.closed)
	}
}
