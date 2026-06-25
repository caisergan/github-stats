package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github-stats/internal/store"
)

func TestSavePATValidatesAndStores(t *testing.T) {
	// Fake GitHub /user that the settings handler validates against.
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good_pat" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":1,"login":"octocat"}`))
	}))
	defer gh.Close()

	srv, st := testServerWithGitHub(t, gh.URL)
	uid, sid := loginSession(t, st, "neo", 7)

	// Valid PAT → 200, stored encrypted.
	body, _ := json.Marshal(map[string]string{"token": "good_pat"})
	req := httptest.NewRequest(http.MethodPut, "/api/settings/pat", bytes.NewReader(body))
	req.AddCookie(&http.Cookie{Name: "gs_session", Value: sid})
	withCSRF(req)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("save status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	cred, err := st.GetCredential(context.Background(), uid, "pat")
	if err != nil {
		t.Fatalf("pat not stored: %v", err)
	}
	dec, _ := srv.cipher.Decrypt(cred.EncToken)
	if string(dec) != "good_pat" {
		t.Fatalf("stored token = %q, want good_pat", dec)
	}

	// Status reflects the stored PAT.
	sreq := httptest.NewRequest(http.MethodGet, "/api/settings/pat", nil)
	sreq.AddCookie(&http.Cookie{Name: "gs_session", Value: sid})
	srec := httptest.NewRecorder()
	srv.Router().ServeHTTP(srec, sreq)
	var status struct {
		HasPAT bool   `json:"has_pat"`
		Login  string `json:"login"`
	}
	json.Unmarshal(srec.Body.Bytes(), &status)
	if !status.HasPAT || status.Login != "octocat" {
		t.Fatalf("status = %+v", status)
	}
}

func TestSavePATRejectsInvalid(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer gh.Close()
	srv, st := testServerWithGitHub(t, gh.URL)
	_, sid := loginSession(t, st, "neo", 7)

	body, _ := json.Marshal(map[string]string{"token": "bad"})
	req := httptest.NewRequest(http.MethodPut, "/api/settings/pat", bytes.NewReader(body))
	req.AddCookie(&http.Cookie{Name: "gs_session", Value: sid})
	withCSRF(req)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid PAT status = %d, want 400", rec.Code)
	}
}

func TestDeletePAT(t *testing.T) {
	srv, st := testServer(t)
	uid, sid := loginSession(t, st, "neo", 7)
	enc, _ := srv.cipher.Encrypt([]byte("tok"))
	st.UpsertCredential(context.Background(), &store.Credential{UserID: uid, Kind: "pat", EncToken: enc})

	req := httptest.NewRequest(http.MethodDelete, "/api/settings/pat", nil)
	req.AddCookie(&http.Cookie{Name: "gs_session", Value: sid})
	withCSRF(req)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", rec.Code)
	}
	if _, err := st.GetCredential(context.Background(), uid, "pat"); err != store.ErrNotFound {
		t.Fatalf("pat still present: %v", err)
	}
}
