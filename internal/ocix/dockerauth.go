package ocix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
)

func dockerConfigPath() (string, error) {
	if dir := os.Getenv("DOCKER_CONFIG"); dir != "" {
		return filepath.Join(dir, "config.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve docker config dir: %w", err)
	}
	return filepath.Join(home, ".docker", "config.json"), nil
}

func storedAuthHosts(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg struct {
		Auths       map[string]json.RawMessage `json:"auths"`
		CredHelpers map[string]string          `json:"credHelpers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	hosts := make(map[string]bool, len(cfg.Auths)+len(cfg.CredHelpers))
	for key := range cfg.Auths {
		hosts[hostFromAuthKey(key)] = true
	}
	for key := range cfg.CredHelpers {
		hosts[hostFromAuthKey(key)] = true
	}
	return hosts, nil
}

func hostFromAuthKey(key string) string {
	host := strings.TrimPrefix(key, "https://")
	host = strings.TrimPrefix(host, "http://")
	host, _, _ = strings.Cut(host, "/")
	return host
}

func gatedCredential(stored map[string]bool, delegate auth.CredentialFunc) auth.CredentialFunc {
	return func(ctx context.Context, hostport string) (auth.Credential, error) {
		key := hostFromAuthKey(credentials.ServerAddressFromHostname(hostport))
		if !stored[key] {
			return auth.EmptyCredential, nil
		}
		return delegate(ctx, hostport)
	}
}
