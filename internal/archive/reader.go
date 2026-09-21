package archive

import (
	"errors"
	"io"

	"oras.land/oras-go/v2/content"
)

type verifyReadCloser struct {
	verify *content.VerifyReader
	rc     io.ReadCloser
}

func (v *verifyReadCloser) Read(p []byte) (int, error) {
	n, err := v.verify.Read(p)
	if errors.Is(err, io.EOF) {
		if verr := v.verify.Verify(); verr != nil {
			return n, verr
		}
	}
	return n, err
}

func (v *verifyReadCloser) Close() error { return v.rc.Close() }
