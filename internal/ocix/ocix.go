package ocix

import (
	"errors"
	"fmt"

	"github.com/vi-dev/nem/internal/spec"
)

const (
	MediaTypePkgV2    = "application/vnd.nem.pkg.v2+yaml"
	MediaTypePkgV3    = "application/vnd.nem.pkg.v3+yaml"
	MediaTypePkgV4    = "application/vnd.nem.pkg.v4+yaml"
	MediaTypeRecipeV3 = "application/vnd.nem.recipe.v3+yaml"
	MediaTypeRecipeV4 = "application/vnd.nem.recipe.v4+yaml"

	AnnotationSchemaVersion = "org.vi-dev.nem.catalog.schemaVersion"

	// minIndexSchema is the lowest catalog schema an index is written with,
	// so catalogs of schema-2 and schema-3 packages keep their "3".
	minIndexSchema   = 3
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

// MediaTypes returns the pkg and recipe layer media types for a package
// schema. Schema 2 and 3 packages share the v3 layers they have always been
// published with, so their manifest digests do not change.
func MediaTypes(schema int) (pkg, recipe string, err error) {
	switch schema {
	case 2, 3:
		return MediaTypePkgV3, MediaTypeRecipeV3, nil
	case 4:
		return MediaTypePkgV4, MediaTypeRecipeV4, nil
	}
	if schema > spec.MaxSchema {
		return "", "", &spec.UnsupportedSchemaError{Got: schema}
	}
	return "", "", fmt.Errorf("%s: schema must be %d to %d, got %d", spec.PkgFileName, spec.MinSchema, spec.MaxSchema, schema)
}

func pkgLayerSchema(mediaType string) (int, bool) {
	switch mediaType {
	case MediaTypePkgV2:
		return 2, true
	case MediaTypePkgV3:
		return 3, true
	case MediaTypePkgV4:
		return 4, true
	}
	return 0, false
}

func recipeLayerSchema(mediaType string) (int, bool) {
	switch mediaType {
	case MediaTypeRecipeV3:
		return 3, true
	case MediaTypeRecipeV4:
		return 4, true
	}
	return 0, false
}

// layerMatchesSchema reports whether a pkg layer of layerSchema may carry a
// pkg.yaml declaring declared: v4 layers carry exactly schema 4, older layers
// carry schema 2 or 3.
func layerMatchesSchema(layerSchema, declared int) bool {
	if layerSchema == 4 || declared == 4 {
		return layerSchema == declared
	}
	return true
}
