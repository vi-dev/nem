package install_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"

	"github.com/vi-dev/nem/internal/install"
	"github.com/vi-dev/nem/internal/spec"
)

type tarEntry struct {
	name    string
	content []byte
	mode    int64
}

func buildTar(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, e := range entries {
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}
		hdr := &tar.Header{
			Name:     e.name,
			Typeflag: tar.TypeReg,
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

func extractPkg(strip int) *spec.Package {
	return pkgWith(spec.Action{Extract: &spec.ExtractAction{Strip: strip}})
}

func stagingFor(t *testing.T, archive []byte) (staging, artifact string) {
	t.Helper()
	base := t.TempDir()
	container := filepath.Join(base, "container")
	staging = filepath.Join(container, "staging")
	if err := os.MkdirAll(staging, 0o755); err != nil {
		t.Fatalf("mkdir staging: %v", err)
	}
	return staging, writeArtifact(t, container, archive)
}

func TestRunActionsExtractTarGzAppliesStrip(t *testing.T) {
	entries := []tarEntry{
		{name: "pkg-1.0/bin/tool", content: []byte("binary"), mode: 0o755},
	}
	staging, artifact := stagingFor(t, gzipBytes(t, buildTar(t, entries)))

	if err := install.RunActions(extractPkg(1), staging, artifact, "v1", spec.Current()); err != nil {
		t.Fatalf("RunActions: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(staging, "bin", "tool"))
	if err != nil {
		t.Fatalf("read tool at stripped path: %v", err)
	}
	if string(got) != "binary" {
		t.Fatalf("tool content = %q, want %q", got, "binary")
	}
}

func TestRunActionsExtractSingleFileNamedFromArtifact(t *testing.T) {
	tests := []struct {
		name        string
		archive     []byte
		artifact    spec.Artifact
		wantName    string
		wantContent string
		wantMode    os.FileMode
	}{
		{
			name:        "gz named from URL",
			archive:     gzipBytes(t, []byte("#!/bin/sh\necho hi\n")),
			artifact:    spec.Artifact{URL: "https://ex.com/dl/tool-{{.Version}}.gz?token=x"},
			wantName:    "tool-v1",
			wantContent: "#!/bin/sh\necho hi\n",
			wantMode:    0o755,
		},
		{
			name:        "zstd named from GitHub asset",
			archive:     zstdBytes(t, []byte("zstd-binary")),
			artifact:    spec.Artifact{GitHub: &spec.GitHubAsset{Repo: "o/r", Asset: "tool_{{.Version}}.zst"}},
			wantName:    "tool_v1",
			wantContent: "zstd-binary",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			staging, artifact := stagingFor(t, tc.archive)

			pkg := extractPkg(0)
			pkg.Artifact = tc.artifact
			if err := install.RunActions(pkg, staging, artifact, "v1", spec.Current()); err != nil {
				t.Fatalf("RunActions: %v", err)
			}

			out := filepath.Join(staging, tc.wantName)
			got, err := os.ReadFile(out)
			if err != nil || string(got) != tc.wantContent {
				t.Fatalf("%s = %q, %v, want %q", tc.wantName, got, err, tc.wantContent)
			}
			if tc.wantMode != 0 {
				info, err := os.Stat(out)
				if err != nil || info.Mode().Perm() != tc.wantMode {
					t.Fatalf("%s mode = %v, %v, want %v", tc.wantName, info.Mode().Perm(), err, tc.wantMode)
				}
			}
		})
	}
}

func TestRunActionsExtractSingleFileWithoutArtifactNameErrors(t *testing.T) {
	staging, artifact := stagingFor(t, gzipBytes(t, []byte("just bytes")))

	err := install.RunActions(extractPkg(0), staging, artifact, "v1", spec.Current())
	if err == nil || !strings.Contains(err.Error(), "single file") {
		t.Fatalf("error = %v, want single-file naming error", err)
	}
}
