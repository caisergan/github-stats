package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIssueAndVerifyCSRF(t *testing.T) {
	svc := testService(t)
	rec := httptest.NewRecorder()
	token := svc.IssueCSRF(rec, httptest.NewRequest(http.MethodGet, "/api/csrf", nil))
	if token == "" {
		t.Fatal("expected a token")
	}
	var cookieVal string
	for _, c := range rec.Result().Cookies() {
		if c.Name == csrfCookie {
			cookieVal = c.Value
		}
	}
	if cookieVal != token {
		t.Fatalf("cookie %q != token %q", cookieVal, token)
	}

	// Matching cookie + header verifies.
	good := httptest.NewRequest(http.MethodPost, "/x", nil)
	good.AddCookie(&http.Cookie{Name: csrfCookie, Value: token})
	good.Header.Set("X-CSRF-Token", token)
	if !svc.VerifyCSRF(good) {
		t.Fatal("expected verify to pass")
	}

	// Mismatched header fails.
	bad := httptest.NewRequest(http.MethodPost, "/x", nil)
	bad.AddCookie(&http.Cookie{Name: csrfCookie, Value: token})
	bad.Header.Set("X-CSRF-Token", "different")
	if svc.VerifyCSRF(bad) {
		t.Fatal("expected verify to fail on mismatch")
	}

	// Missing cookie fails.
	none := httptest.NewRequest(http.MethodPost, "/x", nil)
	none.Header.Set("X-CSRF-Token", token)
	if svc.VerifyCSRF(none) {
		t.Fatal("expected verify to fail without cookie")
	}
}

func TestRequireCSRFExemptsSafeMethodsAndGuardsUnsafe(t *testing.T) {
	svc := testService(t)
	var called bool
	h := svc.RequireCSRF(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	// Safe methods pass through without a token.
	for _, m := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		called = false
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(m, "/api/me", nil))
		if !called || rec.Code != http.StatusOK {
			t.Fatalf("%s: called=%v status=%d, want passthrough", m, called, rec.Code)
		}
	}

	// Unsafe methods without a valid token are rejected with 403.
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		called = false
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(m, "/api/collections", nil))
		if called {
			t.Fatalf("%s: handler ran despite missing CSRF token", m)
		}
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: status = %d, want 403", m, rec.Code)
		}
	}

	// Unsafe method WITH a matching token passes.
	const tok = "tok123"
	called = false
	req := httptest.NewRequest(http.MethodPost, "/api/collections", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookie, Value: tok})
	req.Header.Set("X-CSRF-Token", tok)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !called || rec.Code != http.StatusOK {
		t.Fatalf("valid token: called=%v status=%d, want passthrough", called, rec.Code)
	}
}

func TestIssueCSRFSecureFlagFromForwardedProto(t *testing.T) {
	svc := testService(t) // BaseURL is http://localhost, so config alone => not secure

	// Plain request: not secure.
	rec := httptest.NewRecorder()
	svc.IssueCSRF(rec, httptest.NewRequest(http.MethodGet, "/api/csrf", nil))
	if csrfCookieSecure(rec) {
		t.Fatal("expected Secure=false for plain http request")
	}

	// Behind a TLS-terminating proxy (X-Forwarded-Proto: https): secure.
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/csrf", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	svc.IssueCSRF(rec, req)
	if !csrfCookieSecure(rec) {
		t.Fatal("expected Secure=true behind X-Forwarded-Proto: https")
	}
}

func csrfCookieSecure(rec *httptest.ResponseRecorder) bool {
	for _, c := range rec.Result().Cookies() {
		if c.Name == csrfCookie {
			return c.Secure
		}
	}
	return false
}
