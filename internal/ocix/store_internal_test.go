package ocix

import (
	"slices"
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

func namedDesc(name, id string) ocispec.Descriptor {
	d := ocispec.Descriptor{ArtifactType: id}
	if name != "" {
		d.Annotations = map[string]string{AnnotationTitle: name}
	}
	return d
}

func TestEnumeratePackagesSortsByName(t *testing.T) {
	idx := ocispec.Index{Manifests: []ocispec.Descriptor{
		namedDesc("kubectl", "1"),
		namedDesc("go", "2"),
		namedDesc("erlang", "3"),
	}}

	got := enumerateTitledManifests(idx)

	var names []string
	for _, nm := range got {
		names = append(names, nm.Title)
	}
	want := []string{"erlang", "go", "kubectl"}
	if !slices.Equal(names, want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
}

func TestEnumeratePackagesDedupesFirstOccurrenceWins(t *testing.T) {
	idx := ocispec.Index{Manifests: []ocispec.Descriptor{
		namedDesc("go", "first"),
		namedDesc("go", "second"),
	}}

	got := enumerateTitledManifests(idx)

	if len(got) != 1 {
		t.Fatalf("got %d entries, want exactly 1 for a duplicated name", len(got))
	}
	if got[0].Desc.ArtifactType != "first" {
		t.Fatalf("got entry %q, want the first occurrence", got[0].Desc.ArtifactType)
	}
}

func TestEnumeratePackagesSkipsManifestsWithoutTitle(t *testing.T) {
	idx := ocispec.Index{Manifests: []ocispec.Descriptor{
		namedDesc("", "untitled"),
		namedDesc("go", "go"),
	}}

	got := enumerateTitledManifests(idx)

	if len(got) != 1 || got[0].Title != "go" {
		t.Fatalf("got %+v, want exactly one entry for go", got)
	}
}
