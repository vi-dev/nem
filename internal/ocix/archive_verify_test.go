package ocix

import (
	"context"
	"testing"

	"oras.land/oras-go/v2/content/memory"

	"github.com/vi-dev/nem/internal/spec"
)

func TestVerifyArchiveCommitted(t *testing.T) {
	plat := spec.Platform{OS: "linux", Arch: "amd64"}
	other := spec.Platform{OS: "darwin", Arch: "arm64"}
	target := memory.New()
	entry, pushed, err := PushArchive(context.Background(), target, "v1", plat, []byte("x"), false)
	if err != nil || !pushed {
		t.Fatalf("PushArchive: %v", err)
	}
	if err := VerifyArchiveCommitted(context.Background(), target, "v1", plat, entry); err != nil {
		t.Fatalf("verify after commit: %v", err)
	}
	if err := VerifyArchiveCommitted(context.Background(), target, "v1", other, entry); err == nil {
		t.Fatal("verify must fail for a platform the index lacks")
	}
	if err := VerifyArchiveCommitted(context.Background(), target, "v2", plat, entry); err == nil {
		t.Fatal("verify must fail for a missing tag")
	}
}
