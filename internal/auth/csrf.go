package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
)

const csrfCookie = "gs_csrf"

// secureCookiesFromConfig derives the cookie Secure flag without an *http.Request,
// from the configured BaseURL scheme. Used for cookies issued outside a
// request-derived TLS context (e.g. the CSRF token endpoint, logout clears).
func (s *Service) secureCookiesFromConfig() bool {
	return strings.HasPrefix(s.Cfg.BaseURL, "https://")
}

// IssueCSRF generates a CSRF token, sets it as a readable (non-httpOnly) cookie
// for the double-submit pattern, and returns it for the client to echo in a header.
func (s *Service) IssueCSRF(w http.ResponseWriter) string {
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
		Secure:   s.secureCookiesFromConfig(),
		SameSite: http.SameSiteLaxMode,
	})
	return token
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
