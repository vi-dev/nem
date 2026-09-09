package discover

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"

	"github.com/vi-dev/nem/internal/spec"
)

const maxHTTPBody = 8 << 20

func httpVersions(ctx context.Context, client *http.Client, h *spec.HTTPDiscovery) ([]string, error) {
	re, err := regexp.Compile(h.Filter)
	if err != nil {
		return nil, fmt.Errorf("discovery filter: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request for %s: %w", h.URL, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", h.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: unexpected status %s", h.URL, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxHTTPBody+1))
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", h.URL, err)
	}
	if len(body) > maxHTTPBody {
		return nil, fmt.Errorf("fetch %s: response exceeds %d bytes", h.URL, maxHTTPBody)
	}
	seen := map[string]bool{}
	var out []string
	for _, m := range re.FindAllSubmatch(body, -1) {
		v := m[0]
		if len(m) > 1 {
			v = m[1]
		}
		if len(v) == 0 {
			continue
		}
		s := respell(string(v))
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out, nil
}
