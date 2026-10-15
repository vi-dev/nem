package bump

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/vi-dev/nem/internal/fetch"
	"github.com/vi-dev/nem/internal/report"
	"github.com/vi-dev/nem/internal/spec"
	"github.com/vi-dev/nem/internal/testx"
)

func runBumpVerbose(t *testing.T, opts Options, target string) (res *Result, stderr string, err error) {
	t.Helper()
	var errb bytes.Buffer
	console := report.New(io.Discard, &errb, report.Options{Color: report.ColorNever, Verbose: true})
	res, err = Run(report.NewContext(context.Background(), console), target, opts)
	return res, errb.String(), err
}

func releaseFixture(name string, versionsYAML string) string {
	return fmt.Sprintf(`schema: 3
name: %[1]s
artifact:
  url: "https://github.com/acme/%[1]s/releases/download/v{{.Version}}/%[1]s-{{.Version}}-{{.OS}}-{{.Arch}}"
install:
  - copy: {src: "{{.Artifact}}", dst: "bin/%[1]s", mode: 0o755}
versionDiscovery:
  github:
    repo: acme/%[1]s
    prefix: "v"
%[2]s`, name, versionsYAML)
}

type releaseAPI struct {
	srv    *httptest.Server
	hits   atomic.Int32
	mu     sync.Mutex
	assets map[string][]map[string]any
	status int
	header map[string]string
}

func stubReleaseAPI(t *testing.T) *releaseAPI {
	t.Helper()
	api := &releaseAPI{assets: map[string][]map[string]any{}}
	api.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api.hits.Add(1)
		api.mu.Lock()
		defer api.mu.Unlock()
		if api.status != 0 {
			for k, v := range api.header {
				w.Header().Set(k, v)
			}
			w.WriteHeader(api.status)
			return
		}
		assets, ok := api.assets[strings.TrimPrefix(r.URL.Path, "/repos/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"assets": assets})
	}))
	t.Cleanup(api.srv.Close)
	testx.Swap(t, &newGitHub, func(token string) *fetch.GitHub {
		return &fetch.GitHub{APIBase: api.srv.URL, Token: token, Client: api.srv.Client()}
	})
	return api
}

func (a *releaseAPI) release(repo, tag string, digests map[string]string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var assets []map[string]any
	for name, d := range digests {
		asset := map[string]any{"name": name}
		if d != "" {
			asset["digest"] = "sha256:" + d
		}
		assets = append(assets, asset)
	}
	a.assets[repo+"/releases/tags/"+tag] = assets
}

func (a *releaseAPI) fail(status int, header map[string]string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.status, a.header = status, header
}

func fakeSum(seed string) string {
	var b strings.Builder
	for b.Len() < 64 {
		fmt.Fprintf(&b, "%x", seed)
	}
	return b.String()[:64]
}

func assetDigests(name, version string, platforms []spec.Platform) map[string]string {
	out := map[string]string{}
	for _, p := range platforms {
		out[fmt.Sprintf("%s-%s-%s-%s", name, version, p.OS, p.Arch)] = fakeSum(version + p.String())
	}
	return out
}

func forbidDownloads(t *testing.T) *[]string {
	t.Helper()
	var downloaded []string
	var mu sync.Mutex
	testx.Swap(t, &digestURL, func(_ context.Context, _ *http.Client, url string, _ fetch.Meta, _ report.Task) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		downloaded = append(downloaded, url)
		return fakeSum("downloaded " + url), nil
	})
	return &downloaded
}

func TestBumpUsesReleaseDigests(t *testing.T) {
	api := stubReleaseAPI(t)
	api.release("acme/tool", "v1.1.0", assetDigests("tool", "1.1.0", spec.SupportedPlatforms))
	api.release("acme/tool", "v1.2.0", assetDigests("tool", "1.2.0", spec.SupportedPlatforms))
	downloaded := forbidDownloads(t)
	dir := writeFixture(t, map[string]string{"tool": releaseFixture("tool", jqVersionsYAML("1.0.0"))})
	path := filepath.Join(dir, "pkgs", "tool", "pkg.yaml")
	stubList(t, "1.2.0", "1.1.0", "1.0.0")

	res, errOut, err := runBump(t, Options{GitHubToken: "tok"}, path)
	if err != nil {
		t.Fatalf("bump: %v", err)
	}
	wantCounts(t, res, 1, 0, 0, 0)
	if !strings.Contains(errOut, "Bumped tool 1.0.0 → 1.2.0 (2 versions)") {
		t.Errorf("stderr = %q, want the bumped outcome line", errOut)
	}
	if len(*downloaded) != 0 {
		t.Fatalf("release digests must replace downloads, got %v", *downloaded)
	}
	if got := api.hits.Load(); got != 2 {
		t.Errorf("release API hits = %d, want one per version", got)
	}
	pkg, err := spec.Parse(mustRead(t, path))
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"1.2.0", "1.1.0"} {
		for _, p := range spec.SupportedPlatforms {
			i := 0
			if v == "1.1.0" {
				i = 1
			}
			if got, want := pkg.Versions[i].Sha256[p.String()], fakeSum(v+p.String()); got != want {
				t.Errorf("%s %s sha256 = %q, want release digest %q", v, p, got, want)
			}
		}
	}
}

func TestBumpDownloadsWhenReleaseLacksDigest(t *testing.T) {
	api := stubReleaseAPI(t)
	digests := assetDigests("tool", "1.1.0", spec.SupportedPlatforms)
	digests["tool-1.1.0-linux-arm64"] = ""
	delete(digests, "tool-1.1.0-darwin-amd64")
	api.release("acme/tool", "v1.1.0", digests)
	downloaded := forbidDownloads(t)
	dir := writeFixture(t, map[string]string{"tool": releaseFixture("tool", jqVersionsYAML("1.0.0"))})
	path := filepath.Join(dir, "pkgs", "tool", "pkg.yaml")
	stubList(t, "1.1.0", "1.0.0")

	res, errOut, err := runBumpVerbose(t, Options{}, path)
	if err != nil {
		t.Fatalf("bump: %v", err)
	}
	wantCounts(t, res, 1, 0, 0, 0)
	want := []string{
		"https://github.com/acme/tool/releases/download/v1.1.0/tool-1.1.0-darwin-amd64",
		"https://github.com/acme/tool/releases/download/v1.1.0/tool-1.1.0-linux-arm64",
	}
	if len(*downloaded) != 2 || (*downloaded)[0] != want[0] || (*downloaded)[1] != want[1] {
		t.Fatalf("downloaded = %v, want only the assets without a digest %v", *downloaded, want)
	}
	if got := api.hits.Load(); got != 1 {
		t.Errorf("release API hits = %d, want one per version", got)
	}
	for _, line := range []string{
		"Downloading tool 1.1.0 darwin/amd64: no release digest for tool-1.1.0-darwin-amd64",
		"Downloading tool 1.1.0 linux/arm64: no release digest for tool-1.1.0-linux-arm64",
	} {
		if !strings.Contains(errOut, line) {
			t.Errorf("stderr = %q, want debug line %q", errOut, line)
		}
	}
	pkg, err := spec.Parse(mustRead(t, path))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := pkg.Versions[0].Sha256["linux/amd64"], fakeSum("1.1.0linux/amd64"); got != want {
		t.Errorf("linux/amd64 sha256 = %q, want release digest %q", got, want)
	}
	if got, wantSum := pkg.Versions[0].Sha256["linux/arm64"], fakeSum("downloaded "+want[1]); got != wantSum {
		t.Errorf("linux/arm64 sha256 = %q, want downloaded digest %q", got, wantSum)
	}
}

func TestBumpStopsReleaseLookupsWhenRateLimited(t *testing.T) {
	api := stubReleaseAPI(t)
	api.fail(http.StatusForbidden, map[string]string{"X-RateLimit-Remaining": "0"})
	downloaded := forbidDownloads(t)
	dir := writeFixture(t, map[string]string{"tool": releaseFixture("tool", jqVersionsYAML("1.0.0"))})
	stubList(t, "1.2.0", "1.1.0", "1.0.0")

	res, errOut, err := runBumpVerbose(t, Options{}, dir)
	if err != nil {
		t.Fatalf("bump: %v", err)
	}
	wantCounts(t, res, 1, 0, 0, 0)
	if got := api.hits.Load(); got != 1 {
		t.Errorf("release API hits = %d, want lookups to stop after the first rate limit", got)
	}
	if got, want := len(*downloaded), 2*len(spec.SupportedPlatforms); got != want {
		t.Errorf("downloaded %d artifacts, want every one (%d)", got, want)
	}
	if !strings.Contains(errOut, "GitHub API rate limit exceeded") {
		t.Errorf("stderr = %q, want the rate limit named in a debug line", errOut)
	}
}

func TestBumpDownloadsWhenReleaseLookupFails(t *testing.T) {
	api := stubReleaseAPI(t)
	downloaded := forbidDownloads(t)
	dir := writeFixture(t, map[string]string{"tool": releaseFixture("tool", jqVersionsYAML("1.0.0"))})
	stubList(t, "1.1.0", "1.0.0")

	res, errOut, err := runBumpVerbose(t, Options{}, dir)
	if err != nil {
		t.Fatalf("bump: %v", err)
	}
	wantCounts(t, res, 1, 0, 0, 0)
	if got := api.hits.Load(); got != 1 {
		t.Errorf("release API hits = %d, want the failed lookup cached per version", got)
	}
	if got, want := len(*downloaded), len(spec.SupportedPlatforms); got != want {
		t.Errorf("downloaded %d artifacts, want %d", got, want)
	}
	if !strings.Contains(errOut, "Downloading tool 1.1.0 darwin/arm64: look up release acme/tool v1.1.0: unexpected status 404 Not Found") {
		t.Errorf("stderr = %q, want the lookup failure in a debug line", errOut)
	}
}

func TestBumpHashesSourceFromRelease(t *testing.T) {
	api := stubReleaseAPI(t)
	api.release("acme/lib", "v3.4.2", map[string]string{"lib-3.4.2.tar.gz": fakeSum("source")})
	downloaded := forbidDownloads(t)
	fixture := opensslFixture("https://github.com/acme/lib/releases/download", `/v{{trimPrefix .Version "v"}}/lib-{{trimPrefix .Version "v"}}.tar.gz`,
		`  - version: v3.4.1
    sourceSha256: "002a2d6b30b58bf4bea46c43bdd96365aaf8daa6c428782aa4feee06da197df3"`)
	dir := writeFixture(t, map[string]string{"openssl": fixture})
	path := filepath.Join(dir, "pkgs", "openssl", "pkg.yaml")

	res, _, err := runBump(t, Options{Packages: refs(t, "openssl@v3.4.2")}, path)
	if err != nil {
		t.Fatalf("bump: %v", err)
	}
	wantCounts(t, res, 1, 0, 0, 0)
	if len(*downloaded) != 0 {
		t.Fatalf("source digest must come from the release, got downloads %v", *downloaded)
	}
	pkg, err := spec.Parse(mustRead(t, path))
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Versions[0].SourceSha256 != fakeSum("source") {
		t.Fatalf("versions[0] = %+v", pkg.Versions[0])
	}
}
