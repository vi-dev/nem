package spec

import (
	"fmt"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/parser"
)

func Format(data []byte) ([]byte, error) {
	f, err := parser.ParseBytes(data, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	out := f.String()
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return []byte(out), nil
}

func FormatPackage(data []byte) ([]byte, error) {
	f, err := parser.ParseBytes(data, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	path, err := yaml.PathString("$.schema")
	if err != nil {
		return nil, err
	}
	if node, err := path.FilterFile(f); err == nil && strings.TrimSpace(node.String()) == "2" {
		if err := path.ReplaceWithReader(f, strings.NewReader("3")); err != nil {
			return nil, fmt.Errorf("set schema: %w", err)
		}
	}
	out := f.String()
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return []byte(out), nil
}
