package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginRedirectsWithStateCookie(t *testing.T) {
	svc := testService(t)
	svc.OAuth = &OAuthClient{ClientID: "cid", RedirectURL: "http://app/cb", OAuthBaseURL: "https://github.com"}

	rec := httptest.NewRecorder()
	svc.Login(rec, httptest.NewRequest(http.MethodGet, "/auth/github", nil))

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "login/oauth/authorize") || !strings.Contains(loc, "state=") {
		t.Fatalf("bad redirect: %s", loc)
	}
	var stateSet bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == stateCookie && c.Value != "" {
			stateSet = true
		}
	}
	if !stateSet {
		t.Fatal("state cookie not set")
	}
}

func TestLoginSecureCookieFromForwardedProto(t *testing.T) {
	svc := testService(t)
	svc.OAuth = &OAuthClient{ClientID: "cid", RedirectURL: "http://app/cb", OAuthBaseURL: "https://github.com"}

	req := httptest.NewRequest(http.MethodGet, "/auth/github", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	svc.Login(rec, req)

	var found bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == stateCookie {
			found = true
			if !c.Secure {
				t.Fatalf("state cookie Secure = false, want true for X-Forwarded-Proto: https")
			}
		}
	}
	if !found {
		t.Fatal("state cookie not set")
	}
}

func TestCallbackCreatesUserAndSession(t *testing.T) {
	svc := testService(t)

	// Fake GitHub: token endpoint + user endpoint.
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/access_token"):
			w.Write([]byte(`{"access_token":"gho_tok","scope":"read:user"}`))
		case strings.HasSuffix(r.URL.Path, "/user"):
			w.Write([]byte(`{"id":555,"login":"trinity","avatar_url":"http://a/t.png"}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer gh.Close()
	svc.OAuth = &OAuthClient{
		ClientID: "cid", ClientSecret: "sec", RedirectURL: "http://app/cb",
		OAuthBaseURL: gh.URL, APIBaseURL: gh.URL, HTTP: gh.Client(),
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/github/callback?code=c&state=abc", nil)
	req.AddCookie(&http.Cookie{Name: stateCookie, Value: "abc"})
	rec := httptest.NewRecorder()
	svc.Callback(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302 (body: %s)", rec.Code, rec.Body.String())
	}
	var sessionSet bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie && c.Value != "" {
			sessionSet = true
		}
	}
	if !sessionSet {
		t.Fatal("session cookie not set")
	}
	// User persisted with encrypted credential.
	u, err := svc.Store.GetUserByID(req.Context(), 1)
	if err != nil || u.Login != "trinity" {
		t.Fatalf("user not persisted: %+v err=%v", u, err)
	}
	cred, err := svc.Store.GetCredential(req.Context(), u.ID, "oauth")
	if err != nil {
		t.Fatal(err)
	}
	dec, err := svc.Cipher.Decrypt(cred.EncToken)
	if err != nil || string(dec) != "gho_tok" {
		t.Fatalf("token not stored encrypted: %q err=%v", dec, err)
	}
}

func TestCallbackRejectsStateMismatch(t *testing.T) {
	svc := testService(t)
	req := httptest.NewRequest(http.MethodGet, "/auth/github/callback?code=c&state=evil", nil)
	req.AddCookie(&http.Cookie{Name: stateCookie, Value: "good"})
	rec := httptest.NewRecorder()
	svc.Callback(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
