package report

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestFromContextReturnsAttachedReporter(t *testing.T) {
	c := Discard()
	ctx := NewContext(context.Background(), c)
	if got := FromContext(ctx); got != Reporter(c) {
		t.Fatalf("FromContext returned %v, want the attached reporter", got)
	}
}

func TestFromContextWithoutAttachmentIsSilentNotNil(t *testing.T) {
	r := FromContext(context.Background())
	if r == nil {
		t.Fatal("FromContext returned nil for a bare context")
	}
	r.Info("must not panic")
	r.Task("must not panic").Done("ok")
}

func TestIsCancellation(t *testing.T) {
	if !IsCancellation(context.Canceled) {
		t.Error("context.Canceled must be classified as cancellation")
	}
	if !IsCancellation(fmt.Errorf("wrap: %w", context.DeadlineExceeded)) {
		t.Error("wrapped context.DeadlineExceeded must be classified as cancellation")
	}
	if IsCancellation(errors.New("real failure")) {
		t.Error("an unrelated error must not be classified as cancellation")
	}
	if IsCancellation(nil) {
		t.Error("a nil error must not be classified as cancellation")
	}
}
