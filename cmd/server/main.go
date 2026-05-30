package main

import (
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"

	"github-stats/internal/api"
	"github-stats/internal/auth"
	"github-stats/internal/config"
	"github-stats/internal/crypto"
	"github-stats/internal/store"
)

func main() {
	_ = godotenv.Load() // optional .env in dev; ignored if absent

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	st, err := store.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	cipher, err := crypto.NewCipher(cfg.EncryptionKey)
	if err != nil {
		log.Fatalf("cipher: %v", err)
	}

	oauth := &auth.OAuthClient{
		ClientID:     cfg.GitHubClientID,
		ClientSecret: cfg.GitHubClientSecret,
		RedirectURL:  cfg.RedirectURL(),
		OAuthBaseURL: cfg.GitHubOAuthBaseURL,
		APIBaseURL:   cfg.GitHubAPIBaseURL,
		HTTP:         http.DefaultClient,
	}
	authSvc := auth.NewService(cfg, st, oauth, cipher)
	srv := api.NewServer(cfg, st, authSvc)

	log.Printf("listening on %s", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, srv); err != nil {
		log.Fatal(err)
	}
}
