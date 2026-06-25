package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIssueAndVerifyCSRF(t *testing.T) {
	svc := testService(t)
	rec := httptest.NewRecorder()
	token := svc.IssueCSRF(rec)
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
