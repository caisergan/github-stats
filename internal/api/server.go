package api

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github-stats/internal/auth"
	"github-stats/internal/config"
	"github-stats/internal/store"
	"github-stats/web"
)

// Server holds HTTP dependencies and the router.
type Server struct {
	cfg    config.Config
	store  *store.Store
	auth   *auth.Service
	router chi.Router
}

// NewServer builds the router with all routes mounted.
func NewServer(cfg config.Config, st *store.Store, authSvc *auth.Service) *Server {
	s := &Server{cfg: cfg, store: st, auth: authSvc}
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
