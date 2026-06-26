package nginx

import (
	"bytes"
	"errors"
	"fmt"

	"edge-proxy/internal/store"
)

// RenderBootstrap returns the default :80 server config: catch-all 301 to https.
// certbot --nginx handles its own ACME challenge plumbing temporarily, so no
// explicit /.well-known/acme-challenge location is needed here.
func RenderBootstrap() []byte {
	return []byte(`server {
    listen 80 default_server;
    server_name _;
    location / {
        return 301 https://$host$request_uri;
    }
}
`)
}

// RenderUpstream renders the upstream block from a list of enabled upstreams.
// Returns an error when the list is empty to avoid producing invalid nginx config.
func RenderUpstream(items []store.Upstream) ([]byte, error) {
	if len(items) == 0 {
		return nil, errors.New("upstream pool empty: refusing to render")
	}
	var buf bytes.Buffer
	buf.WriteString("upstream backend {\n")
	for _, u := range items {
		buf.WriteString("    server ")
		buf.WriteString(u.Addr)
		if u.Weight > 1 {
			fmt.Fprintf(&buf, " weight=%d", u.Weight)
		}
		if u.IsBackup {
			buf.WriteString(" backup")
		}
		buf.WriteString(";\n")
	}
	buf.WriteString("}\n")
	return buf.Bytes(), nil
}

// RenderDomain renders a per-domain :443 server block that proxies to the upstream.
// ssl_certificate paths follow the Let's Encrypt default (/etc/letsencrypt/live/<host>/).
func RenderDomain(host string) []byte {
	const tmpl = `server {
    listen 443 ssl http2;
    server_name %s;

    ssl_certificate     /etc/letsencrypt/live/%s/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/%s/privkey.pem;

    location / {
        proxy_pass http://backend;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
`
	return []byte(fmt.Sprintf(tmpl, host, host, host))
}

// FileNameBootstrap is the constant filename of the bootstrap conf.
const FileNameBootstrap = "edge-bootstrap.conf"

// FileNameUpstream is the constant filename of the upstream conf.
const FileNameUpstream = "edge-upstream.conf"

// FileNameDomain returns the per-domain conf filename, e.g. "edge-a.com.conf".
func FileNameDomain(host string) string {
	return "edge-" + host + ".conf"
}
