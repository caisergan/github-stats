package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github-stats/internal/store"
)

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// authedRepo seeds a tracked repo for the session user and returns its id.
func authedRepo(t *testing.T, st *store.Store, userID int64, fullName string, ghID int64) int64 {
	t.Helper()
	rid, err := st.UpsertRepo(context.Background(), &store.Repo{
		GitHubID: ghID, FullName: fullName, DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.TrackRepo(context.Background(), userID, rid); err != nil {
		t.Fatal(err)
	}
	return rid
}

// loginSession seeds a user + session and returns (userID, sessionID).
func loginSession(t *testing.T, st *store.Store, login string, ghID int64) (int64, string) {
	t.Helper()
	ctx := context.Background()
	uid, err := st.UpsertUser(ctx, &store.User{GitHubID: ghID, Login: login})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(ctx, uid, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return uid, sess.ID
}

func TestCollectionsCreateListAndGroup(t *testing.T) {
	srv, st := testServer(t)
	uid, sid := loginSession(t, st, "neo", 7)
	rid := authedRepo(t, st, uid, "octo/a", 10)

	// Create a collection.
	body, _ := json.Marshal(map[string]string{"name": "Backend"})
	req := httptest.NewRequest(http.MethodPost, "/api/collections", bytes.NewReader(body))
	req.AddCookie(&http.Cookie{Name: "gs_session", Value: sid})
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201 (body %s)", rec.Code, rec.Body.String())
	}
	var created struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	json.Unmarshal(rec.Body.Bytes(), &created)
	if created.ID == 0 || created.Name != "Backend" {
		t.Fatalf("created = %+v", created)
	}

	// Add the repo to the collection.
	addReq := httptest.NewRequest(http.MethodPost,
		"/api/collections/"+itoa(created.ID)+"/repos/"+itoa(rid), nil)
	addReq.AddCookie(&http.Cookie{Name: "gs_session", Value: sid})
	addRec := httptest.NewRecorder()
	srv.Router().ServeHTTP(addRec, addReq)
	if addRec.Code != http.StatusNoContent {
		t.Fatalf("add-repo status = %d, want 204", addRec.Code)
	}

	// List collections returns the collection with its repo ids.
	listReq := httptest.NewRequest(http.MethodGet, "/api/collections", nil)
	listReq.AddCookie(&http.Cookie{Name: "gs_session", Value: sid})
	listRec := httptest.NewRecorder()
	srv.Router().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", listRec.Code)
	}
	var got []struct {
		ID      int64   `json:"id"`
		Name    string  `json:"name"`
		RepoIDs []int64 `json:"repo_ids"`
	}
	json.Unmarshal(listRec.Body.Bytes(), &got)
	if len(got) != 1 || len(got[0].RepoIDs) != 1 || got[0].RepoIDs[0] != rid {
		t.Fatalf("list = %+v", got)
	}
}

func TestCollectionPatchDelete(t *testing.T) {
	srv, st := testServer(t)
	uid, sid := loginSession(t, st, "neo", 7)
	cid, _ := st.CreateCollection(context.Background(), uid, "Old")

	// PATCH rename.
	body, _ := json.Marshal(map[string]string{"name": "New"})
	preq := httptest.NewRequest(http.MethodPatch, "/api/collections/"+itoa(cid), bytes.NewReader(body))
	preq.AddCookie(&http.Cookie{Name: "gs_session", Value: sid})
	prec := httptest.NewRecorder()
	srv.Router().ServeHTTP(prec, preq)
	if prec.Code != http.StatusOK {
		t.Fatalf("patch status = %d, want 200", prec.Code)
	}

	// DELETE.
	dreq := httptest.NewRequest(http.MethodDelete, "/api/collections/"+itoa(cid), nil)
	dreq.AddCookie(&http.Cookie{Name: "gs_session", Value: sid})
	drec := httptest.NewRecorder()
	srv.Router().ServeHTTP(drec, dreq)
	if drec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", drec.Code)
	}
}

func TestCollectionForbiddenForOtherUser(t *testing.T) {
	srv, st := testServer(t)
	owner, _ := loginSession(t, st, "owner", 1)
	_, otherSid := loginSession(t, st, "other", 2)
	cid, _ := st.CreateCollection(context.Background(), owner, "Mine")

	// Other user PATCH → 404 (ownership-checked, no leakage).
	body, _ := json.Marshal(map[string]string{"name": "Hijacked"})
	req := httptest.NewRequest(http.MethodPatch, "/api/collections/"+itoa(cid), bytes.NewReader(body))
	req.AddCookie(&http.Cookie{Name: "gs_session", Value: otherSid})
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign patch status = %d, want 404", rec.Code)
	}
}

func TestCollectionExport(t *testing.T) {
	srv, st := testServer(t)
	uid, sid := loginSession(t, st, "neo", 7)
	cid, _ := st.CreateCollection(context.Background(), uid, "Backend")
	rid := authedRepo(t, st, uid, "octo/svc", 10)
	if err := st.AddRepoToCollection(context.Background(), uid, cid, rid); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/collections/"+itoa(cid)+"/export", nil)
	req.AddCookie(&http.Cookie{Name: "gs_session", Value: sid})
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("export status = %d, want 200", rec.Code)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Fatalf("Content-Disposition = %q, want attachment", cd)
	}
	var body struct {
		Name  string   `json:"name"`
		Repos []string `json:"repos"`
	}
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Name != "Backend" || len(body.Repos) != 1 || body.Repos[0] != "octo/svc" {
		t.Fatalf("export body = %+v", body)
	}
}

func TestImportManifestParse(t *testing.T) {
	srv, st := testServer(t)
	_, sid := loginSession(t, st, "neo", 7)

	req := httptest.NewRequest(http.MethodPost, "/api/import?kind=package_json",
		bytes.NewReader([]byte(`{"dependencies":{"@acme/x":"1.0.0","react":"18"}}`)))
	req.AddCookie(&http.Cookie{Name: "gs_session", Value: sid})
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("import status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var res ImportResult
	json.Unmarshal(rec.Body.Bytes(), &res)
	if len(res.Resolved) != 1 || res.Resolved[0] != "acme/x" {
		t.Fatalf("resolved = %v", res.Resolved)
	}
	if len(res.Unresolved) != 1 || res.Unresolved[0] != "react" {
		t.Fatalf("unresolved = %v", res.Unresolved)
	}
}
