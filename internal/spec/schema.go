package spec

import (
	"fmt"

	"github.com/goccy/go-yaml"
)

// MinSchema and MaxSchema bound the pkg.yaml schema versions this nem reads.
// A package declares the lowest schema that can express it.
const (
	MinSchema = 2
	MaxSchema = 4
)

// SchemaExcluded is the first schema that allows excluded versions.
const SchemaExcluded = 4

type UnsupportedSchemaError struct {
	Name string
	Got  int
}

func (e *UnsupportedSchemaError) Error() string {
	if e.Name == "" {
		return fmt.Sprintf("%s uses schema %d; this nem supports up to %d", PkgFileName, e.Got, MaxSchema)
	}
	return fmt.Sprintf("package %s uses schema %d; this nem supports up to %d", e.Name, e.Got, MaxSchema)
}

// PeekSchema reads only the schema and name keys, leniently, so a manifest
// written for a newer schema is recognised before strict parsing rejects its
// unknown keys.
func PeekSchema(data []byte) (schema int, name string) {
	var head struct {
		Schema int    `yaml:"schema"`
		Name   string `yaml:"name"`
	}
	_ = yaml.Unmarshal(data, &head)
	return head.Schema, head.Name
}

func checkSchemaSupported(data []byte) error {
	if schema, name := PeekSchema(data); schema > MaxSchema {
		return &UnsupportedSchemaError{Name: name, Got: schema}
	}
	return nil
}
