package mirror

import (
	"fmt"
	"strings"
	"testing"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"

	"github.com/vi-dev/nem/internal/archive"
	"github.com/vi-dev/nem/internal/publish/publishtest"
	"github.com/vi-dev/nem/internal/testx"
)

const (
	testSrcCatalogRef = "example.com/cat:v2"
	testDstCatalogRef = "internal.example.com/cat:v2"
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

func repoOpenerFor(t *testing.T, srcRef, dstRef string, src, dst *testx.ArchiveFixtures) func(string) (oras.Target, error) {
	t.Helper()
	bases := map[string]*testx.ArchiveFixtures{}
	for ref, fx := range map[string]*testx.ArchiveFixtures{srcRef: src, dstRef: dst} {
		base, err := archive.RefPrefix(ref)
		if err != nil {
			t.Fatalf("archive.RefPrefix(%q): %v", ref, err)
		}
		bases[base] = fx
	}
	return func(ref string) (oras.Target, error) {
		for base, fx := range bases {
			if name, ok := strings.CutPrefix(ref, base); ok {
				return fx.Open(name), nil
			}
		}
		return nil, fmt.Errorf("unexpected archives ref %s", ref)
	}
}

func wireArchives(t *testing.T, src, dst *testx.ArchiveFixtures) {
	t.Helper()
	t.Cleanup(archive.SetRepoOpener(repoOpenerFor(t, testSrcCatalogRef, testDstCatalogRef, src, dst)))
}

func absoluteOCIPkgYAML(name, ociRef, version string) string {
	return fmt.Sprintf("schema: 2\nname: %s\ndescription: test package\nartifact:\n  oci: %q\ninstall:\n  - extract: {strip: 0}\nversions:\n  - version: %s\n", name, ociRef, version)
}
