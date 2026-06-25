package api

import (
	"encoding/json"
	"net/http"
	"strconv"

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
	out := make([]collectionJSON, 0, len(cols))
	for _, c := range cols {
		repos, err := s.store.ListCollectionRepos(r.Context(), u.ID, c.ID)
		if err != nil {
			http.Error(w, "list failed", http.StatusInternalServerError)
			return
		}
		ids := make([]int64, 0, len(repos))
		for _, rp := range repos {
			ids = append(ids, rp.ID)
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
