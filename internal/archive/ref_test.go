package archive_test

import (
	"testing"

	"github.com/vi-dev/nem/internal/archive"
)

func TestRef(t *testing.T) {
	cases := []struct {
		name       string
		catalogRef string
		pkgName    string
		want       string
		wantErr    bool
	}{
		{
			name:       "with tag",
			catalogRef: "ghcr.io/org/cat:v2",
			pkgName:    "go",
			want:       "ghcr.io/org/cat/archives/go",
		},
		{
			name:       "with digest",
			catalogRef: "ghcr.io/org/cat@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			pkgName:    "go",
			want:       "ghcr.io/org/cat/archives/go",
		},
		{
			name:       "no slash",
			catalogRef: "cat:v2",
			pkgName:    "go",
			wantErr:    true,
		},
		{
			name:       "empty name",
			catalogRef: "ghcr.io/org/cat:v2",
			pkgName:    "",
			wantErr:    true,
		},
		{
			name:       "name with slash",
			catalogRef: "ghcr.io/org/cat:v2",
			pkgName:    "a/b",
			wantErr:    true,
		},
		{
			name:       "dot dot name",
			catalogRef: "ghcr.io/org/cat:v2",
			pkgName:    "..",
			wantErr:    true,
		},
		{
			name:       "uppercase name",
			catalogRef: "ghcr.io/org/cat:v2",
			pkgName:    "Tool",
			wantErr:    true,
		},
		{
			name:       "name with space",
			catalogRef: "ghcr.io/org/cat:v2",
			pkgName:    "a b",
			wantErr:    true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := archive.Ref(c.catalogRef, c.pkgName)
			if c.wantErr {
				if err == nil {
					t.Fatalf("Ref(%q): want error, got nil", c.catalogRef)
				}
				return
			}
			if err != nil {
				t.Fatalf("Ref(%q): unexpected error: %v", c.catalogRef, err)
			}
			if got != c.want {
				t.Fatalf("Ref(%q) = %q, want %q", c.catalogRef, got, c.want)
			}
		})
	}
}

func TestRefPrefix(t *testing.T) {
	got, err := archive.RefPrefix("ghcr.io/org/cat:v2")
	if err != nil {
		t.Fatalf("RefPrefix: %v", err)
	}
	if got != "ghcr.io/org/cat/archives/" {
		t.Fatalf("RefPrefix = %q, want %q", got, "ghcr.io/org/cat/archives/")
	}
	if _, err := archive.RefPrefix("cat:v2"); err == nil {
		t.Fatal("RefPrefix must reject a ref without a registry")
	}
}
