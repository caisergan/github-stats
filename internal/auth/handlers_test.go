package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github-stats/internal/store"
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

func TestLogoutRequiresPOSTAndCSRF(t *testing.T) {
	svc := testService(t)
	ctx := context.Background()
	uid, _ := svc.Store.UpsertUser(ctx, &store.User{GitHubID: 7, Login: "neo"})
	sess, _ := svc.Store.CreateSession(ctx, uid, time.Hour)

	// GET is no longer accepted (method not allowed by the router; handler guards too).
	getReq := httptest.NewRequest(http.MethodGet, "/auth/logout", nil)
	getReq.AddCookie(&http.Cookie{Name: sessionCookie, Value: sess.ID})
	getRec := httptest.NewRecorder()
	svc.Logout(getRec, getReq)
	if getRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET logout status = %d, want 405", getRec.Code)
	}

	// POST without CSRF is rejected; the session survives.
	noCSRF := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	noCSRF.AddCookie(&http.Cookie{Name: sessionCookie, Value: sess.ID})
	noRec := httptest.NewRecorder()
	svc.Logout(noRec, noCSRF)
	if noRec.Code != http.StatusForbidden {
		t.Fatalf("POST without CSRF status = %d, want 403", noRec.Code)
	}
	if _, err := svc.Store.GetSession(ctx, sess.ID); err != nil {
		t.Fatalf("session deleted without CSRF: %v", err)
	}

	// POST with matching CSRF clears the session.
	ok := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	ok.AddCookie(&http.Cookie{Name: sessionCookie, Value: sess.ID})
	ok.AddCookie(&http.Cookie{Name: csrfCookie, Value: "tok"})
	ok.Header.Set("X-CSRF-Token", "tok")
	okRec := httptest.NewRecorder()
	svc.Logout(okRec, ok)
	if okRec.Code != http.StatusNoContent {
		t.Fatalf("POST logout status = %d, want 204", okRec.Code)
	}
	if _, err := svc.Store.GetSession(ctx, sess.ID); err != store.ErrNotFound {
		t.Fatalf("session not cleared: %v", err)
	}
}

func TestLogoutEverywhere(t *testing.T) {
	svc := testService(t)
	ctx := context.Background()
	uid, _ := svc.Store.UpsertUser(ctx, &store.User{GitHubID: 7, Login: "neo"})
	s1, _ := svc.Store.CreateSession(ctx, uid, time.Hour)
	s2, _ := svc.Store.CreateSession(ctx, uid, time.Hour)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout/all", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: s1.ID})
	req.AddCookie(&http.Cookie{Name: csrfCookie, Value: "tok"})
	req.Header.Set("X-CSRF-Token", "tok")
	rec := httptest.NewRecorder()
	svc.LogoutEverywhere(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	for _, id := range []string{s1.ID, s2.ID} {
		if _, err := svc.Store.GetSession(ctx, id); err != store.ErrNotFound {
			t.Fatalf("session %s survived logout-all: %v", id, err)
		}
	}
}
