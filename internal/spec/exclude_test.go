package spec

import (
	"strings"
	"testing"
)

const excludePkgYAML = `schema: 4
name: tool
versions:
  - version: 3.2.1
    sha256: {linux/amd64: "aa"}
  - version: 3.2.0
    excluded:
      reason: "CVE-2026-1234: world-readable files"
      use: 3.2.1
  - version: 3.1.0
    excluded: "Broken build"
  - version: 3.0.0
    sha256: {linux/amd64: "bb"}
artifact: {url: "https://example.com/{{.Version}}"}
install: [{copy: {src: "{{.Artifact}}", dst: bin/tool}}]
platforms: [linux/amd64]
`

func TestParseExclusionForms(t *testing.T) {
	p, err := Parse([]byte(excludePkgYAML))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if ex := p.Exclusion("3.2.0"); ex == nil || ex.Reason != "CVE-2026-1234: world-readable files" || ex.Use != "3.2.1" {
		t.Fatalf("mapping form: %+v", ex)
	}
	if ex := p.Exclusion("3.1.0"); ex == nil || ex.Reason != "Broken build" || ex.Use != "" {
		t.Fatalf("string form: %+v", ex)
	}
	if p.Exclusion("3.2.1") != nil {
		t.Fatal("3.2.1 is not excluded")
	}
	if got := len(p.Offered()); got != 2 {
		t.Fatalf("Offered: %d entries, want 2", got)
	}
}

func TestLatestSkipsExcludedHead(t *testing.T) {
	p, err := Parse([]byte(strings.Replace(excludePkgYAML, "  - version: 3.2.1\n    sha256: {linux/amd64: \"aa\"}\n", "", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Latest(); got != "3.0.0" {
		t.Fatalf("Latest = %q, want 3.0.0", got)
	}
}

func TestValidateExclusion(t *testing.T) {
	for _, tc := range []struct {
		name, old, new, want string
	}{
		{"needs schema 4", "schema: 4", "schema: 3", "excluded requires schema 4"},
		{"empty reason", `excluded: "Broken build"`, `excluded: "  "`, "excluded requires a reason"},
		{"multi-line reason", `excluded: "Broken build"`, `excluded: "Broken\nbuild"`, "single line"},
		{"long reason", `excluded: "Broken build"`, `excluded: "` + strings.Repeat("x", 301) + `"`, "the limit is 300"},
		{"use unknown", "use: 3.2.1", "use: 9.9.9", "excluded use 9.9.9 is not a listed version"},
		{"use excluded", "use: 3.2.1", "use: 3.1.0", "excluded use 3.1.0 is itself excluded"},
		{"use self", "use: 3.2.1", "use: 3.2.0", "must name another version"},
		{"unknown key", "use: 3.2.1", "use: 3.2.1\n      until: x", "until"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := Parse([]byte(strings.Replace(excludePkgYAML, tc.old, tc.new, 1)))
			if err == nil {
				err = p.Validate()
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestValidateRefusesEveryVersionExcluded(t *testing.T) {
	data := `schema: 4
name: tool
versions:
  - version: 1.0.0
    excluded: "gone"
artifact: {oci: ":{{.Version}}"}
install: [{extract: {}}]
`
	p, err := Parse([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "every version is excluded") {
		t.Fatalf("err = %v", err)
	}
}

func TestExcludedEntryNeedsNoChecksum(t *testing.T) {
	p, _ := Parse([]byte(excludePkgYAML))
	if err := p.Validate(); err != nil {
		t.Fatalf("excluded entries without sha256 must validate: %v", err)
	}
	p, _ = Parse([]byte(strings.Replace(excludePkgYAML, "    sha256: {linux/amd64: \"bb\"}\n", "", 1)))
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "3.0.0") {
		t.Fatalf("an offered entry still needs sha256: %v", err)
	}
}

const editPkgYAML = `schema: 4
name: tool # the tool
versions:
  - version: 3.2.1
    sha256:
      linux/amd64: "aa"
  - version: 3.2.0
    sha256:
      linux/amd64: "bb"
  - 3.1.0

artifact: {url: "https://example.com/{{.Version}}"}
`

func TestSetExclusionRoundTrip(t *testing.T) {
	ex := &Exclusion{Reason: "CVE-2026-1234", Use: "3.2.1"}
	out, err := SetExclusion([]byte(editPkgYAML), "3.2.0", ex)
	if err != nil {
		t.Fatalf("SetExclusion: %v", err)
	}
	want := strings.Replace(editPkgYAML, "  - version: 3.2.0\n", "  - version: 3.2.0\n    excluded:\n      reason: \"CVE-2026-1234\"\n      use: 3.2.1\n", 1)
	if string(out) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
	back, err := SetExclusion(out, "3.2.0", nil)
	if err != nil || string(back) != editPkgYAML {
		t.Fatalf("removing the exclusion must restore the file: %v\n%s", err, back)
	}
}

func TestSetExclusionOnBareEntry(t *testing.T) {
	out, err := SetExclusion([]byte(editPkgYAML), "3.1.0", &Exclusion{Reason: "Broken build"})
	if err != nil {
		t.Fatalf("SetExclusion: %v", err)
	}
	if !strings.Contains(string(out), "  - version: 3.1.0\n    excluded: \"Broken build\"\n") {
		t.Fatalf("got:\n%s", out)
	}
	p, err := Parse(out)
	if err != nil || p.Exclusion("3.1.0") == nil {
		t.Fatalf("reparse: %v", err)
	}
	if _, err := SetExclusion([]byte(editPkgYAML), "9.9.9", &Exclusion{Reason: "x"}); err == nil {
		t.Fatal("an unlisted version must be refused")
	}
}

func TestSetSchema(t *testing.T) {
	out, err := SetSchema([]byte(editPkgYAML), 3)
	if err != nil || string(out) != strings.Replace(editPkgYAML, "schema: 4", "schema: 3", 1) {
		t.Fatalf("SetSchema: %v\n%s", err, out)
	}
}

func TestSetExclusionKeepsFloatLikeVersionsStrings(t *testing.T) {
	data := "schema: 4\nname: tool\nversions:\n  - \"1.10\"\n  - \"1.9\"\n"
	out, err := SetExclusion([]byte(data), "1.9", &Exclusion{Reason: "bad", Use: "1.10"})
	if err != nil {
		t.Fatalf("SetExclusion: %v", err)
	}
	p, err := Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if p.Versions[1].Version != "1.9" || p.Exclusion("1.9") == nil || p.Exclusion("1.9").Use != "1.10" {
		t.Fatalf("entry rewritten wrongly:\n%s", out)
	}
	if out, _ = SetExclusion([]byte(data), "1.10", &Exclusion{Reason: "bad"}); !strings.Contains(string(out), "version: \"1.10\"") {
		t.Fatalf("1.10 must stay quoted:\n%s", out)
	}
}

func TestExclusionMatchesEqualSpellings(t *testing.T) {
	p, _ := Parse([]byte(excludePkgYAML))
	if p.Exclusion("v3.2.0") == nil {
		t.Fatal("v3.2.0 compares equal to the excluded 3.2.0")
	}
	if p.HasVersion("v3.2.0") || !p.HasVersion("3.2.0") {
		t.Fatal("HasVersion matches the exact spelling only")
	}
}
