package spec

import (
	"strings"
	"testing"
)

const filesPkgYAML = `schema: 3
name: gpgme
versions:
  - 2.2.0
  - 2.1.2
  - 1.24.3
artifact: {oci: ":{{.Version}}"}
install: [{extract: {}}]
bins: [bin]
`

const filesRecipe1YAML = `artifact: {oci: ":{{.Version}}"}
install: [{extract: {strip: 1}}]
platforms: [linux/amd64]
`

func TestParseFilesAssemblesFrozenRecipes(t *testing.T) {
	p, err := ParseFiles(Files{Pkg: []byte(filesPkgYAML), Recipes: map[string][]byte{"recipe@1.yaml": []byte(filesRecipe1YAML)}})
	if err != nil {
		t.Fatalf("ParseFiles: %v", err)
	}
	if len(p.Frozen) != 1 || p.Frozen[0].Prefix != "1" || p.Frozen[0].Install[0].Extract.Strip != 1 {
		t.Fatalf("frozen: %+v", p.Frozen)
	}
	if p.Frozen[0].Bins[0] != "bin" {
		t.Fatalf("bins default must apply per recipe, got %v", p.Frozen[0].Bins)
	}
	r, err := p.Recipe("1.24.3")
	if err != nil || r.Prefix != "1" || len(r.Platforms) != 1 {
		t.Fatalf("Recipe(1.24.3) = %+v, %v", r, err)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestParseFilesSortsFrozenByPrefix(t *testing.T) {
	p, err := ParseFiles(Files{Pkg: []byte(filesPkgYAML), Recipes: map[string][]byte{
		"recipe@10.yaml": []byte(filesRecipe1YAML),
		"recipe@2.yaml":  []byte(filesRecipe1YAML),
		"recipe@1.yaml":  []byte(filesRecipe1YAML),
	}})
	if err != nil {
		t.Fatalf("ParseFiles: %v", err)
	}
	got := []string{p.Frozen[0].Prefix, p.Frozen[1].Prefix, p.Frozen[2].Prefix}
	if strings.Join(got, ",") != "1,2,10" {
		t.Fatalf("order = %v", got)
	}
}

func TestParseFilesRejectsBadNamesAndIdentityInRecipe(t *testing.T) {
	_, err := ParseFiles(Files{Pkg: []byte(filesPkgYAML), Recipes: map[string][]byte{"recipe@v1.yaml": []byte(filesRecipe1YAML)}})
	if err == nil || !strings.Contains(err.Error(), "recipe@<prefix>.yaml") {
		t.Fatalf("bad filename: %v", err)
	}
	_, err = ParseFiles(Files{Pkg: []byte(filesPkgYAML), Recipes: map[string][]byte{"recipe@1.yaml": []byte("name: gpgme\n" + filesRecipe1YAML)}})
	if err == nil || !strings.Contains(err.Error(), "recipe@1.yaml") {
		t.Fatalf("identity field in recipe must name the file: %v", err)
	}
	_, err = Parse([]byte(filesPkgYAML + "recipes: []\n"))
	if err == nil {
		t.Fatal("inline recipes: must be rejected")
	}
}

func TestParseFilesEmptyRecipeNamesTheFile(t *testing.T) {
	p, err := ParseFiles(Files{Pkg: []byte(filesPkgYAML), Recipes: map[string][]byte{"recipe@1.yaml": []byte("")}})
	if err == nil {
		err = p.Validate()
	}
	if err == nil || !strings.Contains(err.Error(), "recipe@1.yaml") {
		t.Fatalf("empty recipe must fail naming the file, got %v", err)
	}
}

func TestValidateChecksumsPerRecipePlatforms(t *testing.T) {
	pkg := `schema: 3
name: t
artifact: {url: "https://example.com/{{.Version}}"}
install: [{extract: {}}]
versions:
  - version: 2.0.0
    sha256: {darwin/arm64: a, darwin/amd64: a, linux/arm64: a, linux/amd64: a}
  - version: 1.0.0
    sha256: {linux/amd64: a}
`
	p, err := ParseFiles(Files{Pkg: []byte(pkg), Recipes: map[string][]byte{
		"recipe@1.yaml": []byte("artifact: {url: \"https://example.com/{{.Version}}\"}\ninstall: [{extract: {}}]\nplatforms: [linux/amd64]\n"),
	}})
	if err != nil {
		t.Fatalf("ParseFiles: %v", err)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("1.0.0 only needs the frozen recipe's platforms: %v", err)
	}
	p.Frozen = nil
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "1.0.0") {
		t.Fatalf("without the frozen recipe 1.0.0 must miss checksums: %v", err)
	}
}

func TestValidateAcceptsSchema2And3(t *testing.T) {
	for _, s := range []string{"2", "3"} {
		p, err := Parse([]byte(strings.Replace(filesPkgYAML, "schema: 3", "schema: "+s, 1)))
		if err != nil {
			t.Fatalf("Parse schema %s: %v", s, err)
		}
		if err := p.Validate(); err != nil {
			t.Fatalf("schema %s: %v", s, err)
		}
	}
	p, _ := Parse([]byte(strings.Replace(filesPkgYAML, "schema: 3", "schema: 4", 1)))
	if err := p.Validate(); err == nil {
		t.Fatal("schema 4 must be rejected")
	}
}

func TestFormatPackageSetsSchema3(t *testing.T) {
	out, err := FormatPackage([]byte("schema: 2\nname: t\n"))
	if err != nil {
		t.Fatalf("FormatPackage: %v", err)
	}
	if !strings.HasPrefix(string(out), "schema: 3\n") {
		t.Fatalf("got %q", out)
	}
	again, _ := FormatPackage(out)
	if string(again) != string(out) {
		t.Fatalf("not idempotent: %q vs %q", again, out)
	}
}
