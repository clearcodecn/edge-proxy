package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"edge-proxy/internal/store"
)

func decodeUpstreamImport(t *testing.T, body string) UpstreamBatchImportResult {
	t.Helper()
	var r UpstreamBatchImportResult
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("decode: %v\nbody: %s", err, body)
	}
	return r
}

func TestUpstream_BatchImport_MixedFieldCounts(t *testing.T) {
	h, repo, nx := newUpstreamHandler(t)
	form := url.Values{}
	form.Set("lines", `
10.0.0.5:80
10.0.0.6:80, 2
10.0.0.7:80, 1, backup
10.0.0.8:8080, 3, , "rack-A 主力"
bad-format-line
10.0.0.9:80, abc
`)
	rec := httptest.NewRecorder()
	h.BatchImportPOST(rec, postForm2("/upstreams/batch", form))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	r := decodeUpstreamImport(t, rec.Body.String())
	if len(r.Created) != 4 {
		t.Errorf("created = %v, want 4", r.Created)
	}
	if len(r.Failed) != 2 {
		t.Errorf("failed = %v, want 2 (bad addr + bad weight)", r.Failed)
	}
	if len(nx.calls) != 1 {
		t.Errorf("nginx refresh called %d times, want 1 after batch", len(nx.calls))
	}

	// Confirm field defaults landed via Repo.
	list, _ := repo.List()
	addrs := map[string]store.Upstream{}
	for _, u := range list {
		addrs[u.Addr] = u
	}
	if u := addrs["10.0.0.7:80"]; !u.IsBackup {
		t.Errorf("10.0.0.7:80 should be backup")
	}
	if u := addrs["10.0.0.8:8080"]; u.Weight != 3 || u.Remark != "rack-A 主力" {
		t.Errorf("10.0.0.8:8080 weight/remark off: %+v", u)
	}
}

func TestUpstream_BatchImport_OverLimit(t *testing.T) {
	h, _, _ := newUpstreamHandler(t)
	var b strings.Builder
	for i := 0; i < 201; i++ {
		b.WriteString("10.0.0.")
		b.WriteString(itoa(i % 256))
		b.WriteString(":80\n")
	}
	form := url.Values{}
	form.Set("lines", b.String())
	rec := httptest.NewRecorder()
	h.BatchImportPOST(rec, postForm2("/upstreams/batch", form))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d", rec.Code)
	}
}

func TestUpstream_BatchImport_Empty(t *testing.T) {
	h, _, _ := newUpstreamHandler(t)
	form := url.Values{}
	form.Set("lines", "\n\n   \n")
	rec := httptest.NewRecorder()
	h.BatchImportPOST(rec, postForm2("/upstreams/batch", form))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d", rec.Code)
	}
}

func TestUpstream_BatchImport_DuplicateAddr(t *testing.T) {
	h, repo, _ := newUpstreamHandler(t)
	_, _ = repo.Create(store.UpstreamInput{Addr: "dup:80"})

	form := url.Values{}
	form.Set("lines", "dup:80\nnew:80")
	rec := httptest.NewRecorder()
	h.BatchImportPOST(rec, postForm2("/upstreams/batch", form))
	r := decodeUpstreamImport(t, rec.Body.String())
	if len(r.Created) != 1 || len(r.Failed) != 1 {
		t.Errorf("got created=%v failed=%v", r.Created, r.Failed)
	}
}

// ── BatchEnable / BatchDisable ────────────────────────────────────────────

func TestUpstream_BatchEnable_OnlyDisabledFlip(t *testing.T) {
	h, repo, nx := newUpstreamHandler(t)
	a, _ := repo.Create(store.UpstreamInput{Addr: "a:80"})
	b, _ := repo.Create(store.UpstreamInput{Addr: "b:80"})
	_ = repo.Toggle(a.ID) // a disabled, b enabled

	form := url.Values{}
	form["ids"] = []string{itoa64(a.ID), itoa64(b.ID)}

	rec := httptest.NewRecorder()
	h.BatchEnablePOST(rec, postForm2("/upstreams/batch/enable", form))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	r := decodeBatchResult(t, rec.Body.String())
	if len(r.Succeeded) != 1 || r.Succeeded[0] != a.ID {
		t.Errorf("succeeded = %v", r.Succeeded)
	}
	if len(r.Failed) != 1 || r.Failed[0].ID != b.ID {
		t.Errorf("failed = %v", r.Failed)
	}
	if len(nx.calls) != 1 {
		t.Errorf("nginx refresh should fire exactly once")
	}
}

func TestUpstream_BatchDisable_AlreadyDisabledRejected(t *testing.T) {
	h, repo, nx := newUpstreamHandler(t)
	a, _ := repo.Create(store.UpstreamInput{Addr: "a:80"})
	b, _ := repo.Create(store.UpstreamInput{Addr: "b:80"})
	// c stays enabled so the pool isn't empty after disabling b — otherwise the
	// nginx renderer refuses to emit (it won't serve a 0-server upstream block),
	// short-circuiting WriteAndApply.
	_, _ = repo.Create(store.UpstreamInput{Addr: "c:80"})
	_ = repo.Toggle(a.ID) // disable a

	form := url.Values{}
	form["ids"] = []string{itoa64(a.ID), itoa64(b.ID)}
	rec := httptest.NewRecorder()
	h.BatchDisablePOST(rec, postForm2("/upstreams/batch/disable", form))
	r := decodeBatchResult(t, rec.Body.String())
	if len(r.Succeeded) != 1 || r.Succeeded[0] != b.ID {
		t.Errorf("succeeded = %v", r.Succeeded)
	}
	if len(r.Failed) != 1 {
		t.Errorf("failed = %v", r.Failed)
	}
	if len(nx.calls) != 1 {
		t.Errorf("nginx refresh should fire once")
	}
}

func TestUpstream_BatchEnable_NoneActuallyChanged(t *testing.T) {
	h, repo, nx := newUpstreamHandler(t)
	a, _ := repo.Create(store.UpstreamInput{Addr: "a:80"})
	form := url.Values{}
	form["ids"] = []string{itoa64(a.ID)} // already enabled
	rec := httptest.NewRecorder()
	h.BatchEnablePOST(rec, postForm2("/upstreams/batch/enable", form))
	r := decodeBatchResult(t, rec.Body.String())
	if len(r.Succeeded) != 0 || len(r.Failed) != 1 {
		t.Errorf("got %+v", r)
	}
	if len(nx.calls) != 0 {
		t.Errorf("nginx refresh should NOT fire when nothing changed")
	}
}

func TestUpstream_BatchEnable_OverLimit(t *testing.T) {
	h, _, _ := newUpstreamHandler(t)
	parts := make([]string, 201)
	for i := range parts {
		parts[i] = itoa(i + 1)
	}
	form := url.Values{}
	form.Set("ids", strings.Join(parts, ","))
	rec := httptest.NewRecorder()
	h.BatchEnablePOST(rec, postForm2("/upstreams/batch/enable", form))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d", rec.Code)
	}
}

// ── BatchDelete ───────────────────────────────────────────────────────────

func TestUpstream_BatchDelete_RemovesRows(t *testing.T) {
	h, repo, _ := newUpstreamHandler(t)
	a, _ := repo.Create(store.UpstreamInput{Addr: "a:80"})
	b, _ := repo.Create(store.UpstreamInput{Addr: "b:80"})
	c, _ := repo.Create(store.UpstreamInput{Addr: "c:80"})

	form := url.Values{}
	form["ids"] = []string{itoa64(a.ID), itoa64(b.ID), "9999"}
	rec := httptest.NewRecorder()
	h.BatchDeletePOST(rec, postForm2("/upstreams/batch", form))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	r := decodeBatchResult(t, rec.Body.String())
	if len(r.Succeeded) != 2 {
		t.Errorf("succeeded = %v", r.Succeeded)
	}
	if len(r.Failed) != 1 || r.Failed[0].ID != 9999 {
		t.Errorf("failed = %v", r.Failed)
	}

	list, _ := repo.List()
	if len(list) != 1 || list[0].ID != c.ID {
		t.Errorf("list after delete = %+v", list)
	}
}

// Locks in the DELETE-body parsing fix in parseIDs(): Go's r.ParseForm()
// only auto-reads the body for POST/PUT/PATCH, so DELETE requests need the
// manual body parse to find their ids.
func TestUpstream_BatchDelete_ReadsBodyOnDeleteMethod(t *testing.T) {
	h, repo, _ := newUpstreamHandler(t)
	a, _ := repo.Create(store.UpstreamInput{Addr: "a:80"})
	b, _ := repo.Create(store.UpstreamInput{Addr: "b:80"})

	form := url.Values{}
	form["ids"] = []string{itoa64(a.ID), itoa64(b.ID)}
	req := httptest.NewRequest("DELETE", "/upstreams/batch", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.BatchDeletePOST(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
	r := decodeBatchResult(t, rec.Body.String())
	if len(r.Succeeded) != 2 {
		t.Errorf("DELETE body not parsed; succeeded = %v", r.Succeeded)
	}
}
