package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github-stats/internal/auth"
)

// heartbeatInterval is how often an idle SSE stream emits a comment frame. It
// keeps proxies from timing out and lets the server detect a vanished client
// (the write fails) even if no sync events arrive — and reaps the connection if
// a terminal Done event was ever dropped under buffer pressure.
const heartbeatInterval = 25 * time.Second

// syncStream handles GET /api/repos/{id}/sync/stream as Server-Sent Events of
// the repo's sync progress. It streams events published to the engine
// broadcaster until the client disconnects or a terminal Done event is sent.
func (s *Server) syncStream(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	repoID, err := repoIDParam(r)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	tracked, err := s.store.IsTracked(r.Context(), u.ID, repoID)
	if err != nil {
		http.Error(w, "lookup failed", http.StatusInternalServerError)
		return
	}
	if !tracked {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch, cancel := s.engine.Broadcaster().Subscribe(repoID)
	defer cancel()

	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			// SSE comment frame; a write error means the client is gone.
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			payload, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			if _, err := w.Write([]byte("data: ")); err != nil {
				return
			}
			if _, err := w.Write(payload); err != nil {
				return
			}
			if _, err := w.Write([]byte("\n\n")); err != nil {
				return
			}
			flusher.Flush()
			if ev.Done {
				return
			}
		}
	}
}
