package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"oras.land/oras-go/v2/registry/remote/errcode"

	"github.com/vi-dev/nem/internal/catalog"
	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/resolve"
)

func hintFor(err error) string {
	if errors.Is(err, ocix.ErrNotSynced) {
		return "Run `nem catalog update`"
	}
	if _, ok := errors.AsType[*catalog.NotFoundError](err); ok {
		return "Run `nem catalog add <name> <ref>`"
	}
	if _, ok := errors.AsType[*catalog.PackageNotFoundError](err); ok {
		return "Check the package name or run `nem catalog update`"
	}
	if _, ok := errors.AsType[*resolve.UnsupportedPlatformError](err); ok {
		return "This package supports none of nem's platforms"
	}
	if _, ok := errors.AsType[*catalog.DigestMismatchError](err); ok {
		return "Re-lock with `nem use <pkg>@<version>`"
	}
	if _, ok := errors.AsType[*UnpinnedToolsError](err); ok {
		return "Pin exact versions in nem.toml or run `nem use <pkg>@<version>`"
	}
	if pce, ok := errors.AsType[*resolve.PinConflictError](err); ok {
		return fmt.Sprintf("Re-pin with `nem use %s@%s` or unuse the tool requiring it", pce.Name, pce.Required)
	}
	if sce, ok := errors.AsType[*resolve.CompatConflictError](err); ok {
		return fmt.Sprintf("Unuse one of the conflicting tools, or re-pin %s to a version they all accept", sce.Name)
	}
	var eresp *errcode.ErrorResponse
	if errors.As(err, &eresp) && eresp.StatusCode == http.StatusUnauthorized {
		if eresp.URL != nil && eresp.URL.Host != "" {
			return fmt.Sprintf("Run `docker login %s`", eresp.URL.Host)
		}
		return "Run `docker login`"
	}
	if _, ok := errors.AsType[*url.Error](err); ok {
		return "Check your network connection or proxy settings"
	}
	return ""
}
