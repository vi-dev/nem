package mirror

import (
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

func TestNoUpstreamHTTPImports(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	forbidden := map[string]bool{
		"net/http":                            true,
		"github.com/vi-dev/nem/internal/netx": true,
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range file.Imports {
			spec := strings.Trim(imp.Path.Value, `"`)
			if forbidden[spec] {
				t.Fatalf("%s imports %s: mirror must reach only the two registries it is given", name, spec)
			}
		}
	}
}
