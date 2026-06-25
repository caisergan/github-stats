package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
)

const csrfCookie = "gs_csrf"

// IssueCSRF generates a CSRF token, sets it as a readable (non-httpOnly) cookie
// for the double-submit pattern, and returns it for the client to echo in a header.
// The Secure flag is derived from the request (TLS / X-Forwarded-Proto / BaseURL)
// so the cookie behaves identically to the session/state cookies behind a
// TLS-terminating proxy.
func (s *Service) IssueCSRF(w http.ResponseWriter, r *http.Request) string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	token := hex.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: false, // must be readable by JS to echo into the header
		Secure:   s.secureCookies(r),
		SameSite: http.SameSiteLaxMode,
	})
	return token
}

// RequireCSRF is middleware that enforces the double-submit CSRF check on unsafe
// (state-changing) HTTP methods. Safe methods (GET, HEAD, OPTIONS) are exempt so
// reads, SSE streams, and export/token endpoints keep working without a token.
func (s *Service) RequireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			// Safe methods: no CSRF token required.
		default:
			if !s.VerifyCSRF(r) {
				http.Error(w, "csrf", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// VerifyCSRF checks the double-submit token: the gs_csrf cookie must match the
// X-CSRF-Token header (constant-time).
func (s *Service) VerifyCSRF(r *http.Request) bool {
	c, err := r.Cookie(csrfCookie)
	if err != nil || c.Value == "" {
		return false
	}
	header := r.Header.Get("X-CSRF-Token")
	if header == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(header)) == 1
}
