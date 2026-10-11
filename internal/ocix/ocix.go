package ocix

import (
	"errors"
	"fmt"
)

const (
	MediaTypePkg    = "application/vnd.nem.pkg.v3+yaml"
	MediaTypePkgV2  = "application/vnd.nem.pkg.v2+yaml"
	MediaTypeRecipe = "application/vnd.nem.recipe.v3+yaml"

	AnnotationSchemaVersion = "org.vi-dev.nem.catalog.schemaVersion"

	CatalogSchema    = "3"
	CatalogSchemaV2  = "2"
	OCISchemaVersion = 2

	AnnotationTitle = "org.opencontainers.image.title"

	AnnotationDescription = "org.opencontainers.image.description"

	AnnotationVersion = "org.opencontainers.image.version"

	LocalTag = "catalog"

	syncConcurrency = 32
)

var ErrNotUpdated = errors.New("catalog store never updated")

type PkgNotInIndexError struct{ Name string }

func (e *PkgNotInIndexError) Error() string {
	return fmt.Sprintf("package %s not in catalog index", e.Name)
}
