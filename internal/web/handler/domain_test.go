package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"edge-proxy/internal/store"
)

// ── fakes ─────────────────────────────────────────────────────────────────

// fakeCertbotForWeb records every Delete(host) call. Mutex-guarded because
// BatchRecycle fires one goroutine per deleted domain — the original
// atomic.Value implementation lost entries under concurrent append.
type fakeCertbotForWeb struct {
	mu      sync.Mutex
	deleted []string
}

func (f *fakeCertbotForWeb) Delete(host string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, host)
	return nil
}

func (f *fakeCertbotForWeb) snapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.deleted))
	copy(out, f.deleted)
	return out
}

func (f *fakeCertbotForWeb) waitForDelete(t *testing.T, host string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if contains(f.snapshot(), host) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("certbot delete not invoked for %s", host)
}

func contains(s []string, target string) bool {
	for _, v := range s {
		if v == target {
			return true
		}
	}
	return false
}

type fakeNginxForWeb struct {
	removed []string
	testErr error
	tested  atomic.Int32
	reloaded atomic.Int32
}

func (f *fakeNginxForWeb) RemoveFile(filename string) error {
	f.removed = append(f.removed, filename)
	return nil
}
func (f *fakeNginxForWeb) TestConfig() error {
	f.tested.Add(1)
	return f.testErr
}
func (f *fakeNginxForWeb) Reload() error {
	f.reloaded.Add(1)
	return nil
}

func newDomainHandler(t *testing.T) (*DomainHandler, *store.DomainRepo, *fakeCertbotForWeb, *fakeNginxForWeb) {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "edge.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	repo := store.NewDomainRepo(db)
	cb := &fakeCertbotForWeb{}
	nx := &fakeNginxForWeb{}
	return NewDomainHandler(repo, cb, nx), repo, cb, nx
}

func reqWithID(method, path string, id int64) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", strconv.FormatInt(id, 10))
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func postForm2(path string, form url.Values) *http.Request {
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

// ── tests ─────────────────────────────────────────────────────────────────

func TestDomain_Create_Success(t *testing.T) {
	h, repo, _, _ := newDomainHandler(t)
	form := url.Values{}
	form.Set("host", "example.com")

	rec := httptest.NewRecorder()
	h.CreatePOST(rec, postForm2("/domains", form))

	if rec.Code != http.StatusOK {
		t.Errorf("code = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "example.com") {
		t.Errorf("body should contain host: %s", rec.Body.String())
	}
	got, _ := repo.GetByHost("example.com")
	if got == nil {
		t.Fatal("domain not persisted")
	}
	if got.Status != store.StatusPending {
		t.Errorf("status = %q", got.Status)
	}
}

func TestDomain_Create_InvalidHostRejected(t *testing.T) {
	h, _, _, _ := newDomainHandler(t)
	for _, bad := range []string{"", " ", "no", "a..b", "-bad.com", "bad-.com", " spaces .com", "https://x.com"} {
		form := url.Values{}
		form.Set("host", bad)
		rec := httptest.NewRecorder()
		h.CreatePOST(rec, postForm2("/domains", form))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("host %q should be rejected, got %d", bad, rec.Code)
		}
	}
}

func TestDomain_Create_DuplicateReturns400(t *testing.T) {
	h, _, _, _ := newDomainHandler(t)
	form := url.Values{}
	form.Set("host", "dup.com")
	h.CreatePOST(httptest.NewRecorder(), postForm2("/domains", form))

	rec := httptest.NewRecorder()
	h.CreatePOST(rec, postForm2("/domains", form))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("dup code = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "域名已存在") {
		t.Errorf("body = %q", rec.Body.String())
	}
}

func TestDomain_DeprecateOnline(t *testing.T) {
	h, repo, _, _ := newDomainHandler(t)
	d, _ := repo.Create("ok.com")
	_ = repo.UpdateStatus(d.ID, store.StatusOnline)

	rec := httptest.NewRecorder()
	h.DeprecatePOST(rec, reqWithID("POST", "/domains/"+strconv.FormatInt(d.ID, 10)+"/deprecate", d.ID))
	if rec.Code != http.StatusOK {
		t.Errorf("code = %d", rec.Code)
	}
	got, _ := repo.GetByID(d.ID)
	if got.Status != store.StatusDeprecated {
		t.Errorf("status = %q", got.Status)
	}
}

func TestDomain_DeprecateAlreadyDeprecated_Rejects(t *testing.T) {
	h, repo, _, _ := newDomainHandler(t)
	d, _ := repo.Create("dep.com")
	_ = repo.UpdateStatus(d.ID, store.StatusDeprecated)

	rec := httptest.NewRecorder()
	h.DeprecatePOST(rec, reqWithID("POST", "/x", d.ID))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d", rec.Code)
	}
}

func TestDomain_RecycleNonDeprecated_Rejects(t *testing.T) {
	h, repo, cb, nx := newDomainHandler(t)
	d, _ := repo.Create("on.com")
	_ = repo.UpdateStatus(d.ID, store.StatusOnline)

	rec := httptest.NewRecorder()
	h.RecyclePOST(rec, reqWithID("POST", "/x", d.ID))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
	if got, _ := repo.GetByID(d.ID); got == nil {
		t.Error("row should still exist")
	}
	if len(nx.removed) != 0 {
		t.Errorf("nginx.RemoveFile should not be called, got %v", nx.removed)
	}
	if got := cb.snapshot(); len(got) != 0 {
		t.Errorf("certbot.Delete should not be called, got %v", got)
	}
}

func TestDomain_RecycleDeprecated_FullCleanup(t *testing.T) {
	h, repo, cb, nx := newDomainHandler(t)
	d, _ := repo.Create("gone.com")
	_ = repo.UpdateStatus(d.ID, store.StatusDeprecated)

	rec := httptest.NewRecorder()
	h.RecyclePOST(rec, reqWithID("POST", "/x", d.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if _, err := repo.GetByID(d.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("row should be deleted, got %v", err)
	}
	if len(nx.removed) != 1 || nx.removed[0] != "edge-gone.com.conf" {
		t.Errorf("nginx.RemoveFile calls = %v", nx.removed)
	}
	if nx.tested.Load() != 1 {
		t.Errorf("TestConfig called %d times", nx.tested.Load())
	}
	if nx.reloaded.Load() != 1 {
		t.Errorf("Reload called %d times", nx.reloaded.Load())
	}
	cb.waitForDelete(t, "gone.com")
}

func TestDomain_RetryFailedCert(t *testing.T) {
	h, repo, _, _ := newDomainHandler(t)
	d, _ := repo.Create("retry.com")
	_ = repo.MarkCertFailed(d.ID, "previous error")

	rec := httptest.NewRecorder()
	h.RetryPOST(rec, reqWithID("POST", "/x", d.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	got, _ := repo.GetByID(d.ID)
	if got.Status != store.StatusPending {
		t.Errorf("status = %q", got.Status)
	}
	if got.FailCount != 0 {
		t.Errorf("fail_count = %d", got.FailCount)
	}
	if got.LastError != "" {
		t.Errorf("last_error = %q", got.LastError)
	}
}

func TestDomain_RetryNotFailed_Rejects(t *testing.T) {
	h, repo, _, _ := newDomainHandler(t)
	d, _ := repo.Create("noo.com") // status pending
	rec := httptest.NewRecorder()
	h.RetryPOST(rec, reqWithID("POST", "/x", d.ID))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d", rec.Code)
	}
}

func TestDomain_DeprecateNotFound(t *testing.T) {
	h, _, _, _ := newDomainHandler(t)
	rec := httptest.NewRecorder()
	h.DeprecatePOST(rec, reqWithID("POST", "/x", 9999))
	if rec.Code != http.StatusNotFound {
		t.Errorf("code = %d, want 404", rec.Code)
	}
}

func TestDomain_ListGET(t *testing.T) {
	h, repo, _, _ := newDomainHandler(t)
	_, _ = repo.Create("a.com")
	_, _ = repo.Create("b.com")
	rec := httptest.NewRecorder()
	h.ListGET(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("code = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "a.com") || !strings.Contains(body, "b.com") {
		t.Errorf("list missing hosts:\n%s", body)
	}
}
