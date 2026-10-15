package fetch_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vi-dev/nem/internal/fetch"
)

func TestParseGitHubAssetURL(t *testing.T) {
	cases := []struct {
		url  string
		want fetch.GitHubAsset
		ok   bool
	}{
		{"https://github.com/jqlang/jq/releases/download/jq-1.9.0/jq-linux-amd64", fetch.GitHubAsset{Repo: "jqlang/jq", Tag: "jq-1.9.0", Name: "jq-linux-amd64"}, true},
		{"https://github.com/fatedier/frp/releases/download/v0.71.1/frp_0.71.1_linux_amd64.tar.gz", fetch.GitHubAsset{Repo: "fatedier/frp", Tag: "v0.71.1", Name: "frp_0.71.1_linux_amd64.tar.gz"}, true},
		{"https://github.com/owner/repo/archive/refs/tags/v1.0.0.tar.gz", fetch.GitHubAsset{}, false},
		{"https://github.com/owner/repo/releases/latest/download/tool.tar.gz", fetch.GitHubAsset{}, false},
		{"https://github.com/owner/repo/releases/download/v1/", fetch.GitHubAsset{}, false},
		{"https://example.com/owner/repo/releases/download/v1/tool.tar.gz", fetch.GitHubAsset{}, false},
		{"http://github.com/owner/repo/releases/download/v1/tool.tar.gz", fetch.GitHubAsset{}, false},
		{"https://github.com/owner/repo/releases/download/v1/tool.tar.gz?x=1", fetch.GitHubAsset{Repo: "owner/repo", Tag: "v1", Name: "tool.tar.gz"}, true},
		{"https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv5.7.1/kustomize_v5.7.1_linux_amd64.tar.gz", fetch.GitHubAsset{Repo: "kubernetes-sigs/kustomize", Tag: "kustomize/v5.7.1", Name: "kustomize_v5.7.1_linux_amd64.tar.gz"}, true},
		{"https://github.com/owner/repo/releases/download/v1/tool%20x.tar.gz", fetch.GitHubAsset{Repo: "owner/repo", Tag: "v1", Name: "tool x.tar.gz"}, true},
		{"https://github.com/owner/repo/releases/download/v1/%zz", fetch.GitHubAsset{}, false},
		{"://bad", fetch.GitHubAsset{}, false},
	}
	for _, c := range cases {
		got, ok := fetch.ParseGitHubAssetURL(c.url)
		if ok != c.ok || got != c.want {
			t.Errorf("ParseGitHubAssetURL(%q) = %+v, %v; want %+v, %v", c.url, got, ok, c.want, c.ok)
		}
	}
}

func releaseServer(t *testing.T, handler http.HandlerFunc) *fetch.GitHub {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &fetch.GitHub{APIBase: srv.URL, Client: srv.Client()}
}

func TestReleaseDigests(t *testing.T) {
	var gotPath, gotAuth, gotAccept string
	gh := releaseServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth, gotAccept = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"tag_name":"jq-1.9.0","assets":[
			{"name":"jq-linux-amd64","digest":"sha256:` + strings.Repeat("a", 64) + `"},
			{"name":"jq-linux-arm64","digest":null},
			{"name":"jq-darwin-arm64","digest":"sha512:` + strings.Repeat("b", 128) + `"},
			{"name":"jq-darwin-amd64","digest":"sha256:nothex"},
			{"name":"jq-windows-amd64.exe"}
		]}`))
	})
	gh.Token = "secret"

	got, err := gh.ReleaseDigests(context.Background(), "jqlang/jq", "jq-1.9.0")
	if err != nil {
		t.Fatalf("ReleaseDigests: %v", err)
	}
	if gotPath != "/repos/jqlang/jq/releases/tags/jq-1.9.0" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAuth != "Bearer secret" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotAccept != "application/vnd.github+json" {
		t.Errorf("Accept = %q", gotAccept)
	}
	want := map[string]string{"jq-linux-amd64": strings.Repeat("a", 64)}
	if len(got) != len(want) || got["jq-linux-amd64"] != want["jq-linux-amd64"] {
		t.Errorf("digests = %v, want %v", got, want)
	}
}

func TestReleaseDigestsWithoutToken(t *testing.T) {
	var gotAuth string
	gotAuth = "unset"
	gh := releaseServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte(`{"assets":[]}`))
	})
	if _, err := gh.ReleaseDigests(context.Background(), "o/r", "v1"); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "" {
		t.Errorf("Authorization = %q, want none", gotAuth)
	}
}

func TestReleaseDigestsEscapesTag(t *testing.T) {
	var gotPath string
	gh := releaseServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.Write([]byte(`{"assets":[]}`))
	})
	if _, err := gh.ReleaseDigests(context.Background(), "o/r", "kustomize/v1+build"); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/repos/o/r/releases/tags/kustomize%2Fv1+build" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestReleaseDigestsRateLimit(t *testing.T) {
	cases := []struct {
		name   string
		status int
		header map[string]string
		want   bool
	}{
		{"primary limit", http.StatusForbidden, map[string]string{"X-RateLimit-Remaining": "0"}, true},
		{"secondary limit", http.StatusForbidden, map[string]string{"Retry-After": "60"}, true},
		{"too many requests", http.StatusTooManyRequests, nil, true},
		{"plain forbidden", http.StatusForbidden, map[string]string{"X-RateLimit-Remaining": "12"}, false},
		{"not found", http.StatusNotFound, nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gh := releaseServer(t, func(w http.ResponseWriter, r *http.Request) {
				for k, v := range c.header {
					w.Header().Set(k, v)
				}
				w.WriteHeader(c.status)
			})
			_, err := gh.ReleaseDigests(context.Background(), "o/r", "v1")
			if err == nil {
				t.Fatal("want error")
			}
			var rl *fetch.RateLimitError
			if got := errors.As(err, &rl); got != c.want {
				t.Errorf("RateLimitError = %v, want %v (err: %v)", got, c.want, err)
			}
			if !c.want && !strings.Contains(err.Error(), "o/r v1") {
				t.Errorf("error %q should name the release", err)
			}
		})
	}
}

func TestReleaseDigestsMalformedBody(t *testing.T) {
	gh := releaseServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"assets": "nope"`))
	})
	if _, err := gh.ReleaseDigests(context.Background(), "o/r", "v1"); err == nil {
		t.Fatal("want error for malformed body")
	}
}
