package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"edge-proxy/internal/store"
)

type fakeUpstreamNginx struct {
	calls []struct {
		filename string
		content  string
	}
	err error
}

func (f *fakeUpstreamNginx) WriteAndApply(filename string, content []byte) error {
	f.calls = append(f.calls, struct {
		filename string
		content  string
	}{filename, string(content)})
	return f.err
}

func newUpstreamHandler(t *testing.T) (*UpstreamHandler, *store.UpstreamRepo, *fakeUpstreamNginx) {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "edge.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	repo := store.NewUpstreamRepo(db)
	nx := &fakeUpstreamNginx{}
	return NewUpstreamHandler(repo, nx), repo, nx
}

func TestUpstream_Create_RewritesConf(t *testing.T) {
	h, repo, nx := newUpstreamHandler(t)

	form := url.Values{}
	form.Set("addr", "10.0.0.5:80")
	form.Set("weight", "2")

	rec := httptest.NewRecorder()
	h.CreatePOST(rec, postForm2("/upstreams", form))

	if rec.Code != http.StatusOK {
		t.Errorf("code = %d", rec.Code)
	}
	if len(nx.calls) != 1 {
		t.Fatalf("nginx applied %d times", len(nx.calls))
	}
	if nx.calls[0].filename != "edge-upstream.conf" {
		t.Errorf("filename = %q", nx.calls[0].filename)
	}
	if !strings.Contains(nx.calls[0].content, "server 10.0.0.5:80 weight=2;") {
		t.Errorf("content missing server line:\n%s", nx.calls[0].content)
	}
	list, _ := repo.List()
	if len(list) != 1 {
		t.Errorf("repo has %d rows", len(list))
	}
}

func TestUpstream_Create_InvalidAddr(t *testing.T) {
	h, _, _ := newUpstreamHandler(t)
	for _, bad := range []string{"", "no-port", "1.2.3.4", ":80", "10.0.0.5:abc", "10.0.0.5"} {
		form := url.Values{}
		form.Set("addr", bad)
		rec := httptest.NewRecorder()
		h.CreatePOST(rec, postForm2("/upstreams", form))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("addr %q should be rejected, got %d", bad, rec.Code)
		}
	}
}

func TestUpstream_Create_Duplicate(t *testing.T) {
	h, _, _ := newUpstreamHandler(t)
	form := url.Values{}
	form.Set("addr", "dup:80")
	h.CreatePOST(httptest.NewRecorder(), postForm2("/upstreams", form))
	rec := httptest.NewRecorder()
	h.CreatePOST(rec, postForm2("/upstreams", form))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d", rec.Code)
	}
}

func TestUpstream_Toggle_DropsFromConf(t *testing.T) {
	h, repo, nx := newUpstreamHandler(t)
	u, _ := repo.Create(store.UpstreamInput{Addr: "a:80"})
	_, _ = repo.Create(store.UpstreamInput{Addr: "b:80"})

	rec := httptest.NewRecorder()
	h.TogglePOST(rec, reqWithID("POST", "/x", u.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if len(nx.calls) != 1 {
		t.Fatalf("nginx applied %d times", len(nx.calls))
	}
	if strings.Contains(nx.calls[0].content, "server a:80") {
		t.Errorf("disabled upstream should be dropped from conf:\n%s", nx.calls[0].content)
	}
	if !strings.Contains(nx.calls[0].content, "server b:80") {
		t.Errorf("enabled upstream should be in conf:\n%s", nx.calls[0].content)
	}
}

func TestUpstream_Delete(t *testing.T) {
	h, repo, _ := newUpstreamHandler(t)
	a, _ := repo.Create(store.UpstreamInput{Addr: "del:80"})
	_, _ = repo.Create(store.UpstreamInput{Addr: "keep:80"})

	rec := httptest.NewRecorder()
	h.DeleteHTTP(rec, reqWithID("DELETE", "/x", a.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if _, err := repo.GetByID(a.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("row should be deleted, got %v", err)
	}
}

func TestUpstream_Delete_TolerantToEmptyPool(t *testing.T) {
	h, repo, _ := newUpstreamHandler(t)
	a, _ := repo.Create(store.UpstreamInput{Addr: "last:80"})

	rec := httptest.NewRecorder()
	h.DeleteHTTP(rec, reqWithID("DELETE", "/x", a.ID))
	// 200 even though refresh fails (empty pool); the row is already gone, alert via log.
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if _, err := repo.GetByID(a.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("row should still be deleted")
	}
}

func TestUpstream_ListGET(t *testing.T) {
	h, repo, _ := newUpstreamHandler(t)
	_, _ = repo.Create(store.UpstreamInput{Addr: "x:80"})

	rec := httptest.NewRecorder()
	h.ListGET(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("code = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "x:80") {
		t.Errorf("body missing addr:\n%s", rec.Body.String())
	}
}

func TestUpstream_ListGET_ResponsiveLayoutHooks(t *testing.T) {
	h, repo, _ := newUpstreamHandler(t)
	_, _ = repo.Create(store.UpstreamInput{Addr: "responsive:80"})

	rec := httptest.NewRecorder()
	h.ListGET(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "responsive:80") {
		t.Fatalf("body missing seeded addr:\n%s", body)
	}
	for _, want := range []string{
		`data-mobile-nav="admin"`,
		`data-admin-shell="responsive"`,
		`data-list-controls="upstreams"`,
		`data-list-table-scroll="upstreams"`,
		`class="table table-sm min-w-[880px]"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
}

func TestUpstream_NginxApplyFailureSurfaced(t *testing.T) {
	h, _, nx := newUpstreamHandler(t)
	nx.err = errors.New("nginx -t failed")

	form := url.Values{}
	form.Set("addr", "10.0.0.5:80")
	rec := httptest.NewRecorder()
	h.CreatePOST(rec, postForm2("/upstreams", form))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("code = %d, want 500", rec.Code)
	}
}

func TestUpstream_BadID(t *testing.T) {
	h, _, _ := newUpstreamHandler(t)
	// req with non-numeric id
	req := httptest.NewRequest("POST", "/x", nil)
	rec := httptest.NewRecorder()
	h.TogglePOST(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d", rec.Code)
	}
	_ = strconv.Itoa
}
