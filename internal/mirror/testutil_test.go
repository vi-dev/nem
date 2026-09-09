package mirror

import (
	"fmt"
	"testing"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"

	"github.com/vi-dev/nem/internal/publish/publishtest"
	"github.com/vi-dev/nem/internal/testx"
)

func newCatalog(t *testing.T, pkgs map[string]string) (oras.Target, string) {
	t.Helper()
	target := memory.New()
	publishtest.PublishCatalog(t, target, "example.com/cat", pkgs)
	return target, "v2"
}

func wireCatalog(t *testing.T, src oras.ReadOnlyTarget, srcTag string, dst oras.Target, dstTag string) *bool {
	t.Helper()
	var dstOpened bool
	t.Cleanup(SetSrcCatalogOpener(func(string) (oras.ReadOnlyTarget, string, error) {
		return src, srcTag, nil
	}))
	t.Cleanup(SetDstCatalogOpener(func(string) (oras.Target, string, error) {
		dstOpened = true
		return dst, dstTag, nil
	}))
	return &dstOpened
}

func wireArchives(t *testing.T, src, dst *testx.ArchiveFixtures) {
	t.Helper()
	t.Cleanup(SetSrcArchivesOpener(func(_, name string) (oras.ReadOnlyTarget, error) {
		return src.Open(name), nil
	}))
	t.Cleanup(SetDstArchivesOpener(func(_, name string) (oras.Target, error) {
		return dst.Open(name), nil
	}))
}

func absoluteOCIPkgYAML(name, ociRef, version string) string {
	return fmt.Sprintf("schema: 2\nname: %s\ndescription: test package\nartifact:\n  oci: %q\ninstall:\n  - extract: {strip: 0}\nversions:\n  - version: %s\n", name, ociRef, version)
}
