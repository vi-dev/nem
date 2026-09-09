package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/template"

	"oras.land/oras-go/v2/registry"

	"github.com/vi-dev/nem/internal/netx"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/report"
	"github.com/vi-dev/nem/internal/spec"
)

type Source struct {
	CatalogRef string
}

var httpClient = netx.Client()

var pullArchive = func(ctx context.Context, catalogRef, name, tag string, plat spec.Platform, dir string) (string, error) {
	repo, err := ocix.RemoteArchives(catalogRef, name)
	if err != nil {
		return "", err
	}
	return ocix.PullArchiveFrom(ctx, repo, tag, plat, dir)
}

func SetPullArchive(f func(ctx context.Context, catalogRef, name, tag string, plat spec.Platform, dir string) (string, error)) (restore func()) {
	prev := pullArchive
	pullArchive = f
	return func() { pullArchive = prev }
}

var remoteByRef = func(ctx context.Context, ref string, plat spec.Platform, dir string) (string, error) {
	parsed, err := registry.ParseReference(ref)
	if err != nil {
		return "", fmt.Errorf("parse oci ref %q: %w", ref, err)
	}
	repo, err := ocix.NewRemoteRepository(parsed.Registry + "/" + parsed.Repository)
	if err != nil {
		return "", err
	}
	return ocix.PullArchiveFrom(ctx, repo, parsed.ReferenceOrDefault(), plat, dir)
}

func Acquire(ctx context.Context, pkg *spec.Package, version string, plat spec.Platform, src Source, dir string, task report.Task) (string, error) {
	if pkg.Artifact.OCI != "" {
		return acquireOCI(ctx, pkg, version, plat, src, dir)
	}

	meta := Meta{Name: pkg.Name, Version: version, Platform: plat}
	sha, err := pkg.Sha256(version, plat)
	if err != nil {
		return "", err
	}

	if src.CatalogRef != "" {
		path, err := pullArchive(ctx, src.CatalogRef, pkg.Name, version, plat, dir)
		switch {
		case err == nil:
			if verr := VerifyFile(path, sha, meta); verr != nil {
				os.Remove(path)
				return "", verr
			}
			return path, nil
		case errors.Is(err, ocix.ErrArchiveNotFound):

		default:
			return "", err
		}
	}

	url, err := UpstreamURL(pkg, version, plat)
	if err != nil {
		return "", err
	}
	return Download(ctx, httpClient, url, sha, dir, meta, task)
}

func acquireOCI(ctx context.Context, pkg *spec.Package, version string, plat spec.Platform, src Source, dir string) (string, error) {
	ref, err := templateOCIRef(pkg.Artifact.OCI, version, plat)
	if err != nil {
		return "", err
	}
	if isRelativeOCIRef(ref) {
		if src.CatalogRef == "" {
			return "", errors.New("relative oci ref requires an oci catalog")
		}
		return pullArchive(ctx, src.CatalogRef, pkg.Name, ociRefTag(ref), plat, dir)
	}
	return remoteByRef(ctx, ref, plat, dir)
}

type ociTemplateCtx struct {
	Version, OS, Arch string
}

var ociHelperFuncs = template.FuncMap{
	"trimPrefix": func(s, prefix string) string { return strings.TrimPrefix(s, prefix) },
	"trimSuffix": func(s, suffix string) string { return strings.TrimSuffix(s, suffix) },
	"replace":    func(s, old, new string) string { return strings.ReplaceAll(s, old, new) },
}

func templateOCIRef(tmpl, version string, plat spec.Platform) (string, error) {
	t, err := template.New("").Funcs(ociHelperFuncs).Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse oci ref template %q: %w", tmpl, err)
	}
	var b strings.Builder
	if err := t.Execute(&b, ociTemplateCtx{Version: version, OS: plat.OS, Arch: plat.Arch}); err != nil {
		return "", fmt.Errorf("expand oci ref template %q: %w", tmpl, err)
	}
	return b.String(), nil
}

func isRelativeOCIRef(ref string) bool {
	return ref == "" || strings.HasPrefix(ref, ":") || strings.HasPrefix(ref, "@")
}

func ociRefTag(ref string) string {
	if ref == "" {
		return ref
	}
	return ref[1:]
}

func VerifyFile(path, wantSHA256 string, meta Meta) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return fmt.Errorf("hash %s: %w", path, err)
	}
	got := hex.EncodeToString(sum.Sum(nil))
	if got != wantSHA256 {
		return &ChecksumMismatchError{Name: meta.Name, Version: meta.Version, Platform: meta.Platform.String(), Got: got, Want: wantSHA256}
	}
	return nil
}
