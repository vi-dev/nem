package fill

import (
	"context"
	"testing"

	"oras.land/oras-go/v2/content/memory"

	"github.com/vi-dev/nem/internal/spec"
	"github.com/vi-dev/nem/internal/testx"
)

func testPkg(t *testing.T, urlTemplate, digestHex string) *spec.Package {
	t.Helper()
	pkg, err := spec.Parse([]byte(urlPkg("go", "1.0.0", urlTemplate, digestHex)))
	if err != nil {
		t.Fatalf("parse test pkg.yaml: %v", err)
	}
	return pkg
}

func TestFillItemShaLookupFailureWarnsAndFails(t *testing.T) {
	up := testx.NewFileServer(t)
	wireUpstream(t, up)
	pkg := testPkg(t, up.URL+"/{{.Version}}", "deadbeef")
	archives := memory.New()

	rep := &testx.Reporter{}
	got, _ := fillItem(context.Background(), newHome(t), archives, pkg, "9.9.9", spec.SupportedPlatforms[0], false, rep, rep.Task("test"))
	if got != outcomeFailed {
		t.Fatalf("outcome = %v, want outcomeFailed for an undeclared version", got)
	}
	if len(rep.Warns()) != 1 {
		t.Fatalf("warns = %v, want exactly one", rep.Warns())
	}
	if up.Hits() != 0 {
		t.Fatal("a sha lookup failure must never reach upstream")
	}
}
