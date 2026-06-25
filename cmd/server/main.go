package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github-stats/internal/api"
	"github-stats/internal/auth"
	"github-stats/internal/config"
	"github-stats/internal/crypto"
	"github-stats/internal/githubapi"
	"github-stats/internal/store"
	gosync "github-stats/internal/sync"
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

	// Per-repo client factory: mint a GitHub client using the OAuth token of the
	// repo's first tracking user (decrypted with the cipher). This is the minimal
	// per-user client construction M3 needs to add and sync a repo.
	factory := newClientFactory(st, cipher, cfg)

	engine := gosync.NewEngine(st, factory, gosync.Config{
		Concurrency:       4,
		SchedulerInterval: time.Minute,
		DeltaCadence:      30 * time.Minute,
		MaxAttempts:       5,
		FailBackoff:       time.Minute,
	})

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	engine.Start(rootCtx)
	defer engine.Stop()

	srv := api.NewServer(cfg, st, authSvc, engine, cipher)
	httpSrv := &http.Server{Addr: cfg.Addr, Handler: srv}

	go func() {
		log.Printf("listening on %s", cfg.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	<-rootCtx.Done()
	log.Println("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
}

// newClientFactory builds a gosync.ClientFactory that resolves a repo to a
// tracking user's decrypted OAuth token and returns a GitHub client for it.
func newClientFactory(st *store.Store, cipher *crypto.Cipher, cfg config.Config) gosync.ClientFactory {
	return func(repoID int64) (*githubapi.Client, error) {
		ctx := context.Background()
		var userID int64
		if err := st.DB.QueryRowContext(ctx,
			`SELECT user_id FROM repo_tracking WHERE repo_id = ? ORDER BY created_at ASC LIMIT 1`,
			repoID,
		).Scan(&userID); err != nil {
			return nil, err
		}
		cred, err := st.GetCredential(ctx, userID, "pat")
		if err == store.ErrNotFound {
			cred, err = st.GetCredential(ctx, userID, "oauth")
		}
		if err != nil {
			return nil, err
		}
		token, err := cipher.Decrypt(cred.EncToken)
		if err != nil {
			return nil, err
		}
		return githubapi.NewClient(githubapi.Options{
			Token:       string(token),
			GraphQLURL:  cfg.GitHubAPIBaseURL + "/graphql",
			RESTBaseURL: cfg.GitHubAPIBaseURL,
			Store:       st,
		}), nil
	}
}
