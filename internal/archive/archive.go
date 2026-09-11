package archive

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

type Options struct {
	Strip int

	SingleName string
}

type Result struct {
	CommonPrefix string
}

var (
	gzipMagic  = []byte{0x1f, 0x8b}
	xzMagic    = []byte{0xfd, 0x37, 0x7a, 0x58, 0x5a, 0x00}
	zstdMagic  = []byte{0x28, 0xb5, 0x2f, 0xfd}
	bzip2Magic = []byte{0x42, 0x5a, 0x68}

	zipLocalFileMagic = []byte{0x50, 0x4b, 0x03, 0x04}
	zipEmptyMagic     = []byte{0x50, 0x4b, 0x05, 0x06}
	zipSpannedMagic   = []byte{0x50, 0x4b, 0x07, 0x08}
)

const tarMagicOffset = 257

const tarHeaderLen = 512

func Extract(artifactPath string, root *os.Root, opts Options) (Result, error) {
	f, err := os.Open(artifactPath)
	if err != nil {
		return Result{}, fmt.Errorf("open artifact: %w", err)
	}
	defer f.Close()

	br := bufio.NewReader(f)
	peek, err := br.Peek(tarHeaderLen)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
		return Result{}, fmt.Errorf("read artifact: %w", err)
	}

	switch {
	case bytes.HasPrefix(peek, gzipMagic):
		gr, err := gzip.NewReader(br)
		if err != nil {
			return Result{}, fmt.Errorf("open gzip stream: %w", err)
		}
		defer gr.Close()
		return extractDecompressed(gr, root, opts)
	case bytes.HasPrefix(peek, xzMagic):
		xr, err := xz.NewReader(br)
		if err != nil {
			return Result{}, fmt.Errorf("open xz stream: %w", err)
		}
		return extractDecompressed(xr, root, opts)
	case bytes.HasPrefix(peek, zstdMagic):
		zr, err := zstd.NewReader(br)
		if err != nil {
			return Result{}, fmt.Errorf("open zstd stream: %w", err)
		}
		defer zr.Close()
		return extractDecompressed(zr, root, opts)
	case bytes.HasPrefix(peek, bzip2Magic):
		return extractDecompressed(bzip2.NewReader(br), root, opts)
	case isZip(peek):
		info, err := f.Stat()
		if err != nil {
			return Result{}, fmt.Errorf("stat artifact: %w", err)
		}
		zr, err := zip.NewReader(f, info.Size())
		if err != nil {
			return Result{}, fmt.Errorf("open zip archive: %w", err)
		}
		return extractZip(zr, root, opts)
	case looksLikeTar(peek):
		return extractTar(tar.NewReader(br), root, opts)
	default:
		info, err := f.Stat()
		if err != nil {
			return Result{}, fmt.Errorf("stat artifact: %w", err)
		}
		zr, err := zip.NewReader(f, info.Size())
		if err != nil {
			return Result{}, errors.New("unrecognized archive format")
		}
		return extractZip(zr, root, opts)
	}
}

func extractDecompressed(r io.Reader, root *os.Root, opts Options) (Result, error) {
	br := bufio.NewReaderSize(r, tarHeaderLen)
	peek, err := br.Peek(tarHeaderLen)
	if err != nil && !errors.Is(err, io.EOF) {
		return Result{}, fmt.Errorf("read decompressed stream: %w", err)
	}
	if looksLikeTar(peek) {
		return extractTar(tar.NewReader(br), root, opts)
	}
	return extractSingleFile(br, root, opts.SingleName)
}

func extractSingleFile(r io.Reader, root *os.Root, name string) (Result, error) {
	if name == "" {
		return Result{}, errors.New("artifact decompresses to a single file, and no output name is available")
	}
	rel := filepath.FromSlash(name)
	if !filepath.IsLocal(rel) {
		return Result{}, fmt.Errorf("single-file name %q escapes extraction root", name)
	}
	if err := mkdirParent(root, rel); err != nil {
		return Result{}, err
	}

	if err := writeFile(root, rel, r, 0o755); err != nil {
		return Result{}, err
	}
	return Result{}, nil
}

func isZip(data []byte) bool {
	return bytes.HasPrefix(data, zipLocalFileMagic) ||
		bytes.HasPrefix(data, zipEmptyMagic) ||
		bytes.HasPrefix(data, zipSpannedMagic)
}

func looksLikeTar(block []byte) bool {
	if len(block) < tarHeaderLen {
		return false
	}
	if string(block[tarMagicOffset:tarMagicOffset+5]) == "ustar" {
		return true
	}
	return validTarChecksum(block)
}

func validTarChecksum(block []byte) bool {
	field := strings.Trim(string(block[148:156]), " \x00")
	want, err := strconv.ParseInt(field, 8, 64)
	if err != nil || want <= 0 {
		return false
	}
	var unsigned, signed int64
	for i, b := range block[:tarHeaderLen] {
		if i >= 148 && i < 156 {
			b = ' '
		}
		unsigned += int64(b)
		signed += int64(int8(b))
	}
	return want == unsigned || want == signed
}
