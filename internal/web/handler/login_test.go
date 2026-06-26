package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	mw "edge-proxy/internal/web/middleware"
)

func mustBcrypt(t *testing.T, plain string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	return string(h)
}

func newLogin(t *testing.T) (*LoginHandler, *mw.SessionStore) {
	t.Helper()
	sessions := mw.NewSessionStore(time.Hour)
	h := NewLoginHandler("admin", mustBcrypt(t, "secret"), sessions)
	return h, sessions
}

func postForm(form url.Values) *http.Request {
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func TestLogin_GET_RendersForm(t *testing.T) {
	h, _ := newLogin(t)
	rec := httptest.NewRecorder()
	h.GET(rec, httptest.NewRequest("GET", "/login", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("code = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<form") {
		t.Errorf("body should contain form, got: %s", rec.Body.String())
	}
}

func TestLogin_POST_Success_SetsCookieAndRedirects(t *testing.T) {
	h, sessions := newLogin(t)
	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "secret")

	rec := httptest.NewRecorder()
	h.POST(rec, postForm(form))

	if rec.Code != http.StatusFound {
		t.Fatalf("code = %d, want 302", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/" {
		t.Errorf("Location = %q", got)
	}
	cookies := rec.Result().Cookies()
	var session *http.Cookie
	for _, c := range cookies {
		if c.Name == mw.SessionCookie {
			session = c
			break
		}
	}
	if session == nil {
		t.Fatal("session cookie not set")
	}
	if !session.HttpOnly {
		t.Error("cookie should be HttpOnly")
	}
	if session.SameSite != http.SameSiteStrictMode {
		t.Errorf("SameSite = %v, want Strict", session.SameSite)
	}
	if !sessions.Validate(session.Value) {
		t.Error("session token should be valid in store")
	}
}

func TestLogin_POST_BadPassword(t *testing.T) {
	h, sessions := newLogin(t)
	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "wrong")

	rec := httptest.NewRecorder()
	h.POST(rec, postForm(form))

	if rec.Code != http.StatusOK {
		t.Errorf("code = %d, want 200 (re-render with error)", rec.Code)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == mw.SessionCookie {
			t.Error("session cookie should NOT be set on bad password")
		}
	}
	if !strings.Contains(rec.Body.String(), "用户名或密码错误") {
		t.Error("error message should be rendered")
	}
	_ = sessions
}

func TestLogin_POST_BadUsername(t *testing.T) {
	h, _ := newLogin(t)
	form := url.Values{}
	form.Set("username", "wrong")
	form.Set("password", "secret")

	rec := httptest.NewRecorder()
	h.POST(rec, postForm(form))

	if rec.Code != http.StatusOK {
		t.Errorf("code = %d", rec.Code)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == mw.SessionCookie {
			t.Error("cookie should NOT be set for unknown user")
		}
	}
}

func TestLogin_LogoutRevokesAndClearsCookie(t *testing.T) {
	h, sessions := newLogin(t)
	tok, _ := sessions.Issue()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/logout", nil)
	req.AddCookie(&http.Cookie{Name: mw.SessionCookie, Value: tok})
	h.LogoutPOST(rec, req)

	if rec.Code != http.StatusFound {
		t.Errorf("code = %d, want 302", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/login" {
		t.Errorf("Location = %q", got)
	}
	if sessions.Validate(tok) {
		t.Error("token should be revoked after logout")
	}
	// Cookie should be cleared (MaxAge negative).
	for _, c := range rec.Result().Cookies() {
		if c.Name == mw.SessionCookie {
			if c.MaxAge >= 0 {
				t.Errorf("cookie should be cleared, MaxAge = %d", c.MaxAge)
			}
		}
	}
}
