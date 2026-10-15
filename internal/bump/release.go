package bump

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/vi-dev/nem/internal/fetch"
	"github.com/vi-dev/nem/internal/netx"
	"github.com/vi-dev/nem/internal/report"
)

var newGitHub = func(token string) *fetch.GitHub {
	return &fetch.GitHub{Token: token, Client: netx.Client()}
}

var errNotGitHubRelease = errors.New("not a GitHub release asset")

type releases struct {
	gh       *fetch.GitHub
	mu       sync.Mutex
	cache    map[string]*release
	disabled atomic.Bool
}

type release struct {
	once    sync.Once
	digests map[string]string
	err     error
}

func newReleases(token string) *releases {
	return &releases{gh: newGitHub(token), cache: map[string]*release{}}
}

func (r *releases) digest(ctx context.Context, rawURL string) (string, error) {
	asset, ok := fetch.ParseGitHubAssetURL(rawURL)
	if !ok {
		return "", errNotGitHubRelease
	}
	digests, err := r.lookup(ctx, asset.Repo, asset.Tag)
	if err != nil {
		return "", err
	}
	sum, ok := digests[asset.Name]
	if !ok {
		return "", fmt.Errorf("no release digest for %s", asset.Name)
	}
	return sum, nil
}

func (r *releases) lookup(ctx context.Context, repo, tag string) (map[string]string, error) {
	key := repo + "@" + tag
	r.mu.Lock()
	ent, ok := r.cache[key]
	if !ok {
		ent = &release{}
		r.cache[key] = ent
	}
	r.mu.Unlock()
	ent.once.Do(func() {
		if r.disabled.Load() {
			ent.err = &fetch.RateLimitError{}
			return
		}
		ent.digests, ent.err = r.gh.ReleaseDigests(ctx, repo, tag)
		if _, ok := errors.AsType[*fetch.RateLimitError](ent.err); ok {
			r.disabled.Store(true)
		}
	})
	return ent.digests, ent.err
}

func hashArtifact(ctx context.Context, rel *releases, url, what string, fmeta fetch.Meta, task report.Task) (string, error) {
	sum, err := rel.digest(ctx, url)
	if err == nil {
		return sum, nil
	}
	if !errors.Is(err, errNotGitHubRelease) {
		report.FromContext(ctx).Debug("Downloading %s %s %s: %v", fmeta.Name, fmeta.Version, what, err)
	}
	return digestURL(ctx, netx.Client(), url, fmeta, task)
}
