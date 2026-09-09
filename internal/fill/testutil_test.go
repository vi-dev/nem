package fill

import (
	"os"
	"testing"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"

	"github.com/vi-dev/nem/internal/home"
	"github.com/vi-dev/nem/internal/publish/publishtest"
	"github.com/vi-dev/nem/internal/testx"
)

func newCatalog(t *testing.T, pkgs map[string]string) (oras.Target, string) {
	t.Helper()
	target := memory.New()
	publishtest.PublishCatalog(t, target, "example.com/cat", pkgs)
	return target, "v2"
}

func wireCatalog(t *testing.T, src oras.ReadOnlyTarget, srcTag string) {
	t.Helper()
	t.Cleanup(SetCatalogOpener(func(string) (oras.ReadOnlyTarget, string, error) {
		return src, srcTag, nil
	}))
}

func wireArchives(t *testing.T, fixtures *testx.ArchiveFixtures) {
	t.Helper()
	t.Cleanup(SetArchivesOpener(func(_, name string) (oras.Target, error) {
		return fixtures.Open(name), nil
	}))
}

func wireUpstream(t *testing.T, u *testx.FileServer) {
	t.Helper()
	t.Cleanup(SetHTTPClient(u.Client()))
}

func urlPkg(name, version, urlTemplate, digestHex string) string {
	return string(publishtest.URLPkgYAML(name, version, urlTemplate, publishtest.UniformSha256(digestHex)))
}

func ociPkg(name, version string) string {
	return string(publishtest.OCIPkgYAML(name, version))
}

func newHome(t *testing.T) home.Home {
	t.Helper()
	h := bareHome(t)
	if err := os.MkdirAll(h.Tmp(), 0o755); err != nil {
		t.Fatalf("mkdir tmp: %v", err)
	}
	return h
}

func bareHome(t *testing.T) home.Home {
	t.Helper()
	return testx.Home(t)
}
