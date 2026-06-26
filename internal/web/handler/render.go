package handler

// inline rendering helpers used by handlers prior to group 13's template wiring.
// keeps tests deterministic without pulling in the real template engine.

import (
	"fmt"
	"net/http"

	"edge-proxy/internal/store"
)

func writeHTML(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func renderDomainRow(w http.ResponseWriter, d *store.Domain) {
	writeHTML(w, http.StatusOK, fmt.Sprintf(
		`<tr id="d-%d" data-host="%s" data-status="%s"><td>%s</td><td>%s</td></tr>`,
		d.ID, d.Host, d.Status, d.Host, d.Status,
	))
}

func renderDomainList(w http.ResponseWriter, list []store.Domain) {
	body := `<table id="domains">`
	for _, d := range list {
		d := d
		body += fmt.Sprintf(`<tr id="d-%d" data-host="%s" data-status="%s"><td>%s</td><td>%s</td></tr>`,
			d.ID, d.Host, d.Status, d.Host, d.Status)
	}
	body += `</table>`
	writeHTML(w, http.StatusOK, body)
}

func renderUpstreamRow(w http.ResponseWriter, u *store.Upstream) {
	writeHTML(w, http.StatusOK, fmt.Sprintf(
		`<tr id="u-%d" data-addr="%s" data-enabled="%t"><td>%s</td><td>%d</td><td>%t</td></tr>`,
		u.ID, u.Addr, u.Enabled, u.Addr, u.Weight, u.Enabled,
	))
}

func renderUpstreamList(w http.ResponseWriter, list []store.Upstream) {
	body := `<table id="upstreams">`
	for _, u := range list {
		u := u
		body += fmt.Sprintf(`<tr id="u-%d" data-addr="%s" data-enabled="%t"><td>%s</td><td>%d</td><td>%t</td></tr>`,
			u.ID, u.Addr, u.Enabled, u.Addr, u.Weight, u.Enabled)
	}
	body += `</table>`
	writeHTML(w, http.StatusOK, body)
}
