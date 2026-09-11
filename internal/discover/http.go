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

func httpVersions(ctx context.Context, client *http.Client, h *spec.HTTPDiscovery) ([]Discovered, error) {
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
	vi := versionGroupIndex(re)
	seen := map[string]bool{}
	var out []Discovered
	for _, m := range re.FindAllStringSubmatch(string(body), -1) {
		var v string
		var meta map[string]string
		switch {
		case vi >= 0:
			v = m[vi]
			meta = metaFromMatch(re, m)
		case len(m) > 1:
			v = m[1]
		default:
			v = m[0]
		}
		if v == "" {
			continue
		}
		s := respell(v)
		if !seen[s] {
			seen[s] = true
			out = append(out, Discovered{Version: s, Meta: meta})
		}
	}
	return out, nil
}
