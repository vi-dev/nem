package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vi-dev/nem/internal/selfupdate"

	"github.com/vi-dev/nem/internal/testx"
)

func buildNemArchive(t *testing.T, script string) []byte {
	t.Helper()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	hdr := &tar.Header{Name: "nem", Mode: 0o755, Size: int64(len(script))}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write([]byte(script)); err != nil {
		t.Fatalf("write tar entry: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

func stageInstalledBinary(t *testing.T) string {
	t.Helper()

	exePath := filepath.Join(t.TempDir(), "nem")
	if err := os.WriteFile(exePath, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("seed exe: %v", err)
	}
	testx.Swap(t, &selfExecutablePath, func() (string, error) { return exePath, nil })
	return exePath
}

func releaseFixture(t *testing.T, tag, script string) string {
	t.Helper()

	archive := buildNemArchive(t, script)
	asset := fmt.Sprintf("nem_%s_%s_%s.tar.gz", tag, runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256(archive)
	checksums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), asset)

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/vi-dev/nem/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name": "` + tag + `"}`))
	})
	mux.HandleFunc("/download/"+tag+"/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(checksums))
	})
	mux.HandleFunc("/download/"+tag+"/"+asset, func(w http.ResponseWriter, r *http.Request) {
		w.Write(archive)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	withUpdater(t, &selfupdate.Updater{
		Client:       srv.Client(),
		APIBase:      srv.URL,
		DownloadBase: srv.URL + "/download",
		Repo:         "vi-dev/nem",
	})
	return stageInstalledBinary(t)
}

func unstableFixture(t *testing.T, script, sha string) string {
	t.Helper()

	archive := buildNemArchive(t, script)
	asset := fmt.Sprintf("nem_unstable_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256(archive)
	checksums := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), asset)

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/vi-dev/nem/commits/unstable", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"sha": "` + sha + `"}`))
	})
	mux.HandleFunc("/download/unstable/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(checksums))
	})
	mux.HandleFunc("/download/unstable/"+asset, func(w http.ResponseWriter, r *http.Request) {
		w.Write(archive)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	withUpdater(t, &selfupdate.Updater{
		Client:       srv.Client(),
		APIBase:      srv.URL,
		DownloadBase: srv.URL + "/download",
		Repo:         "vi-dev/nem",
	})
	return stageInstalledBinary(t)
}

func withVersion(t *testing.T, v string) {
	t.Helper()
	testx.Swap(t, &version, v)
}

func withChannel(t *testing.T, c string) {
	t.Helper()
	testx.Swap(t, &channel, c)
}

func withCurrentCommit(t *testing.T, sha string) {
	t.Helper()
	testx.Swap(t, &currentCommit, func() string { return sha })
}

func withUpdater(t *testing.T, u *selfupdate.Updater) {
	t.Helper()
	testx.Swap(t, &newSelfUpdater, func() *selfupdate.Updater { return u })
}

func latestReleaseAPI(t *testing.T, tag string) *selfupdate.Updater {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name": "` + tag + `"}`))
	}))
	t.Cleanup(srv.Close)
	return &selfupdate.Updater{Client: srv.Client(), APIBase: srv.URL, Repo: "vi-dev/nem"}
}

func TestSelfUpdateCheckUpdateAvailable(t *testing.T) {
	withVersion(t, "v1.0.0")
	withUpdater(t, latestReleaseAPI(t, "v1.1.0"))

	out, err := execNemMerged(t, t.TempDir(), "", "self", "update", "--check")
	if err != nil {
		t.Fatalf("self update --check: %v\n%s", err, out)
	}
	if !strings.Contains(out, "v1.0.0") || !strings.Contains(out, "v1.1.0") {
		t.Errorf("output does not name both versions:\n%s", out)
	}
}

func TestSelfUpdateCheckUpToDate(t *testing.T) {
	withVersion(t, "v1.1.0")
	withUpdater(t, latestReleaseAPI(t, "v1.1.0"))

	out, err := execNemMerged(t, t.TempDir(), "", "self", "update", "--check")
	if err != nil {
		t.Fatalf("self update --check: %v\n%s", err, out)
	}
	if !strings.Contains(out, "up to date") {
		t.Errorf("output missing up-to-date notice:\n%s", out)
	}
}

func TestSelfUpdateInstallsLatest(t *testing.T) {
	withVersion(t, "v1.0.0")
	script := "#!/bin/sh\nexit 0\n"
	exePath := releaseFixture(t, "v1.1.0", script)

	out, err := execNemMerged(t, t.TempDir(), "", "self", "update")
	if err != nil {
		t.Fatalf("self update: %v\n%s", err, out)
	}
	got, err := os.ReadFile(exePath)
	if err != nil {
		t.Fatalf("read updated binary: %v", err)
	}
	if string(got) != script {
		t.Errorf("binary content = %q, want the downloaded script", got)
	}
	if !strings.Contains(out, "https://github.com/vi-dev/nem/releases/tag/v1.1.0") {
		t.Errorf("output missing release notes link:\n%s", out)
	}
}

func TestSelfUpdateUnstableWarns(t *testing.T) {
	withVersion(t, "v1.0.0")
	script := "#!/bin/sh\nexit 0\n"
	exePath := releaseFixture(t, "unstable", script)

	out, err := execNemMerged(t, t.TempDir(), "", "self", "update", "--version", "unstable")
	if err != nil {
		t.Fatalf("self update --version unstable: %v\n%s", err, out)
	}
	if !strings.Contains(out, "WARN") || !strings.Contains(out, "unstable") {
		t.Errorf("output missing unstable warning:\n%s", out)
	}
	got, _ := os.ReadFile(exePath)
	if string(got) != script {
		t.Errorf("binary content = %q, want the downloaded script", got)
	}
	if strings.Contains(out, "releases/tag") {
		t.Errorf("unstable update should not link release notes:\n%s", out)
	}
}

func TestSelfUpdateUnstableChannelFollowsUnstable(t *testing.T) {
	withVersion(t, "v1.0.0-14-gabc1234")
	withChannel(t, "unstable")
	withCurrentCommit(t, strings.Repeat("a", 40))
	script := "#!/bin/sh\nexit 0\n"
	exePath := unstableFixture(t, script, strings.Repeat("b", 40))

	out, err := execNemMerged(t, t.TempDir(), "", "self", "update")
	if err != nil {
		t.Fatalf("self update: %v\n%s", err, out)
	}
	got, err := os.ReadFile(exePath)
	if err != nil {
		t.Fatalf("read updated binary: %v", err)
	}
	if string(got) != script {
		t.Errorf("binary content = %q, want the downloaded unstable build", got)
	}
	if !strings.Contains(out, "unstable channel") {
		t.Errorf("output does not name the unstable channel:\n%s", out)
	}
	if strings.Contains(out, "WARN") {
		t.Errorf("staying on the unstable channel should not warn:\n%s", out)
	}
	if strings.Contains(out, "releases/tag") {
		t.Errorf("unstable update should not link release notes:\n%s", out)
	}
}

func TestSelfUpdateUnstableChannelUpToDate(t *testing.T) {
	sha := strings.Repeat("c", 40)
	withVersion(t, "v1.0.0-14-gabc1234")
	withChannel(t, "unstable")
	withCurrentCommit(t, sha)
	exePath := unstableFixture(t, "#!/bin/sh\nexit 0\n", sha)

	out, err := execNemMerged(t, t.TempDir(), "", "self", "update")
	if err != nil {
		t.Fatalf("self update: %v\n%s", err, out)
	}
	if !strings.Contains(out, "up to date") {
		t.Errorf("output missing up-to-date notice:\n%s", out)
	}
	got, _ := os.ReadFile(exePath)
	if string(got) != "old-binary" {
		t.Errorf("binary replaced despite matching the unstable commit")
	}
}

func TestSelfUpdateVersionStable(t *testing.T) {
	withVersion(t, "v1.0.0-14-gabc1234")
	withChannel(t, "unstable")
	script := "#!/bin/sh\nexit 0\n"
	exePath := releaseFixture(t, "v1.1.0", script)

	out, err := execNemMerged(t, t.TempDir(), "", "self", "update", "--version", "stable")
	if err != nil {
		t.Fatalf("self update --version stable: %v\n%s", err, out)
	}
	got, _ := os.ReadFile(exePath)
	if string(got) != script {
		t.Errorf("binary content = %q, want the downloaded release", got)
	}
	if !strings.Contains(out, "https://github.com/vi-dev/nem/releases/tag/v1.1.0") {
		t.Errorf("output missing release notes link:\n%s", out)
	}
}

func TestSelfUpdateUpToDateWithoutVPrefix(t *testing.T) {
	withVersion(t, "1.1.0")
	withUpdater(t, latestReleaseAPI(t, "v1.1.0"))

	out, err := execNemMerged(t, t.TempDir(), "", "self", "update")
	if err != nil {
		t.Fatalf("self update: %v\n%s", err, out)
	}
	if !strings.Contains(out, "up to date") {
		t.Errorf("output missing up-to-date notice:\n%s", out)
	}
}

func TestVersionPrintsChannel(t *testing.T) {
	withChannel(t, "unstable")

	out, err := execNemMerged(t, t.TempDir(), "", "version")
	if err != nil {
		t.Fatalf("version: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Channel: unstable") {
		t.Errorf("output missing channel line:\n%s", out)
	}
}

func TestSelfUpdateRefusesDevBuild(t *testing.T) {
	withVersion(t, "dev")
	withUpdater(t, latestReleaseAPI(t, "v1.1.0"))

	_, err := execNemMerged(t, t.TempDir(), "", "self", "update")
	if err == nil {
		t.Fatal("want error for dev build")
	}
	if !strings.Contains(err.Error(), "release") {
		t.Errorf("error does not explain the dev-build refusal: %v", err)
	}
}

func TestSelfUpdateRejectsMalformedVersion(t *testing.T) {
	withVersion(t, "v1.0.0")
	withUpdater(t, latestReleaseAPI(t, "v1.1.0"))

	_, err := execNemMerged(t, t.TempDir(), "", "self", "update", "--version", "1.2.3")
	if err == nil {
		t.Fatal("want error for version without v prefix")
	}
	if !strings.Contains(err.Error(), "1.2.3") {
		t.Errorf("error does not name the bad value: %v", err)
	}
}
