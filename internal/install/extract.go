package install

import (
	"os"

	extractpkg "github.com/vi-dev/nem/internal/extract"
	"github.com/vi-dev/nem/internal/spec"
)

func extract(artifactPath string, root *os.Root, strip int, singleName string) error {
	_, err := extractpkg.Extract(artifactPath, root, extractpkg.Options{Strip: strip, SingleName: singleName})
	return err
}

func singleFileName(r *spec.Recipe, pkg *spec.Package, version string, plat spec.Platform) string {
	scoped := *pkg
	scoped.Artifact = r.Artifact
	if r.Artifact.GitHub != nil {
		name, err := scoped.AssetName(version, plat)
		if err != nil {
			return ""
		}
		return extractpkg.SingleNameFromRef(name)
	}
	if r.Artifact.URL == "" {
		return ""
	}
	raw, err := scoped.ArtifactURL(version, plat)
	if err != nil {
		return ""
	}
	return extractpkg.SingleNameFromRef(raw)
}
