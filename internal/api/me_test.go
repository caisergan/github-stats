package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github-stats/internal/auth"
	"github-stats/internal/config"
	"github-stats/internal/crypto"
	"github-stats/internal/githubapi"
	"github-stats/internal/store"
	gosync "github-stats/internal/sync"
)

// testServerWithGitHub builds a test Server whose config points the GitHub REST
// API base at ghBaseURL, so handlers that validate against GitHub (e.g. the PAT
// settings handler) can be pointed at a fake httptest server.
func testServerWithGitHub(t *testing.T, ghBaseURL string) (*Server, *store.Store) {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cph, _ := crypto.NewCipher(make([]byte, 32))
	cfg := config.Config{
		SessionTTL:       time.Hour,
		BaseURL:          "http://localhost:8080",
		GitHubAPIBaseURL: ghBaseURL,
	}
	svc := auth.NewService(cfg, st, &auth.OAuthClient{}, cph)
	eng := gosync.NewEngine(st, func(repoID int64) (*githubapi.Client, error) {
		return githubapi.NewClient(githubapi.Options{
			Token:       "t",
			GraphQLURL:  "http://unused",
			RESTBaseURL: "http://unused",
			Store:       st,
			HTTP:        &http.Client{},
		}), nil
	}, gosync.Config{})
	return NewServer(cfg, st, svc, eng, cph), st
}

func testServer(t *testing.T) (*Server, *store.Store) {
	return testServerWithGitHub(t, "http://unused")
}

// withCSRF attaches a matching gs_csrf cookie + X-CSRF-Token header so a request
// passes the double-submit CSRF check enforced on unsafe API methods.
func withCSRF(req *http.Request) *http.Request {
	const tok = "test-csrf-token"
	req.AddCookie(&http.Cookie{Name: "gs_csrf", Value: tok})
	req.Header.Set("X-CSRF-Token", tok)
	return req
}

func TestMeUnauthorized(t *testing.T) {
	srv, _ := testServer(t)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestMeReturnsUser(t *testing.T) {
	srv, st := testServer(t)
	ctx := context.Background()
	uid, _ := st.UpsertUser(ctx, &store.User{GitHubID: 3, Login: "morpheus", AvatarURL: "http://a/m.png"})
	sess, _ := st.CreateSession(ctx, uid, time.Hour)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(&http.Cookie{Name: "gs_session", Value: sess.ID})
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["login"] != "morpheus" {
		t.Fatalf("login = %v", body["login"])
	}
}

func TestMeIncludesScopeAndPATFlag(t *testing.T) {
	srv, st := testServer(t)
	ctx := context.Background()
	uid, _ := st.UpsertUser(ctx, &store.User{GitHubID: 3, Login: "morpheus"})
	sess, _ := st.CreateSession(ctx, uid, time.Hour)
	st.UpsertCredential(ctx, &store.Credential{UserID: uid, Kind: "oauth", EncToken: "x", Scopes: "read:user"})

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(&http.Cookie{Name: "gs_session", Value: sess.ID})
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	var body map[string]any
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body["scopes"] != "read:user" {
		t.Fatalf("scopes = %v, want read:user", body["scopes"])
	}
	if body["has_pat"] != false {
		t.Fatalf("has_pat = %v, want false", body["has_pat"])
	}
}

func TestUnknownAPIRouteReturnsJSON404(t *testing.T) {
	srv, _ := testServer(t)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/does-not-exist", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("content-type = %q, want application/json", ct)
	}
}

func TestSPAFallbackServesIndex(t *testing.T) {
	srv, _ := testServer(t)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/owner/repo", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type = %q, want html", ct)
	}
}
