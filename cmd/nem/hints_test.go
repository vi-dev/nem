package main

import (
	"fmt"
	"testing"

	"github.com/vi-dev/nem/internal/fetch"
)

func TestHintForRelativeOCIRefWithoutArchiveStore(t *testing.T) {
	want := "Relative oci refs resolve inside the archive store of the catalog that serves the package; " +
		"add that catalog with `nem catalog add <name> <dir|ref>`"
	if got := hintFor(fmt.Errorf("acquire tool v1: %w", fetch.ErrNoArchiveStore)); got != want {
		t.Fatalf("hint = %q, want %q", got, want)
	}
}
