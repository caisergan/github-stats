package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github-stats/internal/config"
	"github-stats/internal/crypto"
	"github-stats/internal/store"
)

func testService(t *testing.T) *Service {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	key := make([]byte, 32)
	cph, _ := crypto.NewCipher(key)
	cfg := config.Config{SessionTTL: time.Hour, BaseURL: "http://localhost:8080"}
	return NewService(cfg, st, &OAuthClient{}, cph)
}

func TestRequireUserRejectsNoCookie(t *testing.T) {
	svc := testService(t)
	h := svc.RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not run without session")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRequireUserLoadsUser(t *testing.T) {
	svc := testService(t)
	ctx := context.Background()
	uid, _ := svc.Store.UpsertUser(ctx, &store.User{GitHubID: 7, Login: "neo"})
	sess, _ := svc.Store.CreateSession(ctx, uid, time.Hour)

	var seen *store.User
	h := svc.RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := UserFromContext(r.Context())
		if !ok {
			t.Fatal("user not in context")
		}
		seen = u
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: sess.ID})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if seen == nil || seen.Login != "neo" {
		t.Fatalf("loaded user = %+v", seen)
	}
}
