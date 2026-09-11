package spec

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

func InsertVersion(data []byte, e VersionEntry) ([]byte, error) {
	return InsertVersionAt(data, e, 0)
}

func InsertVersionAt(data []byte, e VersionEntry, pos int) ([]byte, error) {
	f, err := parser.ParseBytes(data, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	seq, err := locateVersions(f)
	if err != nil {
		return nil, err
	}
	if pos < 0 || pos > len(seq.Values) {
		return nil, fmt.Errorf("insert position %d out of range [0,%d]", pos, len(seq.Values))
	}

	snippet := "versions:\n" + renderVersionEntry(e)
	sf, err := parser.ParseBytes([]byte(snippet), 0)
	if err != nil {
		return nil, fmt.Errorf("render new entry: %w", err)
	}
	p, err := yaml.PathString("$.versions")
	if err != nil {
		return nil, fmt.Errorf("versions path: %w", err)
	}
	sNode, err := p.FilterFile(sf)
	if err != nil {
		return nil, fmt.Errorf("locate rendered entry: %w", err)
	}
	newSeq, ok := sNode.(*ast.SequenceNode)
	if !ok || len(newSeq.Values) != 1 {
		return nil, fmt.Errorf("render new entry: unexpected shape")
	}

	vals := make([]ast.Node, 0, len(seq.Values)+1)
	vals = append(vals, seq.Values[:pos]...)
	vals = append(vals, newSeq.Values[0])
	vals = append(vals, seq.Values[pos:]...)
	seq.Values = vals
	out := f.String()
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return []byte(out), nil
}

func ValidateEditable(data []byte) error {
	f, err := parser.ParseBytes(data, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse yaml: %w", err)
	}
	_, err = locateVersions(f)
	return err
}

func locateVersions(f *ast.File) (*ast.SequenceNode, error) {
	p, err := yaml.PathString("$.versions")
	if err != nil {
		return nil, fmt.Errorf("versions path: %w", err)
	}
	node, err := p.FilterFile(f)
	if err != nil {
		return nil, fmt.Errorf("locate versions: %w", err)
	}
	seq, ok := node.(*ast.SequenceNode)
	if !ok || seq.IsFlowStyle {
		return nil, fmt.Errorf("versions is not a block sequence")
	}
	return seq, nil
}

func renderVersionEntry(e VersionEntry) string {
	var b strings.Builder
	if len(e.Meta) == 0 && len(e.Sha256) == 0 && e.SourceSha256 == "" {
		fmt.Fprintf(&b, "  - %s\n", e.Version)
		return b.String()
	}
	fmt.Fprintf(&b, "  - version: %s\n", e.Version)
	if len(e.Meta) > 0 {
		pairs := make([]string, 0, len(e.Meta))
		for _, k := range slices.Sorted(maps.Keys(e.Meta)) {
			pairs = append(pairs, fmt.Sprintf("%s: %q", k, e.Meta[k]))
		}
		fmt.Fprintf(&b, "    meta: {%s}\n", strings.Join(pairs, ", "))
	}
	if e.SourceSha256 != "" {
		fmt.Fprintf(&b, "    sourceSha256: %q\n", e.SourceSha256)
	}
	if len(e.Sha256) > 0 {
		b.WriteString("    sha256:\n")
		for _, p := range SupportedPlatforms {
			if sum, ok := e.Sha256[p.String()]; ok {
				fmt.Fprintf(&b, "      %s: %q\n", p, sum)
			}
		}
	}
	return b.String()
}
