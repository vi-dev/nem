package catalog

import (
	"strings"
	"testing"
)

func TestPackageNotFoundErrorRendersWithoutCatalogs(t *testing.T) {
	err := &PackageNotFoundError{Name: "jq"}
	if got := err.Error(); got != "package jq not found" {
		t.Fatalf("want clean rendering with no catalog list, got %q", got)
	}
	err = &PackageNotFoundError{Name: "jq", Catalogs: []string{"a", "b"}}
	if got := err.Error(); !strings.Contains(got, "in catalog(s) a, b") {
		t.Fatalf("want catalog list preserved when present, got %q", got)
	}
}
