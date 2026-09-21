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

func singleFileName(pkg *spec.Package, version string, plat spec.Platform) string {
	if pkg.Artifact.GitHub != nil {
		name, err := pkg.AssetName(version, plat)
		if err != nil {
			return ""
		}
		return extractpkg.SingleNameFromRef(name)
	}
	if pkg.Artifact.URL == "" {
		return ""
	}
	raw, err := pkg.ArtifactURL(version, plat)
	if err != nil {
		return ""
	}
	return extractpkg.SingleNameFromRef(raw)
}
