package install

import (
	"os"

	"github.com/vi-dev/nem/internal/archive"
	"github.com/vi-dev/nem/internal/spec"
)

func extract(artifactPath string, root *os.Root, strip int, singleName string) error {
	_, err := archive.Extract(artifactPath, root, archive.Options{Strip: strip, SingleName: singleName})
	return err
}

func singleFileName(pkg *spec.Package, version string, plat spec.Platform) string {
	if pkg.Artifact.GitHub != nil {
		name, err := pkg.AssetName(version, plat)
		if err != nil {
			return ""
		}
		return archive.SingleNameFromRef(name)
	}
	if pkg.Artifact.URL == "" {
		return ""
	}
	raw, err := pkg.ArtifactURL(version, plat)
	if err != nil {
		return ""
	}
	return archive.SingleNameFromRef(raw)
}
