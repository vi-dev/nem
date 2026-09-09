package selfupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/vi-dev/nem/internal/archive"
	"github.com/vi-dev/nem/internal/fetch"
	"github.com/vi-dev/nem/internal/netx"
	"github.com/vi-dev/nem/internal/report"
	"github.com/vi-dev/nem/internal/spec"
)

const maxAPIBody = 1 << 20

type Updater struct {
	Client       *http.Client
	APIBase      string
	DownloadBase string
	Repo         string
	Token        string
}

func (u *Updater) Update(ctx context.Context, version, exePath string, task report.Task) error {

	installDir := filepath.Dir(exePath)
	stage, err := os.CreateTemp(installDir, ".nem-update-*")
	if err != nil {
		return fmt.Errorf("install directory %s is not writable; re-run with permission to modify it (e.g. sudo): %w", installDir, err)
	}
	stagePath := stage.Name()
	stage.Close()
	defer os.Remove(stagePath)

	tmpDir, err := os.MkdirTemp("", "nem-selfupdate-")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	checksums, err := u.fetchChecksums(ctx, version)
	if err != nil {
		return err
	}
	asset := assetName(version, runtime.GOOS, runtime.GOARCH)
	wantSHA, err := parseChecksums(checksums, asset)
	if err != nil {
		return fmt.Errorf("release %s: %w", version, err)
	}

	meta := fetch.Meta{Name: "nem", Version: version, Platform: spec.Platform{OS: runtime.GOOS, Arch: runtime.GOARCH}}
	downloaded, err := fetch.Download(ctx, u.Client, u.assetURL(version, asset), wantSHA, tmpDir, meta, task)
	if err != nil {
		return err
	}

	if err := extractBinary(downloaded, stagePath); err != nil {
		return err
	}
	if out, err := exec.CommandContext(ctx, stagePath, "version").CombinedOutput(); err != nil {
		return fmt.Errorf("downloaded nem %s failed to run (%v): %s", version, err, bytes.TrimSpace(out))
	}

	if err := os.Rename(stagePath, exePath); err != nil {
		return fmt.Errorf("replace %s: %w", exePath, err)
	}
	return nil
}

func (u *Updater) assetURL(version, name string) string {
	return u.DownloadBase + "/" + version + "/" + name
}

func (u *Updater) fetchChecksums(ctx context.Context, version string) ([]byte, error) {
	url := u.assetURL(version, "checksums.txt")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request for %s: %w", url, err)
	}
	resp, err := u.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: unexpected status %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAPIBody))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}
	return data, nil
}

func NewUpdater(token string) *Updater {
	const repo = "vi-dev/nem"
	return &Updater{
		Client:       netx.Client(),
		APIBase:      "https://api.github.com",
		DownloadBase: "https://github.com/" + repo + "/releases/download",
		Repo:         repo,
		Token:        token,
	}
}

func ExecutablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate running binary: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", exe, err)
	}
	return resolved, nil
}

func (u *Updater) ResolveLatest(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", u.APIBase, u.Repo)
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := u.apiJSON(ctx, url, &release); err != nil {
		return "", err
	}
	if release.TagName == "" {
		return "", fmt.Errorf("release metadata from %s has no tag_name", url)
	}
	return release.TagName, nil
}

func (u *Updater) ResolveUnstableCommit(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/commits/unstable", u.APIBase, u.Repo)
	var commit struct {
		SHA string `json:"sha"`
	}
	if err := u.apiJSON(ctx, url, &commit); err != nil {
		return "", err
	}
	if commit.SHA == "" {
		return "", fmt.Errorf("commit metadata from %s has no sha", url)
	}
	return commit.SHA, nil
}

func (u *Updater) apiJSON(ctx context.Context, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request for %s: %w", url, err)
	}
	if u.Token != "" {
		req.Header.Set("Authorization", "Bearer "+u.Token)
	}
	resp, err := u.Client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch %s: unexpected status %s", url, resp.Status)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxAPIBody)).Decode(v); err != nil {
		return fmt.Errorf("decode response from %s: %w", url, err)
	}
	return nil
}

func assetName(version, goos, goarch string) string {
	return fmt.Sprintf("nem_%s_%s_%s.tar.gz", version, goos, goarch)
}

func extractBinary(archivePath, destPath string) error {
	dir, err := os.MkdirTemp("", "nem-update-*")
	if err != nil {
		return fmt.Errorf("create extraction dir: %w", err)
	}
	defer os.RemoveAll(dir)
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("open extraction dir: %w", err)
	}
	defer root.Close()
	if _, err := archive.Extract(archivePath, root, archive.Options{SingleName: "nem"}); err != nil {
		return fmt.Errorf("extract release archive: %w", err)
	}

	var src string
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "nem" {
			src = path
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("search release archive: %w", err)
	}
	if src == "" {
		return errors.New("no nem binary found in release archive")
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open extracted binary: %w", err)
	}
	defer in.Close()
	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("create %s: %w", destPath, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("write %s: %w", destPath, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close %s: %w", destPath, err)
	}

	if err := os.Chmod(destPath, 0o755); err != nil {
		return fmt.Errorf("chmod %s: %w", destPath, err)
	}
	return nil
}

func parseChecksums(data []byte, filename string) (string, error) {
	for line := range strings.Lines(string(data)) {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == filename {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("no checksum entry for %s", filename)
}
