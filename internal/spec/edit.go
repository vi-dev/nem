package spec

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
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
	seq, shape, err := versionsNode(f)
	if err != nil {
		return nil, err
	}
	if shape != versionsBlock {
		if pos != 0 {
			return nil, fmt.Errorf("insert position %d out of range [0,0]", pos)
		}
		if shape == versionsEmptyFlow {
			return replaceEmptyFlowVersions(data, seq, e)
		}
		return appendVersionsBlock(data, e), nil
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
	_, _, err = versionsNode(f)
	return err
}

type versionsShape int

const (
	versionsBlock versionsShape = iota
	versionsMissing
	versionsEmptyFlow
)

func versionsNode(f *ast.File) (*ast.SequenceNode, versionsShape, error) {
	p, err := yaml.PathString("$.versions")
	if err != nil {
		return nil, versionsBlock, fmt.Errorf("versions path: %w", err)
	}
	node, err := p.FilterFile(f)
	if err != nil {
		return nil, versionsMissing, nil
	}
	seq, ok := node.(*ast.SequenceNode)
	if !ok {
		return nil, versionsBlock, fmt.Errorf("versions is not a block sequence")
	}
	if seq.IsFlowStyle {
		if len(seq.Values) == 0 {
			return seq, versionsEmptyFlow, nil
		}
		return nil, versionsBlock, fmt.Errorf("versions is not a block sequence")
	}
	return seq, versionsBlock, nil
}

func appendVersionsBlock(data []byte, e VersionEntry) []byte {
	body := strings.TrimRight(string(data), "\n")
	block := "versions:\n" + renderVersionEntry(e)
	if body == "" {
		return []byte(block)
	}
	return []byte(body + "\n\n" + block)
}

func replaceEmptyFlowVersions(data []byte, seq *ast.SequenceNode, e VersionEntry) ([]byte, error) {
	lines := strings.SplitAfter(string(data), "\n")
	idx := seq.GetToken().Position.Line - 1
	if idx < 0 || idx >= len(lines) || !emptyFlowVersionsRE.MatchString(lines[idx]) {
		return nil, fmt.Errorf("versions is not a block sequence")
	}
	lines[idx] = "versions:\n" + renderVersionEntry(e)
	return []byte(strings.Join(lines, "")), nil
}

func renderVersionEntry(e VersionEntry) string {
	var b strings.Builder
	if len(e.Meta) == 0 && len(e.Sha256) == 0 && e.SourceSha256 == "" && e.Excluded == nil {
		fmt.Fprintf(&b, "  - %s\n", yamlString(e.Version))
		return b.String()
	}
	fmt.Fprintf(&b, "  - version: %s\n", yamlString(e.Version))
	if len(e.Meta) > 0 {
		pairs := make([]string, 0, len(e.Meta))
		for _, k := range slices.Sorted(maps.Keys(e.Meta)) {
			pairs = append(pairs, fmt.Sprintf("%s: %q", k, e.Meta[k]))
		}
		fmt.Fprintf(&b, "    meta: {%s}\n", strings.Join(pairs, ", "))
	}
	if ex := e.Excluded; ex != nil {
		if ex.Use == "" {
			fmt.Fprintf(&b, "    excluded: %q\n", ex.Reason)
		} else {
			fmt.Fprintf(&b, "    excluded:\n      reason: %q\n      use: %s\n", ex.Reason, yamlString(ex.Use))
		}
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

var emptyFlowVersionsRE = regexp.MustCompile(`^versions:\s*\[\s*\]\s*$`)

// SetExclusion rewrites the entry for version with ex as its exclusion, or
// without one when ex is nil. Only that entry is re-rendered.
func SetExclusion(data []byte, version string, ex *Exclusion) ([]byte, error) {
	f, err := parser.ParseBytes(data, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	seq, shape, err := versionsNode(f)
	if err != nil {
		return nil, err
	}
	if shape != versionsBlock {
		return nil, fmt.Errorf("version %s is not listed", version)
	}
	for i, node := range seq.Values {
		e, err := shapeVersion(yaml.RawMessage(node.String()))
		if err != nil {
			return nil, fmt.Errorf("versions[%d]: %w", i, err)
		}
		if e.Version != version {
			continue
		}
		e.Excluded = ex
		replacement, err := renderedEntryNode(e)
		if err != nil {
			return nil, err
		}
		seq.Values[i] = replacement
		out := f.String()
		if !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		return []byte(out), nil
	}
	return nil, fmt.Errorf("version %s is not listed", version)
}

func renderedEntryNode(e VersionEntry) (ast.Node, error) {
	sf, err := parser.ParseBytes([]byte("versions:\n"+renderVersionEntry(e)), 0)
	if err != nil {
		return nil, fmt.Errorf("render entry: %w", err)
	}
	p, err := yaml.PathString("$.versions")
	if err != nil {
		return nil, fmt.Errorf("versions path: %w", err)
	}
	node, err := p.FilterFile(sf)
	if err != nil {
		return nil, fmt.Errorf("locate rendered entry: %w", err)
	}
	seq, ok := node.(*ast.SequenceNode)
	if !ok || len(seq.Values) != 1 {
		return nil, fmt.Errorf("render entry: unexpected shape")
	}
	return seq.Values[0], nil
}

func SetSchema(data []byte, schema int) ([]byte, error) {
	f, err := parser.ParseBytes(data, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	path, err := yaml.PathString("$.schema")
	if err != nil {
		return nil, err
	}
	if err := path.ReplaceWithReader(f, strings.NewReader(strconv.Itoa(schema))); err != nil {
		return nil, fmt.Errorf("set schema: %w", err)
	}
	out := f.String()
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return []byte(out), nil
}

// yamlString writes s plain when YAML reads it back as the same string, and
// quoted otherwise: an unquoted 1.10 would read back as the float 1.1.
func yamlString(s string) string {
	var v map[string]any
	if err := yaml.Unmarshal([]byte("v: "+s), &v); err == nil {
		if got, ok := v["v"].(string); ok && got == s {
			return s
		}
	}
	return strconv.Quote(s)
}
