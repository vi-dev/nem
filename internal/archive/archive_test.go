package archive_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"

	"github.com/vi-dev/nem/internal/archive"
)

type tarEntry struct {
	name       string
	typeflag   byte
	content    []byte
	linkname   string
	mode       int64
	paxRecords map[string]string
}

func buildTar(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		typeflag := e.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		if typeflag == tar.TypeXGlobalHeader {

			if err := tw.WriteHeader(&tar.Header{Typeflag: typeflag, PAXRecords: e.paxRecords}); err != nil {
				t.Fatalf("write tar header %s: %v", e.name, err)
			}
			continue
		}
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}
		hdr := &tar.Header{
			Name:     e.name,
			Typeflag: typeflag,
			Linkname: e.linkname,
			Mode:     mode,
			Size:     int64(len(e.content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write tar header %s: %v", e.name, err)
		}
		if len(e.content) > 0 {
			if _, err := tw.Write(e.content); err != nil {
				t.Fatalf("write tar content %s: %v", e.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	return buf.Bytes()
}

func buildV7Tar(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var hdr [512]byte
	copy(hdr[0:], name)
	copy(hdr[100:], "0000644\x00")
	copy(hdr[108:], "0000000\x00")
	copy(hdr[116:], "0000000\x00")
	copy(hdr[124:], octal11(int64(len(content))))
	copy(hdr[136:], octal11(0))
	hdr[156] = '0'
	for i := 148; i < 156; i++ {
		hdr[i] = ' '
	}
	var sum int64
	for _, b := range hdr {
		sum += int64(b)
	}
	copy(hdr[148:], octal(sum, 6)+"\x00 ")

	var buf bytes.Buffer
	buf.Write(hdr[:])
	buf.Write(content)
	if pad := 512 - len(content)%512; pad != 512 {
		buf.Write(make([]byte, pad))
	}
	buf.Write(make([]byte, 1024))
	return buf.Bytes()
}

func octal(n int64, width int) string {
	s := ""
	for v := n; v > 0; v /= 8 {
		s = string(rune('0'+v%8)) + s
	}
	for len(s) < width {
		s = "0" + s
	}
	return s
}

func octal11(n int64) string { return octal(n, 11) + "\x00" }

func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write(data); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func xzBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	xw, err := xz.NewWriter(&buf)
	if err != nil {
		t.Fatalf("xz writer: %v", err)
	}
	if _, err := xw.Write(data); err != nil {
		t.Fatalf("xz write: %v", err)
	}
	if err := xw.Close(); err != nil {
		t.Fatalf("xz close: %v", err)
	}
	return buf.Bytes()
}

func zstdBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw, err := zstd.NewWriter(&buf)
	if err != nil {
		t.Fatalf("zstd writer: %v", err)
	}
	if _, err := zw.Write(data); err != nil {
		t.Fatalf("zstd write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zstd close: %v", err)
	}
	return buf.Bytes()
}

type zipEntry struct {
	name      string
	content   []byte
	mode      os.FileMode
	isDir     bool
	isSymlink bool
}

func buildZip(t *testing.T, entries []zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		name := e.name
		mode := e.mode
		switch {
		case e.isDir:
			if !strings.HasSuffix(name, "/") {
				name += "/"
			}
			if mode == 0 {
				mode = 0o755 | os.ModeDir
			}
		case e.isSymlink:
			mode = os.ModeSymlink | 0o777
		case mode == 0:
			mode = 0o644
		}
		fh := &zip.FileHeader{Name: name, Method: zip.Deflate}
		fh.SetMode(mode)
		w, err := zw.CreateHeader(fh)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if len(e.content) > 0 {
			if _, err := w.Write(e.content); err != nil {
				t.Fatalf("write zip entry %s: %v", name, err)
			}
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	return buf.Bytes()
}

func extractBytes(t *testing.T, data []byte, opts archive.Options) (string, archive.Result, error) {
	t.Helper()
	tmp := t.TempDir()
	artifact := filepath.Join(tmp, "artifact")
	if err := os.WriteFile(artifact, data, 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	dest := filepath.Join(tmp, "dest")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}
	root, err := os.OpenRoot(dest)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	defer root.Close()
	res, err := archive.Extract(artifact, root, opts)
	return dest, res, err
}

func mustFile(t *testing.T, path, content string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != content {
		t.Fatalf("%s = %q, %v, want %q", path, got, err, content)
	}
}

func mustNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to not exist, stat err = %v", path, err)
	}
}

func TestExtractTarGzStripModesSymlink(t *testing.T) {
	data := gzipBytes(t, buildTar(t, []tarEntry{
		{name: "pkg-1.0/", typeflag: tar.TypeDir, mode: 0o755},
		{name: "pkg-1.0/bin/tool", content: []byte("binary"), mode: 0o755},
		{name: "pkg-1.0/bin/tool-link", typeflag: tar.TypeSymlink, linkname: "tool"},
	}))

	dest, res, err := extractBytes(t, data, archive.Options{Strip: 1})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if res.CommonPrefix != "pkg-1.0" {
		t.Fatalf("CommonPrefix = %q, want %q", res.CommonPrefix, "pkg-1.0")
	}
	mustFile(t, filepath.Join(dest, "bin", "tool"), "binary")
	info, err := os.Stat(filepath.Join(dest, "bin", "tool"))
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("tool mode = %v, %v, want 0755", info.Mode().Perm(), err)
	}
	target, err := os.Readlink(filepath.Join(dest, "bin", "tool-link"))
	if err != nil || target != "tool" {
		t.Fatalf("symlink target = %q, %v, want %q", target, err, "tool")
	}
}

func TestExtractZip(t *testing.T) {
	data := buildZip(t, []zipEntry{
		{name: "src-1.0/file.txt", content: []byte("hi")},
		{name: "src-1.0/sub/other.txt", content: []byte("there"), mode: 0o755},
		{name: "src-1.0/link", isSymlink: true, content: []byte("file.txt")},
		{name: "src-1.0/emptydir", isDir: true},
	})

	dest, res, err := extractBytes(t, data, archive.Options{})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if res.CommonPrefix != "src-1.0" {
		t.Fatalf("CommonPrefix = %q, want %q", res.CommonPrefix, "src-1.0")
	}
	mustFile(t, filepath.Join(dest, "src-1.0", "file.txt"), "hi")
	mustFile(t, filepath.Join(dest, "src-1.0", "sub", "other.txt"), "there")
	target, err := os.Readlink(filepath.Join(dest, "src-1.0", "link"))
	if err != nil || target != "file.txt" {
		t.Fatalf("link target = %q, %v, want %q", target, err, "file.txt")
	}
	info, err := os.Stat(filepath.Join(dest, "src-1.0", "emptydir"))
	if err != nil || !info.IsDir() {
		t.Fatalf("emptydir stat = %v, %v, want a directory", info, err)
	}
}

func TestExtractPlainTarUstar(t *testing.T) {
	data := buildTar(t, []tarEntry{{name: "plain.txt", content: []byte("no compression")}})
	dest, _, err := extractBytes(t, data, archive.Options{})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	mustFile(t, filepath.Join(dest, "plain.txt"), "no compression")
}

func TestExtractPlainTarV7Checksum(t *testing.T) {
	data := buildV7Tar(t, "old.txt", []byte("pre-posix"))
	if bytes.Contains(data[:512], []byte("ustar")) {
		t.Fatal("fixture unexpectedly carries ustar magic, precondition broken")
	}
	dest, _, err := extractBytes(t, data, archive.Options{})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	mustFile(t, filepath.Join(dest, "old.txt"), "pre-posix")
}

func TestExtractTarXzAndZstd(t *testing.T) {
	tarBytes := buildTar(t, []tarEntry{{name: "bin/tool", content: []byte("payload"), mode: 0o755}})
	for name, data := range map[string][]byte{
		"xz":   xzBytes(t, tarBytes),
		"zstd": zstdBytes(t, tarBytes),
	} {
		dest, _, err := extractBytes(t, data, archive.Options{})
		if err != nil {
			t.Fatalf("Extract %s: %v", name, err)
		}
		mustFile(t, filepath.Join(dest, "bin", "tool"), "payload")
	}
}

const bzip2TarFixtureB64 = `QlpoOTFBWSZTWXOm2KsAAO1/sP+4I4BQCf/iOm//c+/v/0AAQo4wAAhAAhwMQBxkaZMTQZMmE0yBkNAaA0yaGAE0BhJI1R6CI0ABoAAAMg0yAAAAHGRpkxNBkyYTTIGQ0BoDTJoYATQGCpSTIiep6myanqb1TTEyaNDTQAGh6jR6mIYh6nqacC+yplT+PzPj0qa/6bmzLGumu1pcj8sLVfsqCvsqkm4owsi8uIOuoSpSSGZaqdpQTDi3W6rmMTnSpdSgOoURfUb7cKk6xdJU0lRoU3VK1rl42HgsJ7SjvM5UmJLqUTgSqlKKdZOJZO0ltry0xWXGo8zKpZYfVqqt7mTZNtOifZwvFLFOvMmATdx1BMBNlKJ8ceZx8lhSBMKakVwdMLKrxQRGFXcyYiqilcIzaxOUDC2GZxRFAgL8orZ/IzxL4VLCdMUJyzwFikqCZNUmYVgTSrKYiTiccklJQFRAVVpyMAcYVy5RdrmnW0thMLK+DiNZa97lcRwuRmZjY7K+qdJcb+/9n3Uft92leWLH0VuhfXWBYsN5RL/No3tTRztpypjTzp1UuJadDYeheTMvFxws6UcKjaa8xp/HMmkl9NpbbS3Txs6Xk/qa0xTKnG21E8SxNRNlKJemRNhOolaWp22d3VaYi+ol5M6dVNuaCbS6exNMuJgmCxNCVZEyLy6XZYc50lT/KI6SiP+LuSKcKEg502xVgA==`

func TestExtractTarBzip2(t *testing.T) {
	data, err := base64.StdEncoding.DecodeString(bzip2TarFixtureB64)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	dest, _, err := extractBytes(t, data, archive.Options{})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	mustFile(t, filepath.Join(dest, "a", "hello.txt"), "hi\n")
}

func TestExtractSingleFileGz(t *testing.T) {
	dest, res, err := extractBytes(t, gzipBytes(t, []byte("#!/bin/sh\necho hi\n")), archive.Options{SingleName: "tool"})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if res.CommonPrefix != "" {
		t.Fatalf("CommonPrefix = %q, want empty for single file", res.CommonPrefix)
	}
	out := filepath.Join(dest, "tool")
	mustFile(t, out, "#!/bin/sh\necho hi\n")
	info, err := os.Stat(out)
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %v, %v, want 0755", info.Mode().Perm(), err)
	}
}

func TestExtractSingleFileGzLarge(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	payload := make([]byte, 2<<20)
	if _, err := rng.Read(payload); err != nil {
		t.Fatalf("fill payload: %v", err)
	}
	dest, _, err := extractBytes(t, gzipBytes(t, payload), archive.Options{SingleName: "blob"})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "blob"))
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("blob mismatch (len %d vs %d), err %v", len(got), len(payload), err)
	}
}

func TestExtractSingleFileWithoutNameErrors(t *testing.T) {
	_, _, err := extractBytes(t, gzipBytes(t, []byte("just bytes")), archive.Options{})
	if err == nil || !strings.Contains(err.Error(), "single file") {
		t.Fatalf("error = %v, want single-file naming error", err)
	}
}

func TestExtractSingleFileIgnoresStrip(t *testing.T) {
	dest, _, err := extractBytes(t, gzipBytes(t, []byte("data")), archive.Options{Strip: 3, SingleName: "f"})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	mustFile(t, filepath.Join(dest, "f"), "data")
}

func TestExtractCommonPrefixMultipleRoots(t *testing.T) {
	data := gzipBytes(t, buildTar(t, []tarEntry{
		{name: "a/x", content: []byte("1")},
		{name: "b/y", content: []byte("2")},
	}))
	_, res, err := extractBytes(t, data, archive.Options{})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if res.CommonPrefix != "" {
		t.Fatalf("CommonPrefix = %q, want empty", res.CommonPrefix)
	}
}

func TestExtractCommonPrefixIgnoresPaxHeader(t *testing.T) {
	data := gzipBytes(t, buildTar(t, []tarEntry{
		{name: "pax_global_header", typeflag: tar.TypeXGlobalHeader, paxRecords: map[string]string{"comment": "x"}},
		{name: "src-1.0/main.c", content: []byte("int main(){}")},
	}))
	dest, res, err := extractBytes(t, data, archive.Options{})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if res.CommonPrefix != "src-1.0" {
		t.Fatalf("CommonPrefix = %q, want %q", res.CommonPrefix, "src-1.0")
	}
	mustFile(t, filepath.Join(dest, "src-1.0", "main.c"), "int main(){}")
}

func TestExtractEscapeEntryRejected(t *testing.T) {
	data := gzipBytes(t, buildTar(t, []tarEntry{{name: "../evil", content: []byte("pwned")}}))
	_, _, err := extractBytes(t, data, archive.Options{})
	if err == nil || !strings.Contains(err.Error(), "escapes extraction root") {
		t.Fatalf("error = %v, want containment error", err)
	}
}

func TestExtractZipEscapeEntryRejected(t *testing.T) {
	data := buildZip(t, []zipEntry{{name: "../evil", content: []byte("pwned")}})
	_, _, err := extractBytes(t, data, archive.Options{})
	if err == nil || !strings.Contains(err.Error(), "escapes extraction root") {
		t.Fatalf("error = %v, want containment error", err)
	}
}

func TestExtractAbsoluteSymlinkRejected(t *testing.T) {
	data := gzipBytes(t, buildTar(t, []tarEntry{
		{name: "link", typeflag: tar.TypeSymlink, linkname: "/etc/passwd"},
	}))
	_, _, err := extractBytes(t, data, archive.Options{})
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("error = %v, want absolute-target error", err)
	}
}

func TestExtractHardlinkRejected(t *testing.T) {
	data := gzipBytes(t, buildTar(t, []tarEntry{
		{name: "real", content: []byte("data")},
		{name: "hard", typeflag: tar.TypeLink, linkname: "real"},
	}))
	_, _, err := extractBytes(t, data, archive.Options{})
	if err == nil || !strings.Contains(err.Error(), "hardlinks are not supported") {
		t.Fatalf("error = %v, want hardlink error", err)
	}
}

func TestExtractEscapingRelativeSymlinkTargetRejected(t *testing.T) {
	tests := []struct {
		name    string
		entries []tarEntry
		link    string
	}{
		{
			name:    "nested link",
			entries: []tarEntry{{name: "a/link", typeflag: tar.TypeSymlink, linkname: "../../x"}},
			link:    "a/link",
		},
		{
			name: "parent target with entry through it",
			entries: []tarEntry{
				{name: "evil", typeflag: tar.TypeSymlink, linkname: ".."},
				{name: "evil/x", content: []byte("pwned")},
			},
			link: "evil",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dest, _, err := extractBytes(t, gzipBytes(t, buildTar(t, tc.entries)), archive.Options{})
			if err == nil || !strings.Contains(err.Error(), "escapes extraction root") {
				t.Fatalf("error = %v, want containment error", err)
			}
			mustNotExist(t, filepath.Join(dest, tc.link))
			mustNotExist(t, filepath.Join(filepath.Dir(dest), "x"))
		})
	}
}

func TestExtractSymlinkChainEscapeRejected(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		preDir   string
		notExist []string
	}{
		{
			name: "tar",
			data: gzipBytes(t, buildTar(t, []tarEntry{
				{name: "y", typeflag: tar.TypeDir},
				{name: "x/a", typeflag: tar.TypeSymlink, linkname: "../y"},
				{name: "x/a/evil", typeflag: tar.TypeSymlink, linkname: "../../pwned"},
				{name: "x/a/evil", content: []byte("pwned"), mode: 0o755},
			})),
			notExist: []string{"container/pwned", "pwned"},
		},
		{
			name: "zip",
			data: buildZip(t, []zipEntry{
				{name: "y", isDir: true},
				{name: "x/a", isSymlink: true, content: []byte("../y")},
				{name: "x/a/evil", isSymlink: true, content: []byte("../../pwned")},
				{name: "x/a/evil", content: []byte("pwned")},
			}),
			notExist: []string{"container/pwned", "pwned"},
		},
		{
			name: "two hop",
			data: gzipBytes(t, buildTar(t, []tarEntry{
				{name: "z", typeflag: tar.TypeDir},
				{name: "y4", typeflag: tar.TypeDir},
				{name: "y/y2", typeflag: tar.TypeSymlink, linkname: "../z"},
				{name: "y/y2/y3", typeflag: tar.TypeSymlink, linkname: "../y4"},
				{name: "y/y2/y3/evil", typeflag: tar.TypeSymlink, linkname: "../../../pwned"},
				{name: "y/y2/y3/evil", content: []byte("pwned"), mode: 0o644},
			})),
			notExist: []string{"container/pwned", "pwned"},
		},
		{
			name: "via nested entry",
			data: gzipBytes(t, buildTar(t, []tarEntry{
				{name: "y", typeflag: tar.TypeDir},
				{name: "x/a", typeflag: tar.TypeSymlink, linkname: "../y"},
				{name: "x/a/evil", typeflag: tar.TypeSymlink, linkname: "../../existing-outside"},
				{name: "x/a/evil/payload", content: []byte("pwned")},
			})),
			preDir:   "existing-outside",
			notExist: []string{"container/existing-outside/payload", "payload"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			container := filepath.Join(base, "container")
			dest := filepath.Join(container, "dest")
			if err := os.MkdirAll(dest, 0o755); err != nil {
				t.Fatalf("mkdir dest: %v", err)
			}
			artifact := filepath.Join(container, "artifact")
			if err := os.WriteFile(artifact, tc.data, 0o644); err != nil {
				t.Fatalf("write artifact: %v", err)
			}
			if tc.preDir != "" {
				if err := os.MkdirAll(filepath.Join(container, tc.preDir), 0o755); err != nil {
					t.Fatalf("pre-create outside dir: %v", err)
				}
			}
			root, err := os.OpenRoot(dest)
			if err != nil {
				t.Fatalf("open root: %v", err)
			}
			defer root.Close()

			if _, err := archive.Extract(artifact, root, archive.Options{}); err == nil {
				t.Fatal("expected error, got nil")
			}
			for _, p := range tc.notExist {
				mustNotExist(t, filepath.Join(base, p))
			}
		})
	}
}

func TestExtractUnsupportedTarTypeRejected(t *testing.T) {
	data := gzipBytes(t, buildTar(t, []tarEntry{{name: "dev", typeflag: tar.TypeChar}}))
	_, _, err := extractBytes(t, data, archive.Options{})
	if err == nil || !strings.Contains(err.Error(), "unsupported tar type") {
		t.Fatalf("error = %v, want unsupported-type error", err)
	}
}

func TestExtractStripDropsTopDirEntry(t *testing.T) {
	data := gzipBytes(t, buildTar(t, []tarEntry{
		{name: "top/", typeflag: tar.TypeDir},
		{name: "top/file", content: []byte("x")},
	}))
	dest, _, err := extractBytes(t, data, archive.Options{Strip: 1})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "file" {
		t.Fatalf("dest entries = %v, want exactly [file]", entries)
	}
}

func TestExtractPlainTarWithPKPrefixedFirstEntry(t *testing.T) {
	data := buildTar(t, []tarEntry{
		{name: "PKGBUILD", content: []byte("# a real tar entry, not a zip")},
	})
	if !bytes.HasPrefix(data, []byte("PK")) {
		t.Fatalf("test fixture doesn't actually start with PK, precondition broken")
	}
	dest, _, err := extractBytes(t, data, archive.Options{})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	mustFile(t, filepath.Join(dest, "PKGBUILD"), "# a real tar entry, not a zip")
}

func TestExtractNegativeStripNoPanic(t *testing.T) {
	data := gzipBytes(t, buildTar(t, []tarEntry{{name: "a/b/file", content: []byte("x")}}))
	dest, _, err := extractBytes(t, data, archive.Options{Strip: -1})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	mustFile(t, filepath.Join(dest, "a", "b", "file"), "x")
}

func TestSingleNameFromRef(t *testing.T) {
	cases := map[string]string{
		"https://ex.com/dl/tool-1.0.gz?token=x": "tool-1.0",
		"tool_v1.zst":                           "tool_v1",
		"https://ex.com/src.tar.gz":             "src.tar",
		"plain-name":                            "plain-name",
		".gz":                                   "",
		"":                                      "",
		"https://ex.com/":                       "",
	}
	for ref, want := range cases {
		if got := archive.SingleNameFromRef(ref); got != want {
			t.Errorf("SingleNameFromRef(%q) = %q, want %q", ref, got, want)
		}
	}
}

func TestExtractUnrecognizedFormatErrors(t *testing.T) {
	_, _, err := extractBytes(t, []byte("not an archive at all"), archive.Options{})
	if err == nil || !strings.Contains(err.Error(), "unrecognized archive format") {
		t.Fatalf("error = %v, want unrecognized-format error", err)
	}
}
