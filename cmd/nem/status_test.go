package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vi-dev/nem/internal/project"
	"github.com/vi-dev/nem/internal/spec"
)

func TestStatusListsToolsAndEnv(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "nem.toml"), []byte(
		"[tools]\ngo = \"v1.26.5\"\n\"dev:kubectl\" = \"v1.34.1\"\n\n[env]\nCGO_ENABLED = \"0\"\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "nem.lock"), []byte(
		"# machine-written by nem — do not edit\nversion = 1\n\n[[package]]\nname = \"go\"\nversion = \"v1.26.5\"\ncatalog = \"official\"\ndirect = true\nplatforms = [\"linux/amd64\"]\n"), 0o644)

	chdir(t, dir)
	text, errb, err := runNem(t, filepath.Join(dir, "nemhome"), "status")
	if err != nil {
		t.Fatalf("status: %v\nstderr: %s", err, errb)
	}
	for _, want := range []string{
		"PACKAGE", "VERSION", "CATALOG", "LOCKED",
		"go", "v1.26.5", "yes",
		"kubectl", "dev", "no",
		"CGO_ENABLED", "0",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("stdout missing %q:\n%s", want, text)
		}
	}
}

func TestStatusInstalledColumn(t *testing.T) {
	nemHomeDir := t.TempDir()
	catalogRoot := downloadableDirCatalog(t, "")
	projDir := t.TempDir()
	chdir(t, projDir)

	if _, errb, err := runNem(t, nemHomeDir, "catalog", "add", "demo", catalogRoot); err != nil {
		t.Fatalf("catalog add: %v\n%s", err, errb)
	}
	if _, errb, err := runNem(t, nemHomeDir, "use", "demo:tool"); err != nil {
		t.Fatalf("use: %v\n%s", err, errb)
	}

	out, errb, err := runNem(t, nemHomeDir, "status")
	if err != nil {
		t.Fatalf("status: %v\nstderr: %s", err, errb)
	}
	if !strings.Contains(out, "INSTALLED") {
		t.Fatalf("stdout missing INSTALLED header:\n%s", out)
	}
	var toolLine string
	for l := range strings.SplitSeq(strings.TrimRight(out, "\n"), "\n") {
		if strings.HasPrefix(l, "tool ") {
			toolLine = l
		}
	}
	if toolLine == "" {
		t.Fatalf("no row for tool in status output:\n%s", out)
	}
	fields := strings.Fields(toolLine)
	if got := fields[len(fields)-1]; got != "yes" {
		t.Fatalf("tool row INSTALLED cell = %q, want yes: %q", got, toolLine)
	}
}

func TestStatusSummarizesUninstalledPackagesPerScope(t *testing.T) {
	nemHomeDir := t.TempDir()
	projDir := t.TempDir()
	chdir(t, projDir)

	platforms := []string{spec.Current().String()}
	writeFile(t, filepath.Join(projDir, "nem.toml"), "[tools]\n\"test:ghost\" = \"v1.0.0\"\n")
	lf := &project.Lockfile{Path: filepath.Join(projDir, "nem.lock"), Packages: []project.LockEntry{
		{Name: "ghost", Version: "v1.0.0", Catalog: "test", Direct: true, OnPath: true, Platforms: platforms},
	}}
	if err := project.WriteLock(lf); err != nil {
		t.Fatalf("WriteLock: %v", err)
	}
	globalLock := &project.Lockfile{Path: testNemHome(nemHomeDir).GlobalLock(), Packages: []project.LockEntry{
		{Name: "gtool", Version: "v2.0.0", Catalog: "test", Direct: true, Platforms: platforms},
		{Name: "htool", Version: "v3.0.0", Catalog: "test", Direct: true, Platforms: platforms},
	}}
	if err := project.WriteLock(globalLock); err != nil {
		t.Fatalf("WriteLock: %v", err)
	}

	_, errb, err := runNem(t, nemHomeDir, "status")
	if err != nil {
		t.Fatalf("status: %v\nstderr: %s", err, errb)
	}
	if !strings.HasPrefix(errb, "\n") || strings.HasPrefix(errb, "\n\n") {
		t.Errorf("want exactly one blank line separating the table from the summary, stderr: %q", errb)
	}
	if !strings.Contains(errb, "1 project package not installed — run `nem sync`") {
		t.Errorf("stderr missing project-scope summary: %q", errb)
	}
	if !strings.Contains(errb, "2 global packages not installed — run `nem sync -g`") {
		t.Errorf("stderr missing global-scope summary: %q", errb)
	}
	if strings.Contains(errb, "no install metadata") {
		t.Errorf("uninstalled packages must be summarized, not warned per package: %q", errb)
	}
}

func TestStatusUnlockedToolShowsInstalledDash(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "nem.toml"), []byte("[tools]\ngo = \"v1.26.5\"\n"), 0o644)

	chdir(t, dir)
	out, errb, err := runNem(t, filepath.Join(dir, "nemhome"), "status")
	if err != nil {
		t.Fatalf("status: %v\nstderr: %s", err, errb)
	}
	var toolLine string
	for l := range strings.SplitSeq(strings.TrimRight(out, "\n"), "\n") {
		if strings.HasPrefix(l, "go ") {
			toolLine = l
		}
	}
	if toolLine == "" {
		t.Fatalf("no row for go in status output:\n%s", out)
	}
	fields := strings.Fields(toolLine)
	if got := fields[len(fields)-1]; got != "-" {
		t.Fatalf("unlocked tool row INSTALLED cell = %q, want -: %q", got, toolLine)
	}
}

func TestStatusShowsComposedEnvWithSource(t *testing.T) {
	nemHomeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(nemHomeDir, "nem.toml"),
		[]byte("[tools]\ntool = \"1.0.0\"\n\n[env]\nFROM_MANIFEST = \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nemHomeDir, "nem.lock"), []byte(
		"# machine-written by nem — do not edit\nversion = 1\n\n"+
			"[[package]]\nname = \"tool\"\nversion = \"1.0.0\"\ncatalog = \"c\"\ndirect = true\non_path = true\nplatforms = [\""+platformString()+"\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	toolDir := filepath.Join(nemHomeDir, "packages", "tool", "1.0.0")
	writeMetaFile(t, toolDir, "package: tool\nversion: 1.0.0\ncatalog: c\nbins: [bin]\n"+
		"env:\n  - name: TOOL_HOME\n    value: \"{{ .InstallDir }}\"\n")
	chdir(t, t.TempDir())

	out, errb, err := runNem(t, nemHomeDir, "status", "-g")
	if err != nil {
		t.Fatalf("status -g: %v\nstderr: %s", err, errb)
	}
	rows := map[string]string{}
	for l := range strings.SplitSeq(strings.TrimRight(out, "\n"), "\n") {
		if f := strings.Fields(l); len(f) > 1 {
			rows[f[0]] = strings.Join(f[1:], " ")
		}
	}
	if !strings.Contains(out, "SOURCE") {
		t.Errorf("stdout missing SOURCE header:\n%s", out)
	}
	if want := toolDir + " tool"; rows["TOOL_HOME"] != want {
		t.Errorf("TOOL_HOME row = %q, want %q", rows["TOOL_HOME"], want)
	}
	if want := "x nem.toml"; rows["FROM_MANIFEST"] != want {
		t.Errorf("FROM_MANIFEST row = %q, want %q", rows["FROM_MANIFEST"], want)
	}
	if !strings.Contains(out, "\n\nVARIABLE") {
		t.Errorf("no blank line between packages and env tables:\n%s", out)
	}
}

func TestStatusWarnsOnReservedEnvVar(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "nem.toml"),
		[]byte("[env]\nPATH = \"/custom\"\nOK_VAR = \"1\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)

	out, errb, err := runNem(t, filepath.Join(dir, "nemhome"), "status")
	if err != nil {
		t.Fatalf("status: %v\nstderr: %s", err, errb)
	}
	if !strings.Contains(errb, `reserved env var "PATH" skipped`) {
		t.Errorf("stderr missing reserved-name warning:\n%s", errb)
	}
	if strings.Contains(out, "/custom") {
		t.Errorf("reserved PATH entry rendered in table:\n%s", out)
	}
}

func TestStatusScopesDoNotMerge(t *testing.T) {
	nemHomeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(nemHomeDir, "nem.toml"),
		[]byte("[tools]\nhelm = \"v4.2.3\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	projDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projDir, "nem.toml"),
		[]byte("[tools]\ngo = \"v1.26.5\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, projDir)

	hasRow := func(out, name string) bool {
		for l := range strings.SplitSeq(strings.TrimRight(out, "\n"), "\n") {
			if strings.HasPrefix(l, name+" ") {
				return true
			}
		}
		return false
	}

	outP, _, err := runNem(t, nemHomeDir, "status")
	if err != nil {
		t.Fatal(err)
	}
	if !hasRow(outP, "go") || hasRow(outP, "helm") {
		t.Fatalf("project status should show go, not helm:\n%s", outP)
	}

	outG, _, err := runNem(t, nemHomeDir, "status", "-g")
	if err != nil {
		t.Fatal(err)
	}
	if !hasRow(outG, "helm") || hasRow(outG, "go") {
		t.Fatalf("global status should show helm, not go:\n%s", outG)
	}
}

func TestStatusDoesNotStampUsage(t *testing.T) {
	nemHomeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(nemHomeDir, "nem.toml"),
		[]byte("[tools]\ntool = \"1.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nemHomeDir, "nem.lock"), []byte(
		"# machine-written by nem — do not edit\nversion = 1\n\n"+
			"[[package]]\nname = \"tool\"\nversion = \"1.0.0\"\ncatalog = \"c\"\ndirect = true\non_path = true\nplatforms = [\""+platformString()+"\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	toolDir := filepath.Join(nemHomeDir, "packages", "tool", "1.0.0")
	writeMetaFile(t, toolDir, "package: tool\nversion: 1.0.0\ncatalog: c\nbins: [bin]\n")
	chdir(t, t.TempDir())

	usagePath := filepath.Join(nemHomeDir, "usage.json")
	seeded := []byte(`{"other@9.9.9":"2026-08-20T12:00:00Z"}`)
	if err := os.WriteFile(usagePath, seeded, 0o644); err != nil {
		t.Fatal(err)
	}

	out, errb, err := runNem(t, nemHomeDir, "status", "-g")
	if err != nil {
		t.Fatalf("status -g: %v\nstderr: %s", err, errb)
	}
	if !strings.Contains(out, "tool") {
		t.Fatalf("status -g did not compose the tool row:\n%s", out)
	}

	got, err := os.ReadFile(usagePath)
	if err != nil {
		t.Fatalf("read usage.json: %v", err)
	}
	if string(got) != string(seeded) {
		t.Fatalf("nem status modified usage.json: got %s, want %s", got, seeded)
	}
}

func TestStatusNoManifestFails(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if _, _, err := runNem(t, filepath.Join(dir, "nemhome"), "status"); err == nil {
		t.Fatal("want error when no nem.toml anywhere")
	}
}
