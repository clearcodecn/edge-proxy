package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

const (
	SessionCookie     = "edge_proxy_session"
	DefaultSessionTTL = 24 * time.Hour
)

// SessionStore is an in-memory token store. Sessions vanish on process restart,
// which is by design (G1 single-admin scope; no persistence needed).
type SessionStore struct {
	mu     sync.Mutex
	tokens map[string]time.Time
	ttl    time.Duration
}

func NewSessionStore(ttl time.Duration) *SessionStore {
	if ttl == 0 {
		ttl = DefaultSessionTTL
	}
	return &SessionStore{
		tokens: make(map[string]time.Time),
		ttl:    ttl,
	}
}

// Issue creates a fresh 32-byte hex-encoded token and stores it.
func (s *SessionStore) Issue() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := hex.EncodeToString(buf)
	s.mu.Lock()
	s.tokens[token] = time.Now().Add(s.ttl)
	s.mu.Unlock()
	return token, nil
}

// Validate returns true when token exists and is not expired.
func (s *SessionStore) Validate(token string) bool {
	if token == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	expireAt, ok := s.tokens[token]
	if !ok {
		return false
	}
	if time.Now().After(expireAt) {
		delete(s.tokens, token)
		return false
	}
	return true
}

func (s *SessionStore) Revoke(token string) {
	s.mu.Lock()
	delete(s.tokens, token)
	s.mu.Unlock()
}

// TTL is the configured session lifetime.
func (s *SessionStore) TTL() time.Duration {
	return s.ttl
}

// RequireAuth returns a middleware that redirects to loginPath when no valid
// session cookie is presented.
func RequireAuth(s *SessionStore, loginPath string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(SessionCookie)
			if err != nil || !s.Validate(cookie.Value) {
				http.Redirect(w, r, loginPath, http.StatusFound)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
