package fetch

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type GitHubAsset struct {
	Repo, Tag, Name string
}

func ParseGitHubAssetURL(rawURL string) (GitHubAsset, bool) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" {
		return GitHubAsset{}, false
	}
	parts := strings.Split(strings.TrimPrefix(u.EscapedPath(), "/"), "/")
	if len(parts) != 6 || parts[2] != "releases" || parts[3] != "download" {
		return GitHubAsset{}, false
	}
	for i, p := range parts {
		decoded, err := url.PathUnescape(p)
		if err != nil || decoded == "" {
			return GitHubAsset{}, false
		}
		parts[i] = decoded
	}
	return GitHubAsset{Repo: parts[0] + "/" + parts[1], Tag: parts[4], Name: parts[5]}, true
}

type GitHub struct {
	APIBase string
	Token   string
	Client  *http.Client
}

type RateLimitError struct{}

func (*RateLimitError) Error() string { return "GitHub API rate limit exceeded" }

func (g *GitHub) ReleaseDigests(ctx context.Context, repo, tag string) (map[string]string, error) {
	base := g.APIBase
	if base == "" {
		base = "https://api.github.com"
	}
	client := g.Client
	if client == nil {
		client = http.DefaultClient
	}
	endpoint := fmt.Sprintf("%s/repos/%s/releases/tags/%s", base, repo, url.PathEscape(tag))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request for %s: %w", endpoint, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("look up release %s %s: %w", repo, tag, err)
	}
	defer resp.Body.Close()
	if rateLimited(resp) {
		return nil, &RateLimitError{}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("look up release %s %s: unexpected status %s", repo, tag, resp.Status)
	}
	var release struct {
		Assets []struct {
			Name   string `json:"name"`
			Digest string `json:"digest"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("look up release %s %s: %w", repo, tag, err)
	}
	out := make(map[string]string, len(release.Assets))
	for _, a := range release.Assets {
		sum, ok := strings.CutPrefix(a.Digest, "sha256:")
		if !ok || len(sum) != hex.EncodedLen(32) {
			continue
		}
		if _, err := hex.DecodeString(sum); err != nil {
			continue
		}
		out[a.Name] = sum
	}
	return out, nil
}

func rateLimited(resp *http.Response) bool {
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		return true
	case http.StatusForbidden:
		return resp.Header.Get("X-RateLimit-Remaining") == "0" || resp.Header.Get("Retry-After") != ""
	}
	return false
}
