package ocix

import (
	"fmt"
	"net"
	"strings"

	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"

	"github.com/vi-dev/nem/internal/netx"
)

func WithTagOrDigest(ref string) error {
	parsed, err := registry.ParseReference(ref)
	if err != nil {
		return fmt.Errorf("parse oci ref %q: %w", ref, err)
	}
	if parsed.Reference == "" {
		return fmt.Errorf("oci ref %q has no tag or digest", ref)
	}
	return nil
}

func WithoutTagOrDigest(ref string) error {
	parsed, err := registry.ParseReference(ref)
	if err != nil {
		return fmt.Errorf("parse oci ref %q: %w", ref, err)
	}
	if parsed.Reference != "" {
		return fmt.Errorf("oci ref %q must be a bare repository ref", ref)
	}
	return nil
}

func loopbackHost(host string) bool {
	h := host
	if hostOnly, _, err := net.SplitHostPort(host); err == nil {
		h = hostOnly
	}
	h = strings.Trim(h, "[]")
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func NewRemoteRepository(ref string) (*remote.Repository, error) {
	repo, err := remote.NewRepository(ref)
	if err != nil {
		return nil, fmt.Errorf("parse oci ref %q: %w", ref, err)
	}
	host := repo.Reference.Registry
	explicit, configured := netx.ForHost(host)
	repo.PlainHTTP = explicit.PlainHTTP || (!configured && loopbackHost(host))

	client, err := netx.HostClient(host)
	if err != nil {
		return nil, fmt.Errorf("configure registry client for %s: %w", host, err)
	}
	credStore, err := credentials.NewStoreFromDocker(credentials.StoreOptions{})
	if err != nil {
		return nil, fmt.Errorf("open docker credentials: %w", err)
	}
	configPath, err := dockerConfigPath()
	if err != nil {
		return nil, err
	}
	stored, err := storedAuthHosts(configPath)
	if err != nil {
		return nil, fmt.Errorf("read docker config: %w", err)
	}
	repo.Client = &auth.Client{
		Client:     client,
		Credential: gatedCredential(stored, credentials.Credential(credStore)),
		Cache:      auth.NewCache(),
	}
	return repo, nil
}

func RemoteCatalog(ref string) (oras.ReadOnlyTarget, string, error) {
	repo, err := NewRemoteRepository(ref)
	if err != nil {
		return nil, "", err
	}
	return repo, repo.Reference.Reference, nil
}

func RemoteCatalogRW(ref string) (oras.Target, string, error) {
	repo, err := NewRemoteRepository(ref)
	if err != nil {
		return nil, "", err
	}
	return repo, repo.Reference.Reference, nil
}
