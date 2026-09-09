package publishtest

import (
	"context"
	"testing"

	"oras.land/oras-go/v2/content/memory"

	"github.com/vi-dev/nem/internal/ocix"
)

func TestPublishCatalogPublishesBothArtifactKinds(t *testing.T) {
	target := memory.New()
	pkgs := map[string]string{
		"urltool": string(URLPkgYAML("urltool", "1.0.0", "https://example.test/{{.Version}}/{{.OS}}-{{.Arch}}.tar.gz", UniformSha256("aaa"))),
		"ocitool": string(OCIPkgYAML("ocitool", "1.0.0")),
	}
	PublishCatalog(t, target, "example.test/cat", pkgs)

	idx, _, err := ocix.FetchCatalogIndex(context.Background(), target, "v2")
	if err != nil {
		t.Fatalf("FetchIndex: %v", err)
	}
	if len(idx.Manifests) != 2 {
		t.Fatalf("manifests = %d, want 2", len(idx.Manifests))
	}
}
