package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGendocsWritesPagesForEveryVisibleCommand(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cli")
	_, stderr, err := runNem(t, t.TempDir(), "gendocs", dir)
	if err != nil {
		t.Fatalf("gendocs: %v\n%s", err, stderr)
	}
	for _, want := range []string{
		"_index.md", "nem.md", "nem-use.md", "nem-catalog.md", "nem-catalog-fill.md",
		"nem-self-update.md", "nem-completion.md", "nem-completion-zsh.md",
	} {
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Errorf("%s: %v", want, err)
		}
	}
	for _, absent := range []string{"nem-gendocs.md", "nem-help.md", "nem-__complete.md"} {
		if _, err := os.Stat(filepath.Join(dir, absent)); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s should not exist (err=%v)", absent, err)
		}
	}
	rootPage, err := os.ReadFile(filepath.Join(dir, "nem.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rootPage), "| `-v, --version` |") {
		t.Errorf("nem.md does not list --version:\n%s", rootPage)
	}
	if !strings.Contains(stderr, "OK Wrote ") {
		t.Errorf("stderr = %q, want an OK line", stderr)
	}
}

func TestGendocsIsHidden(t *testing.T) {
	out, _, err := runNem(t, t.TempDir(), "help")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "gendocs") {
		t.Errorf("gendocs listed in --help:\n%s", out)
	}
}
