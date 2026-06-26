package nginx

import (
	"strings"
	"testing"

	"edge-proxy/internal/store"
)

func TestRenderBootstrap(t *testing.T) {
	got := string(RenderBootstrap())
	for _, want := range []string{
		"listen 80 default_server",
		"server_name _;",
		"return 301 https://$host$request_uri",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("bootstrap missing %q\nfull:\n%s", want, got)
		}
	}
}

func TestRenderUpstream_Empty(t *testing.T) {
	if _, err := RenderUpstream(nil); err == nil {
		t.Fatal("expected error on empty pool")
	}
}

func TestRenderUpstream_Mixed(t *testing.T) {
	items := []store.Upstream{
		{Addr: "10.0.0.5:80", Weight: 1, IsBackup: false},
		{Addr: "10.0.0.6:80", Weight: 2, IsBackup: false},
		{Addr: "10.0.0.7:80", Weight: 1, IsBackup: true},
	}
	out, err := RenderUpstream(items)
	if err != nil {
		t.Fatalf("RenderUpstream: %v", err)
	}
	got := string(out)
	checks := []struct {
		mustContain string
	}{
		{"upstream backend {"},
		{"server 10.0.0.5:80;"},               // weight=1 omitted
		{"server 10.0.0.6:80 weight=2;"},      // explicit weight
		{"server 10.0.0.7:80 backup;"},        // backup token
		{"}\n"},
	}
	for _, c := range checks {
		if !strings.Contains(got, c.mustContain) {
			t.Errorf("upstream missing %q\nfull:\n%s", c.mustContain, got)
		}
	}
}

func TestRenderDomain(t *testing.T) {
	got := string(RenderDomain("a.example.com"))
	wants := []string{
		"listen 443 ssl http2;",
		"server_name a.example.com;",
		"ssl_certificate     /etc/letsencrypt/live/a.example.com/fullchain.pem;",
		"ssl_certificate_key /etc/letsencrypt/live/a.example.com/privkey.pem;",
		"proxy_pass http://backend;",
		"proxy_set_header Host $host;",
		"proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;",
	}
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("domain conf missing %q\nfull:\n%s", want, got)
		}
	}
}

func TestFileNameDomain(t *testing.T) {
	if got := FileNameDomain("x.com"); got != "edge-x.com.conf" {
		t.Errorf("got %q", got)
	}
}
