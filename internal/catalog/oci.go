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
	name      string
	storePath string

	mu    sync.Mutex
	store *ocix.Store
	memo  map[string]memoPkg
}

type memoPkg struct {
	pkg *spec.Package
	dig string
}

var _ Source = (*OCI)(nil)

func NewOCI(name, storePath string) *OCI {
	return &OCI{name: name, storePath: storePath, memo: map[string]memoPkg{}}
}

func (o *OCI) Open(ctx context.Context) error {
	_, err := o.open(ctx)
	return err
}

func (o *OCI) open(ctx context.Context) (*ocix.Store, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.store == nil {
		s, err := ocix.OpenLocalStore(ctx, o.storePath)
		if err != nil {
			return nil, err
		}
		o.store = s
	}
	return o.store, nil
}

func (o *OCI) Summaries(ctx context.Context) ([]Summary, error) {
	s, err := o.open(ctx)
	if err != nil {
		return nil, err
	}
	var out []Summary
	for _, m := range s.Index().Manifests {
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

func (o *OCI) Load(ctx context.Context, name string) (*spec.Package, string, error) {
	o.mu.Lock()
	m, ok := o.memo[name]
	o.mu.Unlock()
	if ok {
		return m.pkg, m.dig, nil
	}
	s, err := o.open(ctx)
	if err != nil {
		return nil, "", err
	}
	data, dig, err := s.PkgBytes(ctx, name)
	if _, ok := errors.AsType[*ocix.PkgNotInIndexError](err); ok {
		return nil, "", &PackageNotFoundError{Name: name}
	}
	if err != nil {
		return nil, "", err
	}
	pkg, err := spec.Parse(data)
	if err != nil {
		return nil, "", fmt.Errorf("catalog %s: %w", o.name, err)
	}
	if err := pkg.Validate(); err != nil {
		return nil, "", fmt.Errorf("catalog %s: package %s: %w", o.name, name, err)
	}
	if pkg.Name != name {
		return nil, "", fmt.Errorf("catalog %s: package %s: manifest declares name %q, want %q", o.name, name, pkg.Name, name)
	}
	o.mu.Lock()
	o.memo[name] = memoPkg{pkg: pkg, dig: dig}
	o.mu.Unlock()
	return pkg, dig, nil
}

func (o *OCI) Versions(ctx context.Context, name string) ([]string, error) {
	pkg, _, err := o.Load(ctx, name)
	if err != nil {
		return nil, err
	}
	return versionsOf(pkg), nil
}
