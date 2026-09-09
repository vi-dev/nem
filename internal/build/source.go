package build

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/vi-dev/nem/internal/archive"
	"github.com/vi-dev/nem/internal/fetch"
	"github.com/vi-dev/nem/internal/spec"
)

func fetchSource(ctx context.Context, client *http.Client, url, wantSHA256, dir string, meta fetch.Meta) (path, sha256sum string, verified bool, err error) {
	if wantSHA256 != "" {
		path, err := fetch.Download(ctx, client, url, wantSHA256, dir, meta, nil)
		if err != nil {
			return "", "", false, err
		}
		return path, wantSHA256, true, nil
	}
	path, sum, err := fetch.DownloadUnverified(ctx, client, url, dir, meta, nil)
	if err != nil {
		return "", "", false, err
	}
	return path, sum, false, nil
}

func unpackSource(archivePath, destDir, singleName string) (root string, err error) {
	dirRoot, err := os.OpenRoot(destDir)
	if err != nil {
		return "", fmt.Errorf("open destination dir %s: %w", destDir, err)
	}
	defer dirRoot.Close()

	res, err := archive.Extract(archivePath, dirRoot, archive.Options{SingleName: singleName})
	if err != nil {
		return "", err
	}
	if res.CommonPrefix != "" {
		return filepath.Join(destDir, res.CommonPrefix), nil
	}
	return destDir, nil
}

func sourceSingleName(pkg *spec.Package, version string) string {
	url, err := pkg.BuildSourceURL(version, spec.Current())
	if err != nil {
		return ""
	}
	return archive.SingleNameFromRef(url)
}
