package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// MaxBatchSize bounds the number of items accepted by every batch endpoint
// (deprecate / retry / recycle / enable / disable / delete / import).
// Matches the spec: "单次批量操作不能超过 200 条".
const MaxBatchSize = 200

// writeJSON serialises v as application/json and writes status. It silently
// drops encoding errors after the header is sent — there's nothing useful we
// can do mid-stream and the alternative (panic) would crash the request loop.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// parseIDs reads `ids` either as repeated form values (ids=1&ids=2) or as a
// single comma-separated string (ids=1,2,3). Returns deduplicated int64 IDs.
//
// Go's r.ParseForm only reads the body for POST/PUT/PATCH; DELETE batch
// endpoints carry their ids in a form-encoded body too, so we manually parse
// the body for non-standard methods when ParseForm yields nothing.
func parseIDs(r *http.Request) ([]int64, error) {
	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	raw := r.PostForm["ids"]
	if len(raw) == 0 {
		raw = r.Form["ids"]
	}
	if len(raw) == 0 && r.Body != nil {
		ct := r.Header.Get("Content-Type")
		if strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
			b, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			if err == nil {
				if vals, err := url.ParseQuery(string(b)); err == nil {
					raw = vals["ids"]
				}
			}
		}
	}

	var flat []string
	for _, s := range raw {
		for _, p := range strings.Split(s, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				flat = append(flat, p)
			}
		}
	}

	out := make([]int64, 0, len(flat))
	seen := make(map[int64]bool, len(flat))
	for _, s := range flat {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, err
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, nil
}

// splitHosts normalises a textarea payload (newline / comma / whitespace
// separated) into a deduplicated, lower-cased slice. Empty result MUST be
// treated by callers as "no input"; this function does NOT enforce MaxBatchSize.
func splitHosts(raw string) []string {
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
