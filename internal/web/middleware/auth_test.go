package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionStore_IssueAndValidate(t *testing.T) {
	s := NewSessionStore(time.Hour)
	tok, err := s.Issue()
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if len(tok) != 64 {
		t.Errorf("token len = %d, want 64 hex chars", len(tok))
	}
	if !s.Validate(tok) {
		t.Error("issued token should validate")
	}
	if s.Validate("nope") {
		t.Error("garbage token should not validate")
	}
}

func TestSessionStore_Revoke(t *testing.T) {
	s := NewSessionStore(time.Hour)
	tok, _ := s.Issue()
	s.Revoke(tok)
	if s.Validate(tok) {
		t.Error("revoked token should not validate")
	}
}

func TestSessionStore_Expiry(t *testing.T) {
	s := NewSessionStore(1 * time.Millisecond)
	tok, _ := s.Issue()
	time.Sleep(5 * time.Millisecond)
	if s.Validate(tok) {
		t.Error("expired token should not validate")
	}
}

func TestSessionStore_IssueUnique(t *testing.T) {
	s := NewSessionStore(time.Hour)
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		tok, _ := s.Issue()
		if seen[tok] {
			t.Fatalf("duplicate token at iter %d", i)
		}
		seen[tok] = true
	}
}

func TestSessionStore_RestartSimulation(t *testing.T) {
	s := NewSessionStore(time.Hour)
	tok, _ := s.Issue()

	// Simulate restart: new instance forgets all sessions.
	fresh := NewSessionStore(time.Hour)
	if fresh.Validate(tok) {
		t.Error("fresh store should not validate token from previous store")
	}
}

// ── RequireAuth middleware ────────────────────────────────────────────────

func protectedHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("protected payload"))
	})
}

func TestRequireAuth_NoCookieRedirects(t *testing.T) {
	s := NewSessionStore(time.Hour)
	h := RequireAuth(s, "/login")(protectedHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Errorf("status = %d, want 302", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/login" {
		t.Errorf("Location = %q", got)
	}
}

func TestRequireAuth_InvalidCookieRedirects(t *testing.T) {
	s := NewSessionStore(time.Hour)
	h := RequireAuth(s, "/login")(protectedHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: "garbage"})
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Errorf("status = %d, want 302", rec.Code)
	}
}

func TestRequireAuth_ValidCookiePasses(t *testing.T) {
	s := NewSessionStore(time.Hour)
	tok, _ := s.Issue()
	h := RequireAuth(s, "/login")(protectedHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: tok})
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "protected payload" {
		t.Errorf("body = %q", rec.Body.String())
	}
}
