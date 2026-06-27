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

func decodeBatchResult(t *testing.T, body string) BatchResult {
	t.Helper()
	var r BatchResult
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("decode BatchResult: %v\nbody: %s", err, body)
	}
	return r
}

func decodeBatchImport(t *testing.T, body string) BatchImportResult {
	t.Helper()
	var r BatchImportResult
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("decode BatchImportResult: %v\nbody: %s", err, body)
	}
	return r
}

// ── BatchImport ───────────────────────────────────────────────────────────

func TestDomain_BatchImport_AllSucceed(t *testing.T) {
	h, repo, _, _ := newDomainHandler(t)
	form := url.Values{}
	form.Set("hosts", "a.com\nb.com\nc.com")

	rec := httptest.NewRecorder()
	h.BatchImportPOST(rec, postForm2("/domains/batch", form))

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
	r := decodeBatchImport(t, rec.Body.String())
	if len(r.Created) != 3 || len(r.Skipped) != 0 || len(r.Failed) != 0 {
		t.Errorf("got created=%v skipped=%v failed=%v", r.Created, r.Skipped, r.Failed)
	}
	list, _ := repo.List()
	if len(list) != 3 {
		t.Errorf("repo has %d rows, want 3", len(list))
	}
}

func TestDomain_BatchImport_PartialSkipsAndFails(t *testing.T) {
	h, repo, _, _ := newDomainHandler(t)
	_, _ = repo.Create("dup.com") // pre-existing duplicate
	form := url.Values{}
	form.Set("hosts", "new.com\ndup.com\nbad..host\n,, ,\n new.com ") // last is a dedup of new.com after trim

	rec := httptest.NewRecorder()
	h.BatchImportPOST(rec, postForm2("/domains/batch", form))

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, body = %s", rec.Code, rec.Body.String())
	}
	r := decodeBatchImport(t, rec.Body.String())
	if len(r.Created) != 1 || len(r.Skipped) != 1 || len(r.Failed) != 1 {
		t.Errorf("got created=%v skipped=%v failed=%v", r.Created, r.Skipped, r.Failed)
	}
	if r.Skipped[0] != "dup.com" {
		t.Errorf("skipped[0] = %q", r.Skipped[0])
	}
	if r.Failed[0].Host != "bad..host" {
		t.Errorf("failed[0] = %+v", r.Failed[0])
	}
}

func TestDomain_BatchImport_OverLimit(t *testing.T) {
	h, _, _, _ := newDomainHandler(t)
	var b strings.Builder
	for i := 0; i < 201; i++ {
		b.WriteString("h")
		b.WriteString(itoa(i))
		b.WriteString(".com\n")
	}
	form := url.Values{}
	form.Set("hosts", b.String())

	rec := httptest.NewRecorder()
	h.BatchImportPOST(rec, postForm2("/domains/batch", form))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "200") {
		t.Errorf("body should mention limit: %s", rec.Body.String())
	}
}

func TestDomain_BatchImport_EmptyInput(t *testing.T) {
	h, _, _, _ := newDomainHandler(t)
	form := url.Values{}
	form.Set("hosts", "   \n\n ,, ")
	rec := httptest.NewRecorder()
	h.BatchImportPOST(rec, postForm2("/domains/batch", form))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", rec.Code)
	}
}

// ── BatchDeprecate ────────────────────────────────────────────────────────

func TestDomain_BatchDeprecate_PartialSuccess(t *testing.T) {
	h, repo, _, _ := newDomainHandler(t)
	a, _ := repo.Create("a.com")
	b, _ := repo.Create("b.com")
	c, _ := repo.Create("c.com")
	_ = repo.UpdateStatus(c.ID, store.StatusDeprecated)

	form := url.Values{}
	form["ids"] = []string{itoa64(a.ID), itoa64(b.ID), itoa64(c.ID), "9999"}

	rec := httptest.NewRecorder()
	h.BatchDeprecatePOST(rec, postForm2("/domains/batch/deprecate", form))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	r := decodeBatchResult(t, rec.Body.String())
	if len(r.Succeeded) != 2 {
		t.Errorf("succeeded = %v, want 2", r.Succeeded)
	}
	if len(r.Failed) != 2 {
		t.Errorf("failed = %v, want 2 (already deprecated + not found)", r.Failed)
	}
}

func TestDomain_BatchDeprecate_OverLimit(t *testing.T) {
	h, _, _, _ := newDomainHandler(t)
	form := url.Values{}
	parts := make([]string, 201)
	for i := range parts {
		parts[i] = itoa(i + 1)
	}
	form.Set("ids", strings.Join(parts, ","))
	rec := httptest.NewRecorder()
	h.BatchDeprecatePOST(rec, postForm2("/domains/batch/deprecate", form))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", rec.Code)
	}
}

func TestDomain_BatchDeprecate_EmptyIDs(t *testing.T) {
	h, _, _, _ := newDomainHandler(t)
	form := url.Values{}
	rec := httptest.NewRecorder()
	h.BatchDeprecatePOST(rec, postForm2("/domains/batch/deprecate", form))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", rec.Code)
	}
}

// ── BatchRetry ────────────────────────────────────────────────────────────

func TestDomain_BatchRetry_OnlyFailedReset(t *testing.T) {
	h, repo, _, _ := newDomainHandler(t)
	a, _ := repo.Create("ok.com")
	b, _ := repo.Create("fail.com")
	_ = repo.MarkCertFailed(b.ID, "boom")
	_ = repo.UpdateStatus(a.ID, store.StatusOnline)

	form := url.Values{}
	form["ids"] = []string{itoa64(a.ID), itoa64(b.ID)}

	rec := httptest.NewRecorder()
	h.BatchRetryPOST(rec, postForm2("/domains/batch/retry", form))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	r := decodeBatchResult(t, rec.Body.String())
	if len(r.Succeeded) != 1 || r.Succeeded[0] != b.ID {
		t.Errorf("succeeded = %v, want [%d]", r.Succeeded, b.ID)
	}
	if len(r.Failed) != 1 || r.Failed[0].ID != a.ID {
		t.Errorf("failed = %v, want a", r.Failed)
	}
}

// ── BatchRecycle ──────────────────────────────────────────────────────────

func TestDomain_BatchRecycle_FullCleanup(t *testing.T) {
	prev := recycleReloadThrottle
	recycleReloadThrottle = 0
	defer func() { recycleReloadThrottle = prev }()

	h, repo, cb, nx := newDomainHandler(t)
	a, _ := repo.Create("dep1.com")
	b, _ := repo.Create("dep2.com")
	c, _ := repo.Create("keep.com")
	_ = repo.UpdateStatus(a.ID, store.StatusDeprecated)
	_ = repo.UpdateStatus(b.ID, store.StatusDeprecated)
	_ = repo.UpdateStatus(c.ID, store.StatusOnline)

	form := url.Values{}
	form["ids"] = []string{itoa64(a.ID), itoa64(b.ID), itoa64(c.ID), "4242"}

	rec := httptest.NewRecorder()
	h.BatchRecyclePOST(rec, postForm2("/domains/batch/recycle", form))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	r := decodeBatchResult(t, rec.Body.String())
	if len(r.Succeeded) != 2 {
		t.Errorf("succeeded = %v, want 2 (a,b)", r.Succeeded)
	}
	if len(r.Failed) != 2 {
		t.Errorf("failed = %v, want 2 (c online, 4242 missing)", r.Failed)
	}

	// nginx ops fired twice (per-domain test+reload)
	if nx.tested.Load() != 2 {
		t.Errorf("nginx.TestConfig called %d times, want 2", nx.tested.Load())
	}
	if nx.reloaded.Load() != 2 {
		t.Errorf("nginx.Reload called %d times, want 2", nx.reloaded.Load())
	}
	if len(nx.removed) != 2 {
		t.Errorf("nginx.RemoveFile called %d times: %v", len(nx.removed), nx.removed)
	}

	// LE delete is fired in goroutines; wait for both.
	cb.waitForDelete(t, "dep1.com")
	cb.waitForDelete(t, "dep2.com")

	// Survivor still present.
	if got, err := repo.GetByID(c.ID); err != nil || got == nil {
		t.Errorf("keep.com should remain, got %v", err)
	}
}

func TestDomain_BatchRecycle_AllRejected(t *testing.T) {
	prev := recycleReloadThrottle
	recycleReloadThrottle = 0
	defer func() { recycleReloadThrottle = prev }()

	h, repo, _, nx := newDomainHandler(t)
	a, _ := repo.Create("on.com") // not deprecated
	form := url.Values{}
	form["ids"] = []string{itoa64(a.ID)}

	rec := httptest.NewRecorder()
	h.BatchRecyclePOST(rec, postForm2("/domains/batch/recycle", form))
	r := decodeBatchResult(t, rec.Body.String())
	if len(r.Succeeded) != 0 || len(r.Failed) != 1 {
		t.Errorf("got succ=%v failed=%v", r.Succeeded, r.Failed)
	}
	if nx.tested.Load() != 0 || nx.reloaded.Load() != 0 {
		t.Errorf("nginx ops should not run when nothing was deletable")
	}
}

// ── helpers ───────────────────────────────────────────────────────────────

func itoa(i int) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = digits[i%10]
		i /= 10
	}
	return string(buf[pos:])
}

func itoa64(i int64) string { return itoa(int(i)) }
