package catalog

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/vi-dev/nem/internal/ocix"
	"github.com/vi-dev/nem/internal/spec"
)

type OCI struct {
	name string
	open func(ctx context.Context) (*ocix.Store, error)

	storeMu sync.Mutex
	store   *ocix.Store

	memoMu sync.Mutex
	memo   map[string]catalogMemo
}

type catalogMemo struct {
	pkg *spec.Package
	dig string
}

var _ Catalog = (*OCI)(nil)

func NewOCI(name string, s *ocix.Store) *OCI {
	return &OCI{name: name, store: s, memo: map[string]catalogMemo{}}
}

func NewLazyOCI(name string, open func(ctx context.Context) (*ocix.Store, error)) *OCI {
	return &OCI{name: name, open: open, memo: map[string]catalogMemo{}}
}

func (s *OCI) openStore(ctx context.Context) (*ocix.Store, error) {
	s.storeMu.Lock()
	defer s.storeMu.Unlock()
	if s.store == nil {
		store, err := s.open(ctx)
		if err != nil {
			return nil, err
		}
		s.store = store
	}
	return s.store, nil
}

func (s *OCI) Open(ctx context.Context) error {
	_, err := s.openStore(ctx)
	return err
}

func (s *OCI) Summaries(ctx context.Context) ([]Summary, error) {
	store, err := s.openStore(ctx)
	if err != nil {
		return nil, err
	}
	var out []Summary
	for _, m := range store.Index().Manifests {
		name := m.Annotations[ocix.AnnotationTitle]
		if name == "" {
			continue
		}
		out = append(out, Summary{
			Name:        name,
			Description: m.Annotations[ocix.AnnotationDescription],
			Latest:      m.Annotations[ocix.AnnotationVersion],
		})
	}
	return out, nil
}

func (s *OCI) PackageNames(ctx context.Context) ([]string, error) {
	store, err := s.openStore(ctx)
	if err != nil {
		return nil, err
	}
	pkgs := store.Packages()
	out := make([]string, 0, len(pkgs))
	for _, m := range pkgs {
		out = append(out, m.Title)
	}
	return out, nil
}

func (s *OCI) Package(ctx context.Context, name string) (*spec.Package, string, error) {
	store, err := s.openStore(ctx)
	if err != nil {
		return nil, "", err
	}

	s.memoMu.Lock()
	m, ok := s.memo[name]
	s.memoMu.Unlock()
	if ok {
		return m.pkg, m.dig, nil
	}

	pkg, dig, err := loadFromStore(ctx, store, s.name, name)
	if err != nil {
		return nil, "", err
	}

	s.memoMu.Lock()
	s.memo[name] = catalogMemo{pkg: pkg, dig: dig}
	s.memoMu.Unlock()
	return pkg, dig, nil
}

func (s *OCI) ReadManifest(ctx context.Context, name string) ([]byte, error) {
	store, err := s.openStore(ctx)
	if err != nil {
		return nil, err
	}
	data, _, err := store.PkgBytes(ctx, name)
	if _, ok := errors.AsType[*ocix.PkgNotInIndexError](err); ok {
		return nil, &PackageNotFoundError{Name: name}
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (s *OCI) Versions(ctx context.Context, name string) ([]string, error) {
	pkg, _, err := s.Package(ctx, name)
	if err != nil {
		return nil, err
	}
	return versionsOf(pkg), nil
}

func loadFromStore(ctx context.Context, store *ocix.Store, catalogName, name string) (*spec.Package, string, error) {
	data, dig, err := store.PkgBytes(ctx, name)
	if _, ok := errors.AsType[*ocix.PkgNotInIndexError](err); ok {
		return nil, "", &PackageNotFoundError{Name: name}
	}
	if err != nil {
		return nil, "", err
	}
	pkg, err := spec.Parse(data)
	if err != nil {
		return nil, "", fmt.Errorf("catalog %s: %w", catalogName, err)
	}
	if err := pkg.Validate(); err != nil {
		return nil, "", fmt.Errorf("catalog %s: package %s: %w", catalogName, name, err)
	}
	if pkg.Name != name {
		return nil, "", fmt.Errorf("catalog %s: package %s: manifest declares name %q, want %q",
			catalogName, name, pkg.Name, name)
	}
	return pkg, dig, nil
}
