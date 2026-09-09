package mirror

import (
	"context"
	"testing"

	"oras.land/oras-go/v2/content/memory"

	"github.com/vi-dev/nem/internal/ocix/ocixtest"
	"github.com/vi-dev/nem/internal/spec"
	"github.com/vi-dev/nem/internal/testx"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		name             string
		pkg              *spec.Package
		wantParticipates bool
		wantPrebuilt     bool
	}{
		{"url", &spec.Package{Artifact: spec.Artifact{URL: "https://example.com/{{.Version}}"}}, true, false},
		{"github", &spec.Package{Artifact: spec.Artifact{GitHub: &spec.GitHubAsset{Repo: "a/b"}}}, true, false},
		{"oci relative", &spec.Package{Artifact: spec.Artifact{OCI: ":{{.Version}}"}}, true, true},
		{"oci relative digest", &spec.Package{Artifact: spec.Artifact{OCI: "@{{.Version}}"}}, true, true},
		{"oci absolute", &spec.Package{Artifact: spec.Artifact{OCI: "ghcr.io/other/pkg:{{.Version}}"}}, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotParticipates, gotPrebuilt := classify(c.pkg)
			if gotParticipates != c.wantParticipates || gotPrebuilt != c.wantPrebuilt {
				t.Errorf("classify() = (%v, %v), want (%v, %v)", gotParticipates, gotPrebuilt, c.wantParticipates, c.wantPrebuilt)
			}
		})
	}
}

func TestMirrorVersionDryRunCopiesNothing(t *testing.T) {
	ctx := context.Background()
	src := memory.New()
	ocixtest.PushFakeArchive(t, src, "1.0.0", map[string][]byte{"linux/amd64": []byte("payload")})
	dst := memory.New()

	rep := &testx.Reporter{}
	got := mirrorVersion(ctx, src, dst, "go", "1.0.0", false, true, rep, rep.Task("test"))
	if got != outcomeCopied {
		t.Fatalf("outcome = %v, want outcomeCopied (would-copy still counts)", got)
	}
	if _, err := dst.Resolve(ctx, "1.0.0"); err == nil {
		t.Fatal("dry run must not write to dst")
	}
	if len(rep.Infos()) != 0 {
		t.Fatalf("infos = %v, want none: per-item progress must not print", rep.Infos())
	}
	statuses := rep.TaskFor("test").Statuses()
	if len(statuses) != 1 || statuses[0] != "would copy 1.0.0" {
		t.Fatalf("statuses = %v, want [\"would copy 1.0.0\"]", statuses)
	}
}
