package auth

import (
	"github-stats/internal/config"
	"github-stats/internal/crypto"
	"github-stats/internal/store"
)

const sessionCookie = "gs_session"

// Service bundles dependencies for auth HTTP handlers and middleware.
type Service struct {
	Cfg    config.Config
	Store  *store.Store
	OAuth  *OAuthClient
	Cipher *crypto.Cipher
}

// NewService constructs an auth Service.
func NewService(cfg config.Config, st *store.Store, oauth *OAuthClient, cipher *crypto.Cipher) *Service {
	return &Service{Cfg: cfg, Store: st, OAuth: oauth, Cipher: cipher}
}
