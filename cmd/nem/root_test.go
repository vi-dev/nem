package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/vi-dev/clix"
)

func TestRootHelpGroups(t *testing.T) {
	var buf bytes.Buffer
	if err := execNem(t, t.TempDir(), nil, &buf, new(bytes.Buffer), "--help"); err != nil {
		t.Fatalf("--help: %v", err)
	}
	out := buf.String()
	titles := []string{"Environment:", "Discovery:", "Catalogs:", "Shell integration:"}
	last := -1
	for _, title := range titles {
		i := strings.Index(out, title)
		if i < 0 {
			t.Fatalf("help output missing group %q:\n%s", title, out)
		}
		if i < last {
			t.Fatalf("group %q listed out of order:\n%s", title, out)
		}
		last = i
	}
}

func TestEveryRootCommandGrouped(t *testing.T) {
	root := newRoot()
	for _, c := range root.Commands() {
		if c.Hidden || c.Name() == "version" {
			continue
		}
		if c.GroupID == "" {
			t.Errorf("command %q has no group", c.Name())
		}
	}
}

func TestUnknownCommandIsUsageError(t *testing.T) {
	var buf bytes.Buffer
	if err := execNem(t, t.TempDir(), nil, &buf, &buf, "definitely-not-a-command"); err == nil {
		t.Fatal("want error for unknown command")
	}
	if ranHook {
		t.Fatal("hook ran for unknown command; usage errors must exit 2")
	}
}

func TestRootHookWarnsOnMalformedConfigAndKeepsRunning(t *testing.T) {
	nemHomeDir := t.TempDir()
	if err := os.MkdirAll(nemHomeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nemHomeDir, "config.yaml"), []byte("hosts: [this is not: valid yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, err := runNem(t, nemHomeDir, "version")
	if err != nil {
		t.Fatalf("version must still succeed on a malformed config: %v\nstderr: %s", err, errb)
	}
	if !strings.Contains(errb, "host settings") {
		t.Fatalf("stderr = %q, want a host settings warning", errb)
	}
}

func TestNarrationEnvInvalidValueExitsTwo(t *testing.T) {
	t.Setenv("NEM_NARRATION", "bogus")
	t.Setenv("NEM_HOME", t.TempDir())
	var out, errb bytes.Buffer
	root := newRoot()
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs([]string{"env", "--color", "never"})
	code, err := clix.Execute(context.Background(), root, nemOptions())
	if err == nil {
		t.Fatal("want error for an invalid NEM_NARRATION value")
	}
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	want := `Error: invalid NEM_NARRATION value "bogus" (want auto, plain, or json)`
	if !strings.Contains(errb.String(), want) {
		t.Fatalf("stderr = %q, want it to contain %q", errb.String(), want)
	}
}

func TestNarrationFlagAppearsInHelp(t *testing.T) {
	var out bytes.Buffer
	if err := execNem(t, t.TempDir(), nil, &out, new(bytes.Buffer), "--help"); err != nil {
		t.Fatalf("--help: %v", err)
	}
	if !strings.Contains(out.String(), "--narration") {
		t.Fatalf("help output missing --narration:\n%s", out.String())
	}
}

func settledGoroutines() int {
	n := runtime.NumGoroutine()
	stable := 0
	deadline := time.Now().Add(2 * time.Second)
	for stable < 5 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		m := runtime.NumGoroutine()
		if m == n {
			stable++
		} else {
			n, stable = m, 0
		}
	}
	return n
}

func TestEnvCommandNarratesNothing(t *testing.T) {
	nemHomeDir := t.TempDir()
	projDir := t.TempDir()
	chdir(t, projDir)
	if err := os.WriteFile(filepath.Join(projDir, "nem.toml"), []byte("[env]\nFOO = \"bar\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	before := settledGoroutines()
	_, errb, err := runNem(t, nemHomeDir, "env")
	if err != nil {
		t.Fatalf("env: %v\n%s", err, errb)
	}
	if errb != "" {
		t.Fatalf("stderr = %q, want it empty", errb)
	}
	if after := settledGoroutines(); after != before {
		t.Fatalf("goroutines = %d after env, want %d", after, before)
	}
}
