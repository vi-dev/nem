package bump

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vi-dev/nem/internal/discover"
	"github.com/vi-dev/nem/internal/report"
	"github.com/vi-dev/nem/internal/spec"
	"github.com/vi-dev/nem/internal/testx"
)

func runBump(t *testing.T, opts Options, target string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errb bytes.Buffer
	console := report.New(&out, &errb, report.Options{Color: report.ColorNever})
	err = Run(context.Background(), target, opts, console)
	return out.String(), errb.String(), err
}

func writeFixture(t *testing.T, pkgs map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, y := range pkgs {
		testx.WriteFile(t, filepath.Join(dir, "pkgs", name, "pkg.yaml"), y)
	}
	return dir
}

func jqFixture(serverURL, versionsYAML string) string {
	return fmt.Sprintf(`schema: 2
name: jq
artifact:
  url: "%s/{{.Version}}/{{.OS}}-{{.Arch}}"
install:
  - copy: {src: "{{.Artifact}}", dst: "bin/jq", mode: 0o755}
versionDiscovery:
  github:
    repo: jqlang/jq
    prefix: "jq-"
%s`, serverURL, versionsYAML)
}

func jqVersionsYAML(versions ...string) string {
	var b strings.Builder
	b.WriteString("versions:\n")
	for i, v := range versions {
		fmt.Fprintf(&b, "  - version: %s\n    sha256:\n", v)
		for j, plat := range []string{"darwin/arm64", "darwin/amd64", "linux/arm64", "linux/amd64"} {
			c := string(rune('a' + 4*i + j))
			fmt.Fprintf(&b, "      %s: %q\n", plat, strings.Repeat(c, 3))
		}
	}
	return b.String()
}

func bumpFixture(serverURL string) string {
	return jqFixture(serverURL, jqVersionsYAML("1.8.2"))
}

func stubList(t *testing.T, versions ...string) {
	t.Helper()
	stubListFunc(t, func(context.Context, *spec.Package) ([]discover.Discovered, error) {
		out := make([]discover.Discovered, len(versions))
		for i, v := range versions {
			out[i] = discover.Discovered{Version: v}
		}
		return out, nil
	})
}

func stubListFunc(t *testing.T, fn func(context.Context, *spec.Package) ([]discover.Discovered, error)) {
	t.Helper()
	testx.Swap(t, &listVersions, fn)
}

func bumpMultiArtifactServer(t *testing.T, versions ...string) (*httptest.Server, map[string]string) {
	t.Helper()
	serve := map[string]bool{}
	sums := map[string]string{}
	for _, v := range versions {
		serve[v] = true
		for _, p := range spec.SupportedPlatforms {
			body := "artifact " + v + " " + p.String()
			s := sha256.Sum256([]byte(body))
			sums[v+" "+p.String()] = hex.EncodeToString(s[:])
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
		if len(parts) != 2 || !serve[parts[0]] {
			http.NotFound(w, r)
			return
		}
		osArch := strings.SplitN(parts[1], "-", 2)
		if len(osArch) != 2 {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, "artifact %s %s/%s", parts[0], osArch[0], osArch[1])
	}))
	t.Cleanup(srv.Close)
	return srv, sums
}

func assertUnchanged(t *testing.T, path string, before []byte) {
	t.Helper()
	if !bytes.Equal(mustRead(t, path), before) {
		t.Fatal("bump must not modify the manifest")
	}
}

func TestBumpAddsAllNewerVersions(t *testing.T) {
	srv, sums := bumpMultiArtifactServer(t, "1.8.3", "1.8.4")
	dir := writeFixture(t, map[string]string{"jq": bumpFixture(srv.URL)})
	path := filepath.Join(dir, "pkgs", "jq", "pkg.yaml")
	stubList(t, "1.8.4", "1.7.1", "1.8.3", "1.8.2")

	_, errOut, err := runBump(t, Options{}, path)
	if err != nil {
		t.Fatalf("bump: %v", err)
	}

	for _, line := range []string{"Hashed jq 1.8.3 linux/amd64", "Hashed jq 1.8.4 darwin/arm64"} {
		if !strings.Contains(errOut, line) {
			t.Errorf("stderr = %q, want %q", errOut, line)
		}
	}
	pkg, err := spec.Parse(mustRead(t, path))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"1.8.4", "1.8.3", "1.8.2"}
	if len(pkg.Versions) != len(want) {
		t.Fatalf("got %d versions, want %d", len(pkg.Versions), len(want))
	}
	for i, v := range want {
		if pkg.Versions[i].Version != v {
			t.Fatalf("versions[%d] = %q, want %q (newest-first)", i, pkg.Versions[i].Version, v)
		}
	}
	if got := pkg.Versions[0].Sha256["linux/amd64"]; got != sums["1.8.4 linux/amd64"] {
		t.Errorf("1.8.4 sha256 = %q, want %q", got, sums["1.8.4 linux/amd64"])
	}
	if got := pkg.Versions[1].Sha256["linux/amd64"]; got != sums["1.8.3 linux/amd64"] {
		t.Errorf("1.8.3 sha256 = %q, want %q", got, sums["1.8.3 linux/amd64"])
	}
}

func TestBumpBackfillCoverage(t *testing.T) {
	srv, sums := bumpMultiArtifactServer(t, "1.8.4", "1.8.3", "1.8.1")
	dir := writeFixture(t, map[string]string{"jq": bumpFixture(srv.URL)})
	path := filepath.Join(dir, "pkgs", "jq", "pkg.yaml")
	stubList(t, "1.8.4", "1.8.3", "1.8.2", "1.8.1", "1.8.0", "1.7.1")

	_, errOut, err := runBump(t, Options{Backfill: 4}, path)
	if err != nil {
		t.Fatalf("bump: %v", err)
	}
	pkg, err := spec.Parse(mustRead(t, path))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"1.8.4", "1.8.3", "1.8.2", "1.8.1"}
	if len(pkg.Versions) != len(want) {
		t.Fatalf("got %d versions, want %d: %+v", len(pkg.Versions), len(want), pkg.Versions)
	}
	for i, v := range want {
		if pkg.Versions[i].Version != v {
			t.Fatalf("versions[%d] = %q, want %q (newest-first)", i, pkg.Versions[i].Version, v)
		}
	}
	if got := pkg.Versions[3].Sha256["linux/amd64"]; got != sums["1.8.1 linux/amd64"] {
		t.Errorf("backfilled 1.8.1 sha256 = %q, want %q", got, sums["1.8.1 linux/amd64"])
	}
	if !strings.Contains(errOut, "Bumped jq 1.8.2 → 1.8.4 (3 versions)") {
		t.Fatalf("stderr = %q, want bumped summary", errOut)
	}
}

func TestBumpBackfillOnlyOldVersions(t *testing.T) {
	srv, _ := bumpMultiArtifactServer(t, "1.8.1")
	dir := writeFixture(t, map[string]string{"jq": bumpFixture(srv.URL)})
	path := filepath.Join(dir, "pkgs", "jq", "pkg.yaml")
	stubList(t, "1.8.2", "1.8.1")

	_, errOut, err := runBump(t, Options{Backfill: 2}, path)
	if err != nil {
		t.Fatalf("bump: %v", err)
	}
	pkg, err := spec.Parse(mustRead(t, path))
	if err != nil {
		t.Fatal(err)
	}
	if len(pkg.Versions) != 2 || pkg.Versions[0].Version != "1.8.2" || pkg.Versions[1].Version != "1.8.1" {
		t.Fatalf("versions = %+v, want [1.8.2 1.8.1]", pkg.Versions)
	}
	if !strings.Contains(errOut, "Backfilled jq (1 version)") {
		t.Fatalf("stderr = %q, want backfilled summary", errOut)
	}
}

func TestBumpBackfillIsIdempotent(t *testing.T) {
	srv, _ := bumpMultiArtifactServer(t, "1.8.1")
	dir := writeFixture(t, map[string]string{"jq": bumpFixture(srv.URL)})
	path := filepath.Join(dir, "pkgs", "jq", "pkg.yaml")
	stubList(t, "1.8.2", "1.8.1")

	if _, _, err := runBump(t, Options{Backfill: 2}, path); err != nil {
		t.Fatalf("first bump: %v", err)
	}
	before := mustRead(t, path)
	_, errOut, err := runBump(t, Options{Backfill: 2}, path)
	if err != nil {
		t.Fatalf("second bump: %v", err)
	}
	if !strings.Contains(errOut, "up to date") {
		t.Fatalf("stderr = %q, want up-to-date notice", errOut)
	}
	if string(mustRead(t, path)) != string(before) {
		t.Fatal("idempotent backfill must not rewrite the file")
	}
}

func opensslFixture(serverURL, urlPath, versionYAML string) string {
	return fmt.Sprintf(`schema: 2
name: openssl
platforms: [darwin/arm64, linux/arm64, linux/amd64]
artifact:
  oci: ":{{.Version}}"
install:
  - extract: {strip: 0}
versions:
%s
build:
  source:
    url: '%s%s'
  output: out
  steps:
    - run: make
`, versionYAML, serverURL, urlPath)
}

func sourceBackfillFixture(serverURL string) string {
	return opensslFixture(serverURL, "/openssl-{{.Version}}.tar.gz",
		`  - version: 3.4.2
    sourceSha256: "0000000000000000000000000000000000000000000000000000000000000000"`)
}

func TestBumpBackfillSourceBuiltWarns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openssl-3.4.1.tar.gz" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, "source tarball 3.4.1")
	}))
	t.Cleanup(srv.Close)
	dir := writeFixture(t, map[string]string{"openssl": sourceBackfillFixture(srv.URL)})
	path := filepath.Join(dir, "pkgs", "openssl", "pkg.yaml")
	stubList(t, "3.4.2", "3.4.1")

	_, errOut, err := runBump(t, Options{Backfill: 2}, path)
	if err != nil {
		t.Fatalf("bump: %v", err)
	}
	pkg, err := spec.Parse(mustRead(t, path))
	if err != nil {
		t.Fatal(err)
	}
	if len(pkg.Versions) != 2 || pkg.Versions[1].Version != "3.4.1" || pkg.Versions[1].SourceSha256 == "" {
		t.Fatalf("versions = %+v, want backfilled 3.4.1 with sourceSha256", pkg.Versions)
	}
	if !strings.Contains(errOut, "source-built") || !strings.Contains(errOut, "build --version") {
		t.Fatalf("stderr = %q, want source-built warning and build hint", errOut)
	}
}

func TestBumpDeadSourceURLNamesItWithoutRetryHint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(http.NotFound))
	t.Cleanup(srv.Close)
	dir := writeFixture(t, map[string]string{"openssl": sourceBackfillFixture(srv.URL)})
	path := filepath.Join(dir, "pkgs", "openssl", "pkg.yaml")
	before := mustRead(t, path)

	_, errOut, err := runBump(t, Options{Version: "3.4.3"}, path)
	if err == nil {
		t.Fatal("want error for a dead source URL")
	}
	if !strings.Contains(errOut, srv.URL) {
		t.Fatalf("stderr = %q, want the failing source URL named", errOut)
	}
	if strings.Contains(errOut, "retry later") {
		t.Fatalf("stderr = %q, must not suggest retrying a dead source URL", errOut)
	}
	if string(mustRead(t, path)) != string(before) {
		t.Fatal("failed bump must not modify the manifest")
	}
}

func TestBumpRejectsBadInvocation(t *testing.T) {
	tests := []struct {
		name       string
		serve      []string
		pkg        string
		fixture    func(serverURL string) string
		stubByName map[string][]string
		dirTarget  bool
		opts       Options
		wantErr    string
	}{
		{
			name:    "version and backfill conflict",
			pkg:     "jq",
			fixture: bumpFixture,
			opts:    Options{Version: "1.8.3", Backfill: 2},
			wantErr: "combined",
		},
		{
			name:    "version outside OCI tag grammar",
			serve:   []string{"25.0.4+7"},
			pkg:     "jq",
			fixture: bumpFixture,
			opts:    Options{Version: "25.0.4+7"},
		},
		{
			name:       "version with directory target",
			serve:      []string{"1.1.0"},
			pkg:        "aa",
			fixture:    func(serverURL string) string { return namedBumpFixture("aa", serverURL, "1.0.0") },
			stubByName: map[string][]string{"aa": {"1.1.0", "1.0.0"}},
			dirTarget:  true,
			opts:       Options{Version: "1.1.0"},
			wantErr:    "single pkg.yaml",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := bumpMultiArtifactServer(t, tt.serve...)
			dir := writeFixture(t, map[string]string{tt.pkg: tt.fixture(srv.URL)})
			path := filepath.Join(dir, "pkgs", tt.pkg, "pkg.yaml")
			if tt.stubByName != nil {
				stubListByName(t, tt.stubByName)
			}
			before := mustRead(t, path)

			target := path
			if tt.dirTarget {
				target = dir
			}
			_, _, err := runBump(t, tt.opts, target)
			if err == nil {
				t.Fatalf("opts %+v: want error", tt.opts)
			}
			if tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
			assertUnchanged(t, path, before)
		})
	}
}

func TestBumpVersionBackfillsInPlace(t *testing.T) {
	srv, _ := bumpMultiArtifactServer(t, "1.8.1")
	dir := writeFixture(t, map[string]string{"jq": bumpFixture(srv.URL)})
	path := filepath.Join(dir, "pkgs", "jq", "pkg.yaml")

	_, errOut, err := runBump(t, Options{Version: "1.8.1"}, path)
	if err != nil {
		t.Fatalf("bump: %v", err)
	}
	pkg, err := spec.Parse(mustRead(t, path))
	if err != nil {
		t.Fatal(err)
	}
	if len(pkg.Versions) != 2 || pkg.Versions[0].Version != "1.8.2" || pkg.Versions[1].Version != "1.8.1" {
		t.Fatalf("versions = %+v, want [1.8.2 1.8.1] (older --version must not become head)", pkg.Versions)
	}
	if !strings.Contains(errOut, "Backfilled jq (1 version)") {
		t.Fatalf("stderr = %q, want backfilled summary", errOut)
	}
}

func TestBumpToleratesLegacyTail(t *testing.T) {
	srv, _ := bumpMultiArtifactServer(t, "2.1.0")
	dir := writeFixture(t, map[string]string{"jq": jqFixture(srv.URL, jqVersionsYAML("2.0.0", "v1.9.0"))})
	path := filepath.Join(dir, "pkgs", "jq", "pkg.yaml")
	stubList(t, "2.1.0", "2.0.0")

	if _, _, err := runBump(t, Options{}, path); err != nil {
		t.Fatalf("bump must tolerate a legacy tail below a migrated head: %v", err)
	}
	pkg, err := spec.Parse(mustRead(t, path))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2.1.0", "2.0.0", "v1.9.0"}
	for i, v := range want {
		if pkg.Versions[i].Version != v {
			t.Fatalf("versions[%d] = %q, want %q", i, pkg.Versions[i].Version, v)
		}
	}
}

func TestBumpRejectsUnorderedHistory(t *testing.T) {
	srv, _ := bumpMultiArtifactServer(t, "2.1.0")
	dir := writeFixture(t, map[string]string{"jq": jqFixture(srv.URL, jqVersionsYAML("2.0.0", "1.0.0", "1.5.0"))})
	path := filepath.Join(dir, "pkgs", "jq", "pkg.yaml")
	stubList(t, "2.1.0")
	before := mustRead(t, path)

	_, _, err := runBump(t, Options{}, path)
	if err == nil || !strings.Contains(err.Error(), "newest-first") {
		t.Fatalf("err = %v, want newest-first rejection (out-of-order history is a lint finding, not bumpable state)", err)
	}
	if string(mustRead(t, path)) != string(before) {
		t.Fatal("rejected bump must not modify the manifest")
	}
}

func TestBumpNoopKeepsManifest(t *testing.T) {
	tests := []struct {
		name       string
		serve      []string
		versions   string
		stub       []string
		opts       Options
		wantNotice string
	}{
		{
			name:       "discovery finds nothing newer",
			versions:   jqVersionsYAML("1.8.2"),
			stub:       []string{"1.8.2", "1.7.1"},
			wantNotice: "up to date",
		},
		{
			name:       "spelling-equal candidate skipped",
			serve:      []string{"1.3.1"},
			versions:   jqVersionsYAML("v1.3.1"),
			stub:       []string{"1.3.1"},
			opts:       Options{Backfill: 1},
			wantNotice: "up to date",
		},
		{
			name:       "explicit version already present",
			serve:      []string{"1.8.2"},
			versions:   jqVersionsYAML("1.8.2"),
			opts:       Options{Version: "1.8.2"},
			wantNotice: "already present",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := bumpMultiArtifactServer(t, tt.serve...)
			dir := writeFixture(t, map[string]string{"jq": jqFixture(srv.URL, tt.versions)})
			path := filepath.Join(dir, "pkgs", "jq", "pkg.yaml")
			if tt.stub != nil {
				stubList(t, tt.stub...)
			}
			before := mustRead(t, path)

			_, errOut, err := runBump(t, tt.opts, path)
			if err != nil {
				t.Fatalf("bump: %v", err)
			}
			if !strings.Contains(errOut, tt.wantNotice) {
				t.Fatalf("stderr = %q, want %q notice", errOut, tt.wantNotice)
			}
			assertUnchanged(t, path, before)
		})
	}
}

func TestBumpRejectsFlowVersionsBeforeDownloading(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		fmt.Fprint(w, "ok")
	}))
	t.Cleanup(srv.Close)
	flowVersions := `versions: [{version: 1.8.2, sha256: {darwin/arm64: "aaa", darwin/amd64: "bbb", linux/arm64: "ccc", linux/amd64: "ddd"}}]` + "\n"
	dir := writeFixture(t, map[string]string{"jq": jqFixture(srv.URL, flowVersions)})
	path := filepath.Join(dir, "pkgs", "jq", "pkg.yaml")
	before := mustRead(t, path)

	_, _, err := runBump(t, Options{Version: "1.8.3"}, path)
	if err == nil || !strings.Contains(err.Error(), "block sequence") {
		t.Fatalf("err = %v, want block-sequence rejection", err)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("server got %d requests, want 0 (must fail before downloading)", n)
	}
	if string(mustRead(t, path)) != string(before) {
		t.Fatal("rejected bump must not modify the manifest")
	}
}

func TestBumpSkipsFailedCandidate(t *testing.T) {
	srv, _ := bumpMultiArtifactServer(t, "1.8.4")
	dir := writeFixture(t, map[string]string{"jq": bumpFixture(srv.URL)})
	path := filepath.Join(dir, "pkgs", "jq", "pkg.yaml")
	stubList(t, "1.8.3", "1.8.4")

	_, errOut, err := runBump(t, Options{}, path)
	if err != nil {
		t.Fatalf("bump must succeed when another candidate lands: %v", err)
	}
	pkg, err := spec.Parse(mustRead(t, path))
	if err != nil {
		t.Fatal(err)
	}
	if len(pkg.Versions) != 2 || pkg.Versions[0].Version != "1.8.4" || pkg.Versions[1].Version != "1.8.2" {
		t.Fatalf("versions = %+v, want [1.8.4 1.8.2]", pkg.Versions)
	}
	if !strings.Contains(errOut, "1.8.3") {
		t.Fatalf("stderr = %q, want a warning naming the skipped 1.8.3", errOut)
	}
}

func TestBumpFailureWritesNothing(t *testing.T) {
	tests := []struct {
		name      string
		newServer func(t *testing.T) string
		stub      []string
		opts      Options
	}{
		{
			name: "every candidate fails",
			newServer: func(t *testing.T) string {
				srv, _ := bumpMultiArtifactServer(t)
				return srv.URL
			},
			stub: []string{"1.8.3"},
		},
		{
			name: "one platform artifact missing",
			newServer: func(t *testing.T) string {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if strings.Contains(r.URL.Path, "darwin-arm64") {
						http.NotFound(w, r)
						return
					}
					fmt.Fprint(w, "ok")
				}))
				t.Cleanup(srv.Close)
				return srv.URL
			},
			opts: Options{Version: "1.8.3"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeFixture(t, map[string]string{"jq": bumpFixture(tt.newServer(t))})
			path := filepath.Join(dir, "pkgs", "jq", "pkg.yaml")
			if tt.stub != nil {
				stubList(t, tt.stub...)
			}
			before := mustRead(t, path)

			_, errOut, err := runBump(t, tt.opts, path)
			if err == nil {
				t.Fatal("want error when candidates fail to download")
			}
			if !strings.Contains(errOut, "retry later") {
				t.Fatalf("stderr = %q, want not-uploaded-yet hint", errOut)
			}
			assertUnchanged(t, path, before)
		})
	}
}

func TestBumpSourceBuilt(t *testing.T) {
	body := []byte("source tarball v3.4.2")
	wantSum := sha256.Sum256(body)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openssl-3.4.2.tar.gz" {
			http.NotFound(w, r)
			return
		}
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	fixture := opensslFixture(srv.URL, `/openssl-{{trimPrefix .Version "v"}}.tar.gz`,
		`  - version: v3.4.1
    sourceSha256: "002a2d6b30b58bf4bea46c43bdd96365aaf8daa6c428782aa4feee06da197df3"`)
	dir := writeFixture(t, map[string]string{"openssl": fixture})
	path := filepath.Join(dir, "pkgs", "openssl", "pkg.yaml")

	_, errOut, err := runBump(t, Options{Version: "v3.4.2"}, path)
	if !strings.Contains(errOut, "Hashed openssl v3.4.2 source") {
		t.Errorf("stderr = %q, want source line naming the version", errOut)
	}
	if err != nil {
		t.Fatalf("bump: %v", err)
	}
	pkg, err := spec.Parse(mustRead(t, path))
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Versions[0].Version != "v3.4.2" ||
		pkg.Versions[0].SourceSha256 != hex.EncodeToString(wantSum[:]) {
		t.Fatalf("versions[0] = %+v", pkg.Versions[0])
	}
}

func TestBumpBareOCI(t *testing.T) {
	fixture := `schema: 2
name: tool
artifact:
  oci: ":{{.Version}}"
install:
  - extract: {strip: 0}
versions:
  - v1.0.0
`
	dir := writeFixture(t, map[string]string{"tool": fixture})
	path := filepath.Join(dir, "pkgs", "tool", "pkg.yaml")

	if _, _, err := runBump(t, Options{Version: "v1.1.0"}, path); err != nil {
		t.Fatalf("bump: %v", err)
	}
	pkg, err := spec.Parse(mustRead(t, path))
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Versions[0].Version != "v1.1.0" || pkg.Versions[0].Sha256 != nil || pkg.Versions[0].SourceSha256 != "" {
		t.Fatalf("versions[0] = %+v, want bare scalar", pkg.Versions[0])
	}
}

func sweepFixture(name, serverURL, current string, discovery bool) string {
	disc := ""
	if discovery {
		disc = fmt.Sprintf("versionDiscovery:\n  github:\n    repo: example/%s\n", name)
	}
	return fmt.Sprintf(`schema: 2
name: %s
artifact:
  url: "%s/{{.Version}}/{{.OS}}-{{.Arch}}"
install:
  - copy: {src: "{{.Artifact}}", dst: "bin/%s", mode: 0o755}
%sversions:
  - version: %s
    sha256:
      darwin/arm64: "aaa"
      darwin/amd64: "bbb"
      linux/arm64: "ccc"
      linux/amd64: "ddd"
`, name, serverURL, name, disc, current)
}

func namedBumpFixture(name, serverURL, current string) string {
	return sweepFixture(name, serverURL, current, true)
}

func stubListByName(t *testing.T, byName map[string][]string) {
	t.Helper()
	stubListFunc(t, func(_ context.Context, pkg *spec.Package) ([]discover.Discovered, error) {
		vs, ok := byName[pkg.Name]
		if !ok {
			return nil, fmt.Errorf("discovery unavailable for %s", pkg.Name)
		}
		out := make([]discover.Discovered, len(vs))
		for i, v := range vs {
			out[i] = discover.Discovered{Version: v}
		}
		return out, nil
	})
}

func TestBumpDirMixedOutcomes(t *testing.T) {
	srv, _ := bumpMultiArtifactServer(t, "1.1.0")
	stubListByName(t, map[string][]string{
		"aa": {"1.1.0", "1.0.0"},
		"bb": {"2.0.0"},
	})
	newDir := func(t *testing.T) string {
		return writeFixture(t, map[string]string{
			"aa": namedBumpFixture("aa", srv.URL, "1.0.0"),
			"bb": namedBumpFixture("bb", srv.URL, "2.0.0"),
			"cc": namedBumpFixture("cc", srv.URL, "3.0.0"),
			"dd": sweepFixture("dd", srv.URL, "1.0.0", false),
		})
	}

	t.Run("summary exits zero", func(t *testing.T) {
		dir := newDir(t)
		bbPath := filepath.Join(dir, "pkgs", "bb", "pkg.yaml")
		bbBefore := mustRead(t, bbPath)

		_, errOut, err := runBump(t, Options{}, dir)
		if err != nil {
			t.Fatalf("sweep must exit zero on per-package failures: %v", err)
		}
		if !strings.Contains(errOut, "discovery unavailable for cc") {
			t.Errorf("stderr = %q, want cc failure warning", errOut)
		}
		if !strings.Contains(errOut, "Checked 4 packages: 1 bumped, 1 up to date, 1 failed, 1 without discovery") {
			t.Fatalf("stderr = %q, want mixed-outcome summary", errOut)
		}
		if string(mustRead(t, bbPath)) != string(bbBefore) {
			t.Fatal("up-to-date package must not be rewritten")
		}
	})

	t.Run("json rows", func(t *testing.T) {
		dir := newDir(t)

		out, _, err := runBump(t, Options{JSON: true}, dir)
		if err != nil {
			t.Fatalf("bump: %v", err)
		}
		rows := decodeRows(t, out)
		if len(rows) != 3 {
			t.Fatalf("got %d rows, want 3 (packages without discovery are not rows): %s", len(rows), out)
		}
		byName := map[string]jsonRow{}
		for _, r := range rows {
			byName[r.Name] = r
		}
		aa := byName["aa"]
		if aa.Current != "1.0.0" || aa.Head != "1.1.0" || len(aa.Added) != 1 || aa.Added[0] != "1.1.0" || aa.Error != "" {
			t.Errorf("aa row = %+v, want current 1.0.0, head 1.1.0, added [1.1.0]", aa)
		}
		if want := filepath.Join(dir, "pkgs", "aa", "pkg.yaml"); aa.Path != want {
			t.Errorf("aa path = %q, want %q", aa.Path, want)
		}
		bb := byName["bb"]
		if bb.Current != "2.0.0" || bb.Head != "2.0.0" || len(bb.Added) != 0 || bb.Error != "" {
			t.Errorf("bb row = %+v, want unchanged head 2.0.0 and no added versions", bb)
		}
		cc := byName["cc"]
		if !strings.Contains(cc.Error, "discovery unavailable for cc") || cc.Head != "" || len(cc.Added) != 0 {
			t.Errorf("cc row = %+v, want error and no head", cc)
		}
	})
}

func TestBumpDirSweepsAllPackages(t *testing.T) {
	srv, _ := bumpMultiArtifactServer(t, "1.1.0", "1.8.3")
	dir := writeFixture(t, map[string]string{
		"aa": namedBumpFixture("aa", srv.URL, "1.0.0"),
		"jq": namedBumpFixture("jq", srv.URL, "1.8.2"),
	})
	stubListByName(t, map[string][]string{
		"aa": {"1.1.0", "1.0.0"},
		"jq": {"1.8.3", "1.8.2"},
	})

	_, errOut, err := runBump(t, Options{}, dir)
	if err != nil {
		t.Fatalf("bump: %v", err)
	}
	for name, want := range map[string]string{"aa": "1.1.0", "jq": "1.8.3"} {
		pkg, err := spec.Parse(mustRead(t, filepath.Join(dir, "pkgs", name, "pkg.yaml")))
		if err != nil {
			t.Fatal(err)
		}
		if pkg.Versions[0].Version != want {
			t.Fatalf("%s head = %q, want %q", name, pkg.Versions[0].Version, want)
		}
	}
	for _, line := range []string{"Bumped aa 1.0.0 → 1.1.0", "Bumped jq 1.8.2 → 1.8.3"} {
		if !strings.Contains(errOut, line) {
			t.Errorf("stderr = %q, want %q", errOut, line)
		}
	}
	if !strings.Contains(errOut, "Checked 2 packages: 2 bumped, 0 up to date, 0 failed, 0 without discovery") {
		t.Fatalf("stderr = %q, want sweep summary", errOut)
	}
}

type jsonRow struct {
	Name    string   `json:"name"`
	Path    string   `json:"path"`
	Current string   `json:"current"`
	Head    string   `json:"head"`
	Added   []string `json:"added"`
	Error   string   `json:"error"`
}

func decodeRows(t *testing.T, out string) []jsonRow {
	t.Helper()
	var rows []jsonRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out)
	}
	return rows
}

func TestBumpDirRunsPackagesConcurrently(t *testing.T) {
	srv, _ := bumpMultiArtifactServer(t)
	dir := writeFixture(t, map[string]string{
		"aa": namedBumpFixture("aa", srv.URL, "1.0.0"),
		"bb": namedBumpFixture("bb", srv.URL, "2.0.0"),
	})

	gate := make(chan struct{})
	bbDone := make(chan struct{})
	stubListFunc(t, func(_ context.Context, pkg *spec.Package) ([]discover.Discovered, error) {
		select {
		case gate <- struct{}{}:
		case <-gate:
		case <-time.After(2 * time.Second):
			return nil, fmt.Errorf("no concurrent discovery for %s within 2s", pkg.Name)
		}
		if pkg.Name == "aa" {
			<-bbDone
			return []discover.Discovered{{Version: "1.0.0"}}, nil
		}
		defer close(bbDone)
		return []discover.Discovered{{Version: "2.0.0"}}, nil
	})

	out, errOut, err := runBump(t, Options{JSON: true}, dir)
	if err != nil {
		t.Fatalf("bump: %v", err)
	}
	if !strings.Contains(errOut, "Checked 2 packages: 0 bumped, 2 up to date, 0 failed, 0 without discovery") {
		t.Fatalf("stderr = %q, want both packages up to date (failures mean the sweep ran sequentially)", errOut)
	}
	rows := decodeRows(t, out)
	if len(rows) != 2 || rows[0].Name != "aa" || rows[1].Name != "bb" {
		t.Fatalf("rows = %+v, want manifest order [aa bb] regardless of completion order", rows)
	}
}

func TestBumpSingleFileJSON(t *testing.T) {
	srv, _ := bumpMultiArtifactServer(t, "1.8.3")
	dir := writeFixture(t, map[string]string{"jq": bumpFixture(srv.URL)})
	path := filepath.Join(dir, "pkgs", "jq", "pkg.yaml")
	stubList(t, "1.8.3", "1.8.2")

	out, _, err := runBump(t, Options{JSON: true}, path)
	if err != nil {
		t.Fatalf("bump: %v", err)
	}
	rows := decodeRows(t, out)
	if len(rows) != 1 || rows[0].Name != "jq" || rows[0].Current != "1.8.2" || rows[0].Head != "1.8.3" {
		t.Fatalf("rows = %+v, want one jq row 1.8.2 → 1.8.3", rows)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func metaFixture(serverURL, versionsYAML string) string {
	return fmt.Sprintf(`schema: 2
name: jq
artifact:
  url: "%s/{{.Version}}/{{.OS}}-{{.Arch}}?d={{.Meta.date}}"
install:
  - copy: {src: "{{.Artifact}}", dst: "bin/jq", mode: 0o755}
versionDiscovery:
  github:
    repo: jqlang/jq
    prefix: "jq-"
%s`, serverURL, versionsYAML)
}

func TestBumpWritesDiscoveredMeta(t *testing.T) {
	srv, _ := bumpMultiArtifactServer(t, "1.9.0")
	stubListFunc(t, func(context.Context, *spec.Package) ([]discover.Discovered, error) {
		return []discover.Discovered{{Version: "1.9.0", Meta: map[string]string{"date": "20260101"}}}, nil
	})
	yaml := metaFixture(srv.URL, "versions:\n  - version: 1.8.2\n    meta: {date: \"20250101\"}\n    sha256:\n      darwin/arm64: \"a\"\n      darwin/amd64: \"b\"\n      linux/arm64: \"c\"\n      linux/amd64: \"d\"\n")
	dir := writeFixture(t, map[string]string{"jq": yaml})
	path := filepath.Join(dir, "pkgs", "jq", "pkg.yaml")

	if _, _, err := runBump(t, Options{}, path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := spec.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Versions[0].Version != "1.9.0" || pkg.Versions[0].Meta["date"] != "20260101" {
		t.Fatalf("head entry = %+v", pkg.Versions[0])
	}
}

func TestBumpVersionFlagDiscoversMetaWhenTemplatesNeedIt(t *testing.T) {
	srv, _ := bumpMultiArtifactServer(t, "1.9.0")
	stubListFunc(t, func(context.Context, *spec.Package) ([]discover.Discovered, error) {
		return []discover.Discovered{{Version: "1.9.0", Meta: map[string]string{"date": "20260101"}}}, nil
	})
	yaml := metaFixture(srv.URL, "versions:\n  - version: 1.8.2\n    meta: {date: \"20250101\"}\n    sha256:\n      darwin/arm64: \"a\"\n      darwin/amd64: \"b\"\n      linux/arm64: \"c\"\n      linux/amd64: \"d\"\n")
	dir := writeFixture(t, map[string]string{"jq": yaml})
	path := filepath.Join(dir, "pkgs", "jq", "pkg.yaml")

	if _, _, err := runBump(t, Options{Version: "1.9.0"}, path); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	pkg, err := spec.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Versions[0].Meta["date"] != "20260101" {
		t.Fatalf("head entry = %+v", pkg.Versions[0])
	}
}

func TestBumpVersionFlagSkipsDiscoveryWithoutMetaTemplates(t *testing.T) {
	srv, _ := bumpMultiArtifactServer(t, "1.9.0")
	stubListFunc(t, func(context.Context, *spec.Package) ([]discover.Discovered, error) {
		t.Error("discovery must not run for --version without meta templates")
		return nil, nil
	})
	dir := writeFixture(t, map[string]string{"jq": bumpFixture(srv.URL)})
	path := filepath.Join(dir, "pkgs", "jq", "pkg.yaml")

	if _, _, err := runBump(t, Options{Version: "1.9.0"}, path); err != nil {
		t.Fatal(err)
	}
}

func TestBumpFreshManifestVersionFlag(t *testing.T) {
	srv, sums := bumpMultiArtifactServer(t, "1.9.0")
	dir := writeFixture(t, map[string]string{"jq": jqFixture(srv.URL, "")})
	path := filepath.Join(dir, "pkgs", "jq", "pkg.yaml")

	if _, _, err := runBump(t, Options{Version: "1.9.0"}, path); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	pkg, err := spec.Parse(data)
	if err != nil {
		t.Fatalf("re-parse: %v\n%s", err, data)
	}
	if len(pkg.Versions) != 1 || pkg.Versions[0].Version != "1.9.0" {
		t.Fatalf("versions = %+v", pkg.Versions)
	}
	if got := pkg.Versions[0].Sha256["darwin/arm64"]; got != sums["1.9.0 darwin/arm64"] {
		t.Errorf("sha256 = %q", got)
	}
}

func TestBumpFreshManifestSeedsLatestOnly(t *testing.T) {
	srv, _ := bumpMultiArtifactServer(t, "1.8.2", "1.9.0")
	stubList(t, "1.8.2", "1.9.0")
	dir := writeFixture(t, map[string]string{"jq": jqFixture(srv.URL, "")})
	path := filepath.Join(dir, "pkgs", "jq", "pkg.yaml")

	if _, _, err := runBump(t, Options{}, path); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	pkg, err := spec.Parse(data)
	if err != nil {
		t.Fatalf("re-parse: %v\n%s", err, data)
	}
	if len(pkg.Versions) != 1 || pkg.Versions[0].Version != "1.9.0" {
		t.Fatalf("versions = %+v, want just 1.9.0", pkg.Versions)
	}
}
