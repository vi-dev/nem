package archive_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/vi-dev/nem/internal/archive"
	"github.com/vi-dev/nem/internal/report"
	"github.com/vi-dev/nem/internal/testx"
)

func TestProgressBlobReportsBytesRead(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 100_000)
	_, rep := testx.ReporterContext(context.Background())
	task := rep.Task("Pushing x 1.0.0")
	task.Segment("uploading")

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
	last := calls[len(calls)-1]
	if last.Unit != report.Bytes || last.Done != int64(len(data)) || last.Total != int64(len(data)) {
		t.Fatalf("last progress = %+v, want %d/%d bytes", last, len(data), len(data))
	}
}
