package archive

import (
	"fmt"

	"oras.land/oras-go/v2/registry"

	"github.com/vi-dev/nem/internal/spec"
)

func validName(name string) error {
	if !spec.NameRE.MatchString(name) {
		return fmt.Errorf("invalid package name %q for archive store", name)
	}
	return nil
}

func RefPrefix(catalogRef string) (string, error) {
	parsed, err := registry.ParseReference(catalogRef)
	if err != nil {
		return "", fmt.Errorf("parse catalog ref %q: %w", catalogRef, err)
	}
	return fmt.Sprintf("%s/%s/archives/", parsed.Registry, parsed.Repository), nil
}

func Ref(catalogRef, name string) (string, error) {
	if err := validName(name); err != nil {
		return "", err
	}
	prefix, err := RefPrefix(catalogRef)
	if err != nil {
		return "", err
	}
	return prefix + name, nil
}
