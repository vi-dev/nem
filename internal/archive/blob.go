package archive

import (
	"bytes"
	"io"
	"os"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/vi-dev/nem/internal/report"
)

type Blob struct {
	Digest digest.Digest
	Size   int64
	Open   func() (io.ReadCloser, error)
}

func BytesBlob(data []byte) Blob {
	return Blob{
		Digest: digest.FromBytes(data),
		Size:   int64(len(data)),
		Open: func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(data)), nil
		},
	}
}

func FileBlob(path, sha256Hex string, size int64) Blob {
	return Blob{
		Digest: digest.NewDigestFromEncoded(digest.SHA256, sha256Hex),
		Size:   size,
		Open: func() (io.ReadCloser, error) {
			return os.Open(path)
		},
	}
}

func (b Blob) descriptor() ocispec.Descriptor {
	return ocispec.Descriptor{MediaType: MediaType, Digest: b.Digest, Size: b.Size}
}

func ProgressBlob(b Blob, task report.Task) Blob {
	open := b.Open
	b.Open = func() (io.ReadCloser, error) {
		rc, err := open()
		if err != nil {
			return nil, err
		}
		return &progressReader{ReadCloser: rc, total: b.Size, task: task}, nil
	}
	return b
}

type progressReader struct {
	io.ReadCloser
	sent  int64
	total int64
	task  report.Task
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	r.sent += int64(n)
	r.task.Progress(r.sent, r.total, report.Bytes)
	return n, err
}
