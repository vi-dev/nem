package ocix

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"oras.land/oras-go/v2/registry/remote/auth"
)

func writeDockerConfig(t *testing.T, contents string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(contents), 0o600); err != nil {
		t.Fatalf("write config.json: %v", err)
	}
	t.Setenv("DOCKER_CONFIG", dir)
}

func repoCredential(t *testing.T, host string) (auth.Credential, error) {
	t.Helper()
	repo, err := NewRemoteRepository("ghcr.io/vi-dev/test-catalog:v2")
	if err != nil {
		t.Fatalf("NewRemoteRepository: %v", err)
	}
	client := repo.Client.(*auth.Client)
	return client.Credential(context.Background(), host)
}

func TestCredentialAnonymousWithoutStoredLogin(t *testing.T) {

	writeDockerConfig(t, `{"credsStore":"nem-test-absent-helper"}`)

	cred, err := repoCredential(t, "ghcr.io")
	if err != nil {
		t.Fatalf("want anonymous credential without exec'ing helper, got error: %v", err)
	}
	if cred != auth.EmptyCredential {
		t.Fatalf("want EmptyCredential, got %+v", cred)
	}
}

func TestCredentialNoConfigFileIsAnonymous(t *testing.T) {
	t.Setenv("DOCKER_CONFIG", t.TempDir())

	cred, err := repoCredential(t, "ghcr.io")
	if err != nil || cred != auth.EmptyCredential {
		t.Fatalf("want EmptyCredential without config file, got %+v, %v", cred, err)
	}
}

func TestCredentialDockerHubKeyMatchesHubHost(t *testing.T) {

	writeDockerConfig(t, `{"auths":{"https://index.docker.io/v1/":{}},"credsStore":"nem-test-absent-helper"}`)

	if _, err := repoCredential(t, "registry-1.docker.io"); err == nil {
		t.Fatal("want helper-exec error for logged-in Docker Hub, got nil")
	}
}

func TestCredentialSchemePrefixedKeyMatchesHost(t *testing.T) {
	writeDockerConfig(t, `{"auths":{"https://ghcr.io":{}},"credsStore":"nem-test-absent-helper"}`)

	if _, err := repoCredential(t, "ghcr.io"); err == nil {
		t.Fatal("want helper-exec error for scheme-prefixed auths key, got nil")
	}
}

func TestCredentialConsultsPerHostCredHelper(t *testing.T) {

	writeDockerConfig(t, `{"credHelpers":{"registry.example.com":"nem-test-absent-helper"}}`)

	if _, err := repoCredential(t, "registry.example.com"); err == nil {
		t.Fatal("want helper-exec error for credHelpers registry, got nil")
	}
}

func TestStoredAuthHostsRejectsMalformedConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write config.json: %v", err)
	}

	if _, err := storedAuthHosts(path); err == nil {
		t.Fatal("want parse error for malformed config, got nil")
	}
}

func TestCredentialConsultsStoreForStoredLogin(t *testing.T) {

	writeDockerConfig(t, `{"auths":{"ghcr.io":{}},"credsStore":"nem-test-absent-helper"}`)

	if _, err := repoCredential(t, "ghcr.io"); err == nil {
		t.Fatal("want helper-exec error for logged-in registry, got nil")
	}
}
