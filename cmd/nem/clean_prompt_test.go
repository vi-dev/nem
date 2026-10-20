package main

import (
	"strings"
	"testing"
	"time"

	"github.com/vi-dev/nem/internal/usage"
)

func TestCleanPromptReadsAnswerFromStdin(t *testing.T) {
	root := t.TempDir()
	seedPackageVersion(t, root, "go", "1.26.5")

	orig := usageIndex
	t.Cleanup(func() { usageIndex = orig })
	old := time.Now().Add(-90 * 24 * time.Hour)
	usageIndex = func() usage.Index {
		return usage.Index{usage.Key("go", "1.26.5"): old}
	}

	out, err := execNemMerged(t, root, "n\n", "clean", "--unused", "30d", "--color", "never")
	if err != nil {
		t.Fatalf("clean: %v\n%s", err, out)
	}
	want := "? Remove these package versions? [y/N]: n\n"
	if !strings.Contains(out, want) {
		t.Fatalf("output = %q, want it to contain %q", out, want)
	}
}
