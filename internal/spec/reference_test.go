package spec

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

var actionKeys = []string{"extract", "copy", "move", "mkdir"}

func schemaKeys() []string {
	seen := map[string]bool{}
	var walk func(t reflect.Type)
	walk = func(t reflect.Type) {
		for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Map {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct {
			return
		}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			key := strings.Split(f.Tag.Get("yaml"), ",")[0]
			if key == "" {
				key = strings.ToLower(f.Name)
			}
			if key == "-" {
				continue
			}
			seen[key] = true
			walk(f.Type)
		}
	}
	for _, root := range []any{rawPackage{}, rawVersionEntry{}, ExtractAction{}, CopyAction{}, MoveAction{}} {
		walk(reflect.TypeOf(root))
	}
	for _, k := range actionKeys {
		seen[k] = true
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestReferenceDocumentsEverySchemaKey(t *testing.T) {
	t.Parallel()
	page, err := os.ReadFile(filepath.Join("..", "..", "website", "content", "docs", "reference", "pkg-yaml.md"))
	if err != nil {
		t.Fatalf("read pkg-yaml.md: %v", err)
	}
	for _, key := range schemaKeys() {
		if !strings.Contains(string(page), "`"+key+"`") {
			t.Errorf("pkg-yaml.md does not document `%s`", key)
		}
	}
}
