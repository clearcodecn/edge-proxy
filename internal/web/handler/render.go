package handler

// inline rendering helpers used by handlers prior to group 13's template wiring.
// keeps tests deterministic without pulling in the real template engine.

import (
	"fmt"
	"net/http"
	"strings"

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
	body := `<div data-admin-shell="responsive"><button data-mobile-nav="admin" type="button">menu</button><div data-list-controls="domains"></div><div data-list-table-scroll="domains" class="overflow-x-auto"><table id="domains" class="table table-sm min-w-[720px]">`
	for _, d := range list {
		d := d
		body += fmt.Sprintf(`<tr id="d-%d" data-host="%s" data-status="%s"><td>%s</td><td>%s</td></tr>`,
			d.ID, d.Host, d.Status, d.Host, d.Status)
	}
	body += `</table></div></div>`
	writeHTML(w, http.StatusOK, body)
}

func renderUpstreamRow(w http.ResponseWriter, u *store.Upstream) {
	writeHTML(w, http.StatusOK, fmt.Sprintf(
		`<tr id="u-%d" data-addr="%s" data-enabled="%t"><td>%s</td><td>%d</td><td>%t</td></tr>`,
		u.ID, u.Addr, u.Enabled, u.Addr, u.Weight, u.Enabled,
	))
}

func renderUpstreamList(w http.ResponseWriter, list []store.Upstream) {
	body := `<div data-admin-shell="responsive"><button data-mobile-nav="admin" type="button">menu</button><div data-list-controls="upstreams"></div><div data-list-table-scroll="upstreams" class="overflow-x-auto"><table id="upstreams" class="table table-sm min-w-[880px]">`
	for _, u := range list {
		u := u
		body += fmt.Sprintf(`<tr id="u-%d" data-addr="%s" data-enabled="%t"><td>%s</td><td>%d</td><td>%t</td></tr>`,
			u.ID, u.Addr, u.Enabled, u.Addr, u.Weight, u.Enabled)
	}
	body += `</table></div></div>`
	writeHTML(w, http.StatusOK, body)
}

func yesNoBadge(b bool) string {
	if b {
		return `<span class="badge badge-success badge-sm">已配置</span>`
	}
	return `<span class="badge badge-ghost badge-sm">未配置</span>`
}

func configRow(term, value string) string {
	return fmt.Sprintf(`<dt class="text-base-content/60">%s</dt><dd class="min-w-0 break-all">%s</dd>`, term, value)
}

func configCard(title, dlClass string, rows ...string) string {
	return fmt.Sprintf(
		`<div class="card bg-base-100 shadow-sm"><div class="card-body p-5"><h2 class="card-title text-base mb-1">%s</h2><dl class="%s">%s</dl></div></div>`,
		title,
		dlClass,
		strings.Join(rows, ""),
	)
}
