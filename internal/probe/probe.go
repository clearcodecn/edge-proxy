// Package probe performs HTTPS reachability checks against domains served by
// this edge-proxy node. Adapted from im-api/internal/pkg/probe; key differences:
//   - No plain HTTP variant — edge-proxy only validates the HTTPS path.
//   - HealthPath is supplied per-call (from config.probe.health_path), not a const.
//   - OK status codes broadened to {200, 204, 301, 302} so SPA/landing pages that
//     redirect to a CDN-prefixed asset path still count as "reachable".
package probe

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

const ProbeTimeout = 3 * time.Second

type ProbeResult struct {
	OK         bool
	StatusCode int
	Err        error
}

// probeHTTPSTransport is a test seam: when non-nil it overrides the default
// transport so httptest.NewTLSServer's self-signed cert can be trusted.
// Production code path keeps http.DefaultTransport (InsecureSkipVerify=false).
var probeHTTPSTransport http.RoundTripper

// CheckHealthyHTTPS issues a GET to https://<host><path> with a 3s overall
// timeout, no redirect follow, and strict TLS verification.
//
// OK is true only when the response status code is 200, 204, 301, or 302.
func CheckHealthyHTTPS(ctx context.Context, host, path string) ProbeResult {
	url := fmt.Sprintf("https://%s%s", host, path)
	client := &http.Client{
		Timeout: ProbeTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	if probeHTTPSTransport != nil {
		client.Transport = probeHTTPSTransport
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ProbeResult{Err: err}
	}
	resp, err := client.Do(req)
	if err != nil {
		return ProbeResult{Err: err}
	}
	defer resp.Body.Close()
	return ProbeResult{
		OK:         isHealthyStatus(resp.StatusCode),
		StatusCode: resp.StatusCode,
	}
}

func isHealthyStatus(c int) bool {
	switch c {
	case http.StatusOK, http.StatusNoContent, http.StatusMovedPermanently, http.StatusFound:
		return true
	}
	return false
}
