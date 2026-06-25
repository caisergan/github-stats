package api

import (
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github-stats/internal/auth"
	"github-stats/internal/store"
)

type collectionJSON struct {
	ID      int64   `json:"id"`
	Name    string  `json:"name"`
	RepoIDs []int64 `json:"repo_ids"`
}

// listCollections handles GET /api/collections — the caller's collections with member repo ids.
func (s *Server) listCollections(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	cols, err := s.store.ListCollections(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "list failed", http.StatusInternalServerError)
		return
	}
	// One join query for all (collection_id -> repo_ids) instead of a per-collection
	// ListCollectionRepos call (which avoids the 1+N query pattern).
	repoIDs, err := s.store.ListUserCollectionRepoIDs(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "list failed", http.StatusInternalServerError)
		return
	}
	out := make([]collectionJSON, 0, len(cols))
	for _, c := range cols {
		ids := repoIDs[c.ID]
		if ids == nil {
			ids = []int64{}
		}
		out = append(out, collectionJSON{ID: c.ID, Name: c.Name, RepoIDs: ids})
	}
	writeJSON(w, http.StatusOK, out)
}

// createCollection handles POST /api/collections.
func (s *Server) createCollection(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	id, err := s.store.CreateCollection(r.Context(), u.ID, body.Name)
	if err != nil {
		http.Error(w, "create failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, collectionJSON{ID: id, Name: body.Name, RepoIDs: []int64{}})
}

// patchCollection handles PATCH /api/collections/{id} (rename).
func (s *Server) patchCollection(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	cid, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if err := s.store.RenameCollection(r.Context(), u.ID, cid, body.Name); err != nil {
		if err == store.ErrNotFound {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "rename failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, collectionJSON{ID: cid, Name: body.Name})
}

// deleteCollection handles DELETE /api/collections/{id}.
func (s *Server) deleteCollection(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	cid, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteCollection(r.Context(), u.ID, cid); err != nil {
		if err == store.ErrNotFound {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "delete failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// addCollectionRepo handles POST /api/collections/{id}/repos/{repoId}.
func (s *Server) addCollectionRepo(w http.ResponseWriter, r *http.Request) {
	s.mutateCollectionRepo(w, r, true)
}

// removeCollectionRepo handles DELETE /api/collections/{id}/repos/{repoId}.
func (s *Server) removeCollectionRepo(w http.ResponseWriter, r *http.Request) {
	s.mutateCollectionRepo(w, r, false)
}

func (s *Server) mutateCollectionRepo(w http.ResponseWriter, r *http.Request, add bool) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	cid, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad collection id", http.StatusBadRequest)
		return
	}
	rid, err := strconv.ParseInt(chi.URLParam(r, "repoId"), 10, 64)
	if err != nil {
		http.Error(w, "bad repo id", http.StatusBadRequest)
		return
	}
	// The repo must be tracked by the caller (no orphan membership rows).
	tracked, err := s.store.IsTracked(r.Context(), u.ID, rid)
	if err != nil {
		http.Error(w, "lookup failed", http.StatusInternalServerError)
		return
	}
	if !tracked {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if add {
		err = s.store.AddRepoToCollection(r.Context(), u.ID, cid, rid)
	} else {
		err = s.store.RemoveRepoFromCollection(r.Context(), u.ID, cid, rid)
	}
	if err != nil {
		if err == store.ErrNotFound {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "update failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// exportCollection handles GET /api/collections/{id}/export — a downloadable JSON file.
func (s *Server) exportCollection(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	cid, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	cols, err := s.store.ListCollections(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "lookup failed", http.StatusInternalServerError)
		return
	}
	var name string
	found := false
	for _, c := range cols {
		if c.ID == cid {
			name, found = c.Name, true
			break
		}
	}
	if !found {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	repos, err := s.store.ListCollectionRepos(r.Context(), u.ID, cid)
	if err != nil {
		http.Error(w, "lookup failed", http.StatusInternalServerError)
		return
	}
	names := make([]string, 0, len(repos))
	for _, rp := range repos {
		names = append(names, rp.FullName)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition",
		`attachment; filename="`+sanitizeFilename(name)+`.collection.json"`)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"name":  name,
		"repos": names,
	})
}

// importManifest handles POST /api/import?kind=package_json|requirements_txt|collection.
// It parses the uploaded body and returns candidate repos for the user to confirm.
func (s *Server) importManifest(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.UserFromContext(r.Context()); !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MiB cap
	if err != nil {
		http.Error(w, "read failed", http.StatusBadRequest)
		return
	}
	switch r.URL.Query().Get("kind") {
	case "package_json":
		writeJSON(w, http.StatusOK, ParsePackageJSON(data))
	case "requirements_txt":
		writeJSON(w, http.StatusOK, ParseRequirementsTxt(data))
	case "collection":
		var c struct {
			Name  string   `json:"name"`
			Repos []string `json:"repos"`
		}
		if err := json.Unmarshal(data, &c); err != nil {
			http.Error(w, "invalid collection json", http.StatusBadRequest)
			return
		}
		// A collection file is already owner/repo strings: everything is "resolved".
		writeJSON(w, http.StatusOK, ImportResult{Resolved: dedupeSort(c.Repos), Unresolved: []string{}})
	default:
		http.Error(w, "unknown kind", http.StatusBadRequest)
	}
}

// sanitizeFilename keeps only safe characters for a download filename.
func sanitizeFilename(name string) string {
	var b strings.Builder
	for _, ch := range name {
		switch {
		case ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z', ch >= '0' && ch <= '9', ch == '-', ch == '_':
			b.WriteRune(ch)
		default:
			b.WriteRune('-')
		}
	}
	if b.Len() == 0 {
		return "collection"
	}
	return b.String()
}

func dedupeSort(in []string) []string {
	seen := map[string]struct{}{}
	for _, s := range in {
		if s != "" {
			seen[s] = struct{}{}
		}
	}
	out := keys(seen)
	sort.Strings(out)
	return out
}
