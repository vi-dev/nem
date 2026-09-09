package netx

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"maps"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"oras.land/oras-go/v2/registry/remote/retry"
)

const (
	dialTimeout           = 10 * time.Second
	tlsHandshakeTimeout   = 10 * time.Second
	responseHeaderTimeout = 30 * time.Second
)

var client = &http.Client{Transport: newTransport()}

func Client() *http.Client { return client }

func newTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   dialTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   tlsHandshakeTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: responseHeaderTimeout,
	}
}

type HostSettings struct {
	CA        string
	PlainHTTP bool
	Insecure  bool
}

var (
	mu      sync.Mutex
	hosts   map[string]HostSettings
	clients map[string]*http.Client
)

func Set(h map[string]HostSettings) {
	cp := make(map[string]HostSettings, len(h))
	maps.Copy(cp, h)
	mu.Lock()
	defer mu.Unlock()
	hosts = cp
	clients = nil
}

func ForHost(host string) (HostSettings, bool) {
	mu.Lock()
	defer mu.Unlock()
	s, ok := hosts[host]
	return s, ok
}

func HostClient(host string) (*http.Client, error) {
	mu.Lock()
	defer mu.Unlock()
	if c, ok := clients[host]; ok {
		return c, nil
	}
	c, err := buildHostClient(hosts[host])
	if err != nil {
		return nil, err
	}
	if clients == nil {
		clients = make(map[string]*http.Client)
	}
	clients[host] = c
	return c, nil
}

func buildHostClient(s HostSettings) (*http.Client, error) {
	t := newTransport()
	switch {
	case s.CA != "":
		pool, err := loadCAPool(s.CA)
		if err != nil {
			return nil, fmt.Errorf("load ca %s: %w", s.CA, err)
		}
		t.TLSClientConfig = &tls.Config{RootCAs: pool}
	case s.Insecure:
		t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &http.Client{Transport: retry.NewTransport(t)}, nil
}

func loadCAPool(path string) (*x509.CertPool, error) {
	pemBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemBytes) {
		return nil, fmt.Errorf("%s contains no valid PEM certificates", path)
	}
	return pool, nil
}
