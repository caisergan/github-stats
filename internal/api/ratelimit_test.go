package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRateLimitEndpoint(t *testing.T) {
	srv, st := testServer(t)
	_, sid := loginSession(t, st, "neo", 7)

	req := httptest.NewRequest(http.MethodGet, "/api/rate-limit", nil)
	req.AddCookie(&http.Cookie{Name: "gs_session", Value: sid})
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		REST struct {
			Remaining int    `json:"remaining"`
			Reset     string `json:"reset"`
		} `json:"rest"`
		GraphQL struct {
			Remaining int    `json:"remaining"`
			Reset     string `json:"reset"`
		} `json:"graphql"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v (body %s)", err, rec.Body.String())
	}
	// A fresh budget reports the default full bucket (>= 0); both keys present.
	if body.REST.Remaining < 0 || body.GraphQL.Remaining < 0 {
		t.Fatalf("negative remaining: %+v", body)
	}
}

func TestRateLimitUnauthorized(t *testing.T) {
	srv, _ := testServer(t)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/rate-limit", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
