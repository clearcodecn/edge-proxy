package handler

import (
	"net/http"

	"golang.org/x/crypto/bcrypt"

	mw "edge-proxy/internal/web/middleware"
)

const loginFallbackHTML = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>edge-proxy login</title></head>
<body>
<form method="post" action="/login">
  <input name="username" placeholder="username" autofocus required>
  <input name="password" type="password" placeholder="password" required>
  {{if .Error}}<p style="color:red">{{.Error}}</p>{{end}}
  <button type="submit">Sign in</button>
</form>
</body></html>`

// LoginRenderer renders the login page. Group 13 will replace this with the
// embedded htmx template; for now a minimal inline HTML works for tests.
type LoginRenderer interface {
	Render(w http.ResponseWriter, errMsg string)
}

// inlineRenderer is the default fallback when no template engine is wired yet.
type inlineRenderer struct{}

func (inlineRenderer) Render(w http.ResponseWriter, errMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!DOCTYPE html><html><body><form method="post" action="/login">` +
		`<input name="username" autofocus required>` +
		`<input name="password" type="password" required>` +
		errMsgHTML(errMsg) +
		`<button type="submit">Sign in</button></form></body></html>`))
}

func errMsgHTML(s string) string {
	if s == "" {
		return ""
	}
	return `<p style="color:red">` + s + `</p>`
}

type LoginHandler struct {
	Username     string
	PasswordHash string
	Sessions     *mw.SessionStore
	Renderer     LoginRenderer
}

func NewLoginHandler(username, passwordHash string, sessions *mw.SessionStore) *LoginHandler {
	return &LoginHandler{
		Username:     username,
		PasswordHash: passwordHash,
		Sessions:     sessions,
		Renderer:     inlineRenderer{},
	}
}

func (h *LoginHandler) GET(w http.ResponseWriter, _ *http.Request) {
	h.Renderer.Render(w, "")
}

func (h *LoginHandler) POST(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	username := r.PostForm.Get("username")
	password := r.PostForm.Get("password")

	if username != h.Username {
		h.Renderer.Render(w, "用户名或密码错误")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(h.PasswordHash), []byte(password)); err != nil {
		h.Renderer.Render(w, "用户名或密码错误")
		return
	}

	token, err := h.Sessions.Issue()
	if err != nil {
		http.Error(w, "session issue: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     mw.SessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(h.Sessions.TTL().Seconds()),
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *LoginHandler) LogoutPOST(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(mw.SessionCookie); err == nil {
		h.Sessions.Revoke(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     mw.SessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/login", http.StatusFound)
}
