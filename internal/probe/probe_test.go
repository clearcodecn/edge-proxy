package probe

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func mustHost(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return u.Host
}

func withHTTPSTransport(t *testing.T, srv *httptest.Server) {
	t.Helper()
	prev := probeHTTPSTransport
	probeHTTPSTransport = srv.Client().Transport
	t.Cleanup(func() { probeHTTPSTransport = prev })
}

func tlsServer(t *testing.T, status int, observedPath *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if observedPath != nil {
			*observedPath = r.URL.Path
		}
		if status >= 300 && status < 400 {
			w.Header().Set("Location", "/elsewhere")
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	withHTTPSTransport(t, srv)
	return srv
}

func TestProbe_StatusOK(t *testing.T) {
	cases := []struct {
		name string
		code int
	}{
		{"200", 200},
		{"204", 204},
		{"301", 301},
		{"302", 302},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			srv := tlsServer(t, c.code, nil)
			r := CheckHealthyHTTPS(context.Background(), mustHost(t, srv.URL), "/")
			if !r.OK {
				t.Errorf("status %d: expected OK, got %+v", c.code, r)
			}
			if r.StatusCode != c.code {
				t.Errorf("got code %d", r.StatusCode)
			}
		})
	}
}

func TestProbe_StatusFailureCodes(t *testing.T) {
	cases := []int{303, 307, 308, 400, 401, 403, 404, 500, 502, 503}
	for _, code := range cases {
		code := code
		t.Run(http.StatusText(code), func(t *testing.T) {
			srv := tlsServer(t, code, nil)
			r := CheckHealthyHTTPS(context.Background(), mustHost(t, srv.URL), "/")
			if r.OK {
				t.Errorf("status %d should not be OK", code)
			}
			if r.StatusCode != code {
				t.Errorf("got code %d, want %d", r.StatusCode, code)
			}
			if r.Err != nil {
				t.Errorf("expected nil err, got %v", r.Err)
			}
		})
	}
}

func TestProbe_PathPropagated(t *testing.T) {
	var observed string
	srv := tlsServer(t, 200, &observed)
	_ = CheckHealthyHTTPS(context.Background(), mustHost(t, srv.URL), "/api/_healthy")
	if observed != "/api/_healthy" {
		t.Errorf("observed path = %q", observed)
	}
}

func TestProbe_NoRedirectFollow(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path == "/" {
			w.Header().Set("Location", "/elsewhere")
			w.WriteHeader(http.StatusMovedPermanently)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	withHTTPSTransport(t, srv)

	r := CheckHealthyHTTPS(context.Background(), mustHost(t, srv.URL), "/")
	if !r.OK {
		t.Errorf("301 should still be OK without follow, got %+v", r)
	}
	if r.StatusCode != 301 {
		t.Errorf("StatusCode = %d", r.StatusCode)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("expected 1 hit, got %d", got)
	}
}

func TestProbe_Timeout(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(10 * time.Second):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	withHTTPSTransport(t, srv)

	start := time.Now()
	r := CheckHealthyHTTPS(context.Background(), mustHost(t, srv.URL), "/")
	elapsed := time.Since(start)
	if r.OK {
		t.Fatal("expected not OK")
	}
	if r.Err == nil {
		t.Fatal("expected error")
	}
	if elapsed > 4*time.Second {
		t.Errorf("expected ~3s timeout, took %v", elapsed)
	}
	var ne net.Error
	if errors.As(r.Err, &ne) && ne.Timeout() {
		return
	}
	if errors.Is(r.Err, context.DeadlineExceeded) || os.IsTimeout(r.Err) {
		return
	}
	t.Fatalf("expected timeout error, got %v", r.Err)
}

func TestProbe_ConnectionRefused(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	host := mustHost(t, srv.URL)
	withHTTPSTransport(t, srv)
	srv.Close()

	r := CheckHealthyHTTPS(context.Background(), host, "/")
	if r.OK {
		t.Fatal("expected not OK")
	}
	if r.Err == nil {
		t.Fatal("expected error")
	}
}

// TestProbe_TLSVerifyEnforced asserts that the production code path does NOT
// trust self-signed certs (InsecureSkipVerify stays false).
func TestProbe_TLSVerifyEnforced(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	// Deliberately skip withHTTPSTransport.

	r := CheckHealthyHTTPS(context.Background(), mustHost(t, srv.URL), "/")
	if r.OK {
		t.Fatal("expected not OK due to TLS verification failure")
	}
	if r.Err == nil {
		t.Fatal("expected TLS error")
	}
	errStr := r.Err.Error()
	if !strings.Contains(errStr, "x509") && !strings.Contains(errStr, "certificate") && !strings.Contains(errStr, "tls:") {
		t.Errorf("expected TLS-related error, got: %v", r.Err)
	}
}
