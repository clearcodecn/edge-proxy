package web

import (
	"strconv"
	"strings"

	"edge-proxy/internal/store"
)

// pageSize is fixed by spec — every list page returns 50 rows when not in
// chip-search mode. Search mode disables pagination (capped at 200 matches).
const pageSize = 50

// MaxBatchHosts is duplicated from handler.MaxBatchSize but referenced here so
// views.go does not import handler (which would be a cycle).
const MaxBatchHosts = 200

// splitTokens turns a textarea-style payload (newline / comma / whitespace
// separated) into a deduplicated, lower-cased slice. Mirrors handler.splitHosts.
func splitTokens(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == '\t' || r == ' '
	})
	out := make([]string, 0, len(parts))
	seen := make(map[string]bool, len(parts))
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// BuildDomainListView fetches matching domains and packages them with all the
// state the template needs (chip prefill, pagination, truncation banner).
//
// query parameter conventions:
//   - hostsRaw   — raw textarea / chip-string from the search form
//   - status     — exact status enum, or "" / "all" for no filter
//   - pageRaw    — 1-based page number; defaults to 1
func BuildDomainListView(repo *store.DomainRepo, hostsRaw, status, pageRaw string) (DomainListView, error) {
	hosts := splitTokens(hostsRaw)
	truncated := false
	if len(hosts) > MaxBatchHosts {
		hosts = hosts[:MaxBatchHosts]
		truncated = true
	}

	if strings.TrimSpace(status) == "all" {
		status = ""
	}
	status = strings.TrimSpace(status)

	page, _ := strconv.Atoi(pageRaw)
	if page < 1 {
		page = 1
	}

	items, total, err := repo.Search(hosts, status, page, pageSize)
	if err != nil {
		return DomainListView{}, err
	}

	view := DomainListView{
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		Hosts:      hosts,
		HostsStr:   strings.Join(hosts, "\n"),
		Status:     status,
		SearchMode: len(hosts) > 0,
		Truncated:  truncated,
	}
	if view.SearchMode {
		view.TotalPages = 1
	} else {
		view.TotalPages = (total + pageSize - 1) / pageSize
		if view.TotalPages < 1 {
			view.TotalPages = 1
		}
	}
	return view, nil
}

// BuildUpstreamListView is the upstream counterpart. status: "enabled" |
// "disabled" | "" / "all".
func BuildUpstreamListView(repo *store.UpstreamRepo, addrsRaw, status, pageRaw string) (UpstreamListView, error) {
	addrs := splitTokens(addrsRaw)
	truncated := false
	if len(addrs) > MaxBatchHosts {
		addrs = addrs[:MaxBatchHosts]
		truncated = true
	}

	var enabledFilter *bool
	switch strings.TrimSpace(status) {
	case "enabled":
		t := true
		enabledFilter = &t
	case "disabled":
		f := false
		enabledFilter = &f
	}

	page, _ := strconv.Atoi(pageRaw)
	if page < 1 {
		page = 1
	}

	items, total, err := repo.Search(addrs, enabledFilter, page, pageSize)
	if err != nil {
		return UpstreamListView{}, err
	}

	view := UpstreamListView{
		Items:      items,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		Addrs:      addrs,
		AddrsStr:   strings.Join(addrs, "\n"),
		Status:     strings.TrimSpace(status),
		SearchMode: len(addrs) > 0,
		Truncated:  truncated,
	}
	if view.SearchMode {
		view.TotalPages = 1
	} else {
		view.TotalPages = (total + pageSize - 1) / pageSize
		if view.TotalPages < 1 {
			view.TotalPages = 1
		}
	}
	return view, nil
}
