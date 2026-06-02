package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github-stats/internal/auth"
	"github-stats/internal/config"
	"github-stats/internal/crypto"
	"github-stats/internal/metrics"
	"github-stats/internal/store"
	gosync "github-stats/internal/sync"
	"github-stats/web"
)

// Server holds HTTP dependencies and the router.
type Server struct {
	cfg      config.Config
	store    *store.Store
	auth     *auth.Service
	engine   *gosync.Engine
	cipher   *crypto.Cipher
	router   chi.Router
	registry *metrics.Registry
	now      func() time.Time
}

// NewServer builds the router with all routes mounted. It also takes the sync
// Engine (for triggering/streaming syncs) and the Cipher (to decrypt the
// caller's OAuth token when minting a per-user GitHub client).
func NewServer(cfg config.Config, st *store.Store, authSvc *auth.Service, engine *gosync.Engine, cipher *crypto.Cipher) *Server {
	s := &Server{cfg: cfg, store: st, auth: authSvc, engine: engine, cipher: cipher}
	s.registry = metrics.DefaultRegistry()
	if s.now == nil {
		s.now = time.Now
	}
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	// Auth routes.
	r.Get("/auth/github", authSvc.Login)
	r.Get("/auth/github/callback", authSvc.Callback)
	r.Get("/auth/logout", authSvc.Logout)

	// JSON API (auth-gated).
	r.Route("/api", func(api chi.Router) {
		api.Group(func(pr chi.Router) {
			pr.Use(authSvc.RequireUser)
			pr.Get("/me", s.me)
			pr.Post("/repos", s.addRepo)
			pr.Get("/repos", s.listRepos)
			pr.Delete("/repos/{id}", s.untrackRepo)
			pr.Post("/repos/{id}/refresh", s.refreshRepo)
			pr.Get("/repos/{id}/sync/stream", s.syncStream)
			pr.Get("/repos/{id}/metrics", s.repoMetrics)
		})
	})

	// Embedded SPA (must be last; serves everything else).
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
			return
		}
		web.Handler().ServeHTTP(w, req)
	})

	s.router = r
	return s
}

// Router returns the chi router.
func (s *Server) Router() chi.Router { return s.router }

// ServeHTTP lets Server satisfy http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}
