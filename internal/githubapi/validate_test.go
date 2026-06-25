package githubapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidatePATSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			t.Errorf("path = %s, want /user", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer pat_xyz" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("X-OAuth-Scopes", "repo, read:org")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":1,"login":"octocat"}`))
	}))
	defer srv.Close()

	info, err := ValidatePAT(context.Background(), srv.Client(), srv.URL, "pat_xyz")
	if err != nil {
		t.Fatal(err)
	}
	if info.Login != "octocat" || info.Scopes != "repo, read:org" {
		t.Fatalf("info = %+v", info)
	}
}

func TestValidatePATRejectsBadToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	if _, err := ValidatePAT(context.Background(), srv.Client(), srv.URL, "bad"); err == nil {
		t.Fatal("expected error for 401 token")
	}
}
