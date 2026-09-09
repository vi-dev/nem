package ocix

import (
	"errors"
	"fmt"
)

const (
	MediaTypePkg = "application/vnd.nem.pkg.v2+yaml"

	MediaTypeArchive = "application/vnd.nem.archive.v2"

	AnnotationSchemaVersion = "org.vi-dev.nem.catalog.schemaVersion"

	SchemaVersion    = "2"
	SchemaVersionInt = 2

	AnnotationTitle = "org.opencontainers.image.title"

	AnnotationDescription = "org.opencontainers.image.description"

	AnnotationVersion = "org.opencontainers.image.version"

	LocalTag = "catalog"

	syncConcurrency = 32
)

var ErrNotSynced = errors.New("catalog store not synced")

var ErrArchiveNotFound = errors.New("archive not found in registry")

type PkgNotInIndexError struct{ Name string }

func (e *PkgNotInIndexError) Error() string {
	return fmt.Sprintf("package %s not in catalog index", e.Name)
}
