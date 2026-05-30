# M1 — Skeleton & Auth Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up a self-hosted Go single-binary that lets a user log in with GitHub OAuth, persists the user + an encrypted token + a session in SQLite, and serves an embedded React shell that shows who is logged in.

**Architecture:** Go backend (Chi router) over pure-Go SQLite (`modernc.org/sqlite`, WAL). GitHub OAuth Authorization-Code flow; tokens AES-GCM encrypted at rest; sessions are random tokens in an httpOnly cookie backed by a DB row. A React + Vite SPA is embedded via `go:embed` for production (single binary) and proxied by Vite in development. This is milestone M1 of the design spec at `docs/superpowers/specs/2026-05-30-github-stats-design.md`.

**Tech Stack:** Go 1.22+, `github.com/go-chi/chi/v5`, `modernc.org/sqlite`, `github.com/joho/godotenv` (dev only), React 18 + Vite + TypeScript.

---

## File Structure

```
github-stats/
├── go.mod
├── Makefile
├── .gitignore
├── .env.example
├── cmd/server/main.go              # entrypoint: load config, wire deps, run server
├── internal/
│   ├── config/
│   │   ├── config.go               # Config struct + Load()
│   │   └── config_test.go
│   ├── crypto/
│   │   ├── crypto.go               # AES-GCM Cipher (encrypt/decrypt tokens)
│   │   └── crypto_test.go
│   ├── store/
│   │   ├── store.go                # Store{DB}, Open(), ErrNotFound
│   │   ├── migrate.go              # embedded migration runner
│   │   ├── migrations/0001_init.sql
│   │   ├── store_test.go           # migrations apply + idempotent
│   │   ├── users.go                # User + UpsertUser/GetUserByID
│   │   ├── users_test.go
│   │   ├── sessions.go             # Session + Create/Get/Delete
│   │   ├── sessions_test.go
│   │   ├── credentials.go          # Credential + Upsert/Get
│   │   └── credentials_test.go
│   ├── auth/
│   │   ├── oauth.go                # OAuthClient: AuthorizeURL/Exchange/GetUser
│   │   ├── oauth_test.go
│   │   ├── service.go              # Service wiring store+oauth+cipher+cfg
│   │   ├── handlers.go             # Login/Callback/Logout HTTP handlers
│   │   ├── handlers_test.go
│   │   ├── middleware.go           # RequireUser + context helpers
│   │   └── middleware_test.go
│   └── api/
│       ├── server.go               # Chi router, route mounting, static serving
│       ├── me.go                   # GET /api/me
│       └── me_test.go              # /api/me + server/SPA-fallback + /api 404 tests
└── web/
    ├── embed.go                    # //go:embed dist + SPA-fallback Handler
    ├── package.json
    ├── tsconfig.json
    ├── tsconfig.node.json
    ├── vite.config.ts              # /api + /auth dev proxy, build → dist
    ├── index.html
    ├── dist/index.html             # committed placeholder so the package always builds
    └── src/{main.tsx,App.tsx,api.ts}
```

---

## Task 1: Project scaffolding

**Files:**
- Create: `go.mod`, `.gitignore`, `.env.example`, `Makefile`, `cmd/server/main.go`

- [ ] **Step 1: Create `go.mod`**

```
module github-stats

go 1.22

require (
	github.com/go-chi/chi/v5 v5.1.0
	github.com/joho/godotenv v1.5.1
	modernc.org/sqlite v1.34.1
)
```

- [ ] **Step 2: Create `.gitignore`**

```
/github-stats
/bin/
*.db
*.db-wal
*.db-shm
.env
/web/node_modules/
/web/dist/*
!/web/dist/index.html
/web/*.tsbuildinfo
/web/vite.config.d.ts
```

- [ ] **Step 3: Create `.env.example`**

```
# Copy to .env for local dev (loaded by godotenv).
ADDR=:8080
DATABASE_PATH=app.db
BASE_URL=http://localhost:8080
GITHUB_CLIENT_ID=replace-me
GITHUB_CLIENT_SECRET=replace-me
# 32-byte key, hex-encoded (64 hex chars). Generate: openssl rand -hex 32
ENCRYPTION_KEY=0000000000000000000000000000000000000000000000000000000000000000
```

- [ ] **Step 4: Create a temporary `cmd/server/main.go` so the module compiles**

```go
package main

func main() {}
```

- [ ] **Step 5: Create `Makefile`**

```makefile
.PHONY: dev-api dev-web build test tidy

tidy:
	go mod tidy

test:
	go test ./...

# Build frontend first, then embed into the Go binary.
build:
	cd web && npm install && npm run build
	go build -o bin/github-stats ./cmd/server

dev-api:
	go run ./cmd/server

dev-web:
	cd web && npm run dev
```

- [ ] **Step 6: Fetch deps and verify it builds**

Run: `go mod tidy && go build ./...`
Expected: completes with no errors (an empty binary builds).

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum .gitignore .env.example Makefile cmd/
git commit -m "chore: scaffold go module and project layout"
```

---

## Task 2: Config package

**Files:**
- Create: `internal/config/config.go`, `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

`internal/config/config_test.go`:
```go
package config

import "testing"

func TestLoadRequiresClientID(t *testing.T) {
	env := map[string]string{
		"GITHUB_CLIENT_SECRET": "s",
		"ENCRYPTION_KEY":       "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff",
	}
	_, err := Load(func(k string) string { return env[k] })
	if err == nil {
		t.Fatal("expected error when GITHUB_CLIENT_ID missing")
	}
}

func TestLoadParsesDefaultsAndKey(t *testing.T) {
	env := map[string]string{
		"GITHUB_CLIENT_ID":     "id",
		"GITHUB_CLIENT_SECRET": "secret",
		"ENCRYPTION_KEY":       "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff",
	}
	cfg, err := Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Addr != ":8080" {
		t.Errorf("Addr default = %q, want :8080", cfg.Addr)
	}
	if cfg.DatabasePath != "app.db" {
		t.Errorf("DatabasePath default = %q, want app.db", cfg.DatabasePath)
	}
	if len(cfg.EncryptionKey) != 32 {
		t.Errorf("EncryptionKey len = %d, want 32", len(cfg.EncryptionKey))
	}
	if cfg.RedirectURL() != "http://localhost:8080/auth/github/callback" {
		t.Errorf("RedirectURL = %q", cfg.RedirectURL())
	}
}

func TestLoadRejectsBadKey(t *testing.T) {
	env := map[string]string{
		"GITHUB_CLIENT_ID":     "id",
		"GITHUB_CLIENT_SECRET": "secret",
		"ENCRYPTION_KEY":       "tooshort",
	}
	if _, err := Load(func(k string) string { return env[k] }); err == nil {
		t.Fatal("expected error for non-32-byte key")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -v`
Expected: FAIL — `undefined: Load`.

- [ ] **Step 3: Write minimal implementation**

`internal/config/config.go`:
```go
package config

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Config holds all runtime configuration.
type Config struct {
	Addr               string
	DatabasePath       string
	BaseURL            string
	GitHubClientID     string
	GitHubClientSecret string
	GitHubScopes       string
	EncryptionKey      []byte
	SessionTTL         time.Duration

	// Overridable for tests; default to the public GitHub endpoints.
	GitHubOAuthBaseURL string // https://github.com
	GitHubAPIBaseURL   string // https://api.github.com
}

// RedirectURL is the OAuth callback URL derived from BaseURL.
func (c Config) RedirectURL() string {
	return strings.TrimRight(c.BaseURL, "/") + "/auth/github/callback"
}

func get(getenv func(string) string, key, def string) string {
	if v := getenv(key); v != "" {
		return v
	}
	return def
}

// Load builds Config from a getenv function (os.Getenv in production).
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		Addr:               get(getenv, "ADDR", ":8080"),
		DatabasePath:       get(getenv, "DATABASE_PATH", "app.db"),
		BaseURL:            get(getenv, "BASE_URL", "http://localhost:8080"),
		GitHubClientID:     getenv("GITHUB_CLIENT_ID"),
		GitHubClientSecret: getenv("GITHUB_CLIENT_SECRET"),
		GitHubScopes:       get(getenv, "GITHUB_SCOPES", "read:user public_repo"),
		SessionTTL:         30 * 24 * time.Hour,
		GitHubOAuthBaseURL: get(getenv, "GITHUB_OAUTH_BASE_URL", "https://github.com"),
		GitHubAPIBaseURL:   get(getenv, "GITHUB_API_BASE_URL", "https://api.github.com"),
	}
	if cfg.GitHubClientID == "" {
		return Config{}, fmt.Errorf("GITHUB_CLIENT_ID is required")
	}
	if cfg.GitHubClientSecret == "" {
		return Config{}, fmt.Errorf("GITHUB_CLIENT_SECRET is required")
	}
	key, err := hex.DecodeString(getenv("ENCRYPTION_KEY"))
	if err != nil {
		return Config{}, fmt.Errorf("ENCRYPTION_KEY must be hex: %w", err)
	}
	if len(key) != 32 {
		return Config{}, fmt.Errorf("ENCRYPTION_KEY must decode to 32 bytes, got %d", len(key))
	}
	cfg.EncryptionKey = key
	return cfg, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -v`
Expected: PASS (all three tests).

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: config loading with validation"
```

---

## Task 3: Crypto package (AES-GCM)

**Files:**
- Create: `internal/crypto/crypto.go`, `internal/crypto/crypto_test.go`

- [ ] **Step 1: Write the failing test**

`internal/crypto/crypto_test.go`:
```go
package crypto

import (
	"bytes"
	"testing"
)

func key32() []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i)
	}
	return k
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	c, err := NewCipher(key32())
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("gho_secrettoken")
	enc, err := c.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	if enc == string(plain) {
		t.Fatal("ciphertext equals plaintext")
	}
	got, err := c.Decrypt(enc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("round trip mismatch: %q != %q", got, plain)
	}
}

func TestEncryptIsNonDeterministic(t *testing.T) {
	c, _ := NewCipher(key32())
	a, _ := c.Encrypt([]byte("same"))
	b, _ := c.Encrypt([]byte("same"))
	if a == b {
		t.Fatal("expected unique ciphertext per call (random nonce)")
	}
}

func TestDecryptRejectsTampered(t *testing.T) {
	c, _ := NewCipher(key32())
	enc, _ := c.Encrypt([]byte("data"))
	tampered := "00" + enc[2:]
	if _, err := c.Decrypt(tampered); err == nil {
		t.Fatal("expected error on tampered ciphertext")
	}
}

func TestNewCipherRejectsBadKey(t *testing.T) {
	if _, err := NewCipher(make([]byte, 16)); err == nil {
		t.Fatal("expected error for non-32-byte key")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/crypto/ -v`
Expected: FAIL — `undefined: NewCipher`.

- [ ] **Step 3: Write minimal implementation**

`internal/crypto/crypto.go`:
```go
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// Cipher provides AES-256-GCM encryption for tokens at rest.
type Cipher struct {
	aead cipher.AEAD
}

// NewCipher builds a Cipher from a 32-byte key.
func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt returns a hex-encoded nonce||ciphertext string.
func (c *Cipher) Encrypt(plaintext []byte) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := c.aead.Seal(nonce, nonce, plaintext, nil)
	return hex.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt.
func (c *Cipher) Decrypt(s string) ([]byte, error) {
	raw, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	ns := c.aead.NonceSize()
	if len(raw) < ns {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ct := raw[:ns], raw[ns:]
	return c.aead.Open(nil, nonce, ct, nil)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/crypto/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/crypto/
git commit -m "feat: AES-GCM cipher for token encryption"
```

---

## Task 4: Store open + migrations

**Files:**
- Create: `internal/store/store.go`, `internal/store/migrate.go`, `internal/store/migrations/0001_init.sql`, `internal/store/store_test.go`

- [ ] **Step 1: Write the migration SQL**

`internal/store/migrations/0001_init.sql`:
```sql
CREATE TABLE users (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    github_id   INTEGER NOT NULL UNIQUE,
    login       TEXT    NOT NULL,
    avatar_url  TEXT    NOT NULL DEFAULT '',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE credentials (
    user_id    INTEGER NOT NULL,
    kind       TEXT    NOT NULL,             -- 'oauth' | 'pat'
    enc_token  TEXT    NOT NULL,
    scopes     TEXT    NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, kind),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE sessions (
    id         TEXT    PRIMARY KEY,
    user_id    INTEGER NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX idx_sessions_user ON sessions(user_id);
```

- [ ] **Step 2: Write the failing test**

`internal/store/store_test.go`:
```go
package store

import (
	"path/filepath"
	"testing"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenAppliesMigrations(t *testing.T) {
	s := openTemp(t)
	for _, table := range []string{"users", "credentials", "sessions"} {
		var name string
		err := s.DB.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s1.Close()
	s2, err := Open(path) // re-open must not re-run migrations and fail
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	s2.Close()
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/store/ -v`
Expected: FAIL — `undefined: Open`.

- [ ] **Step 4: Write the store + migration runner**

`internal/store/store.go`:
```go
package store

import (
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned by DAO lookups when no row matches.
var ErrNotFound = errors.New("store: not found")

// Store wraps the SQLite database connection.
type Store struct {
	DB *sql.DB
}

// Open opens (creating if needed) the SQLite database, enables WAL + foreign
// keys, and applies migrations.
func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)",
		path,
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite: serialize writers; simplest correct default
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{DB: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.DB.Close() }
```

`internal/store/migrate.go`:
```go
package store

import (
	"embed"
	"fmt"
	"sort"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrate applies any migration files not yet recorded in schema_migrations.
func (s *Store) migrate() error {
	if _, err := s.DB.Exec(
		`CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY)`,
	); err != nil {
		return err
	}
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		var exists string
		err := s.DB.QueryRow(
			`SELECT version FROM schema_migrations WHERE version=?`, name,
		).Scan(&exists)
		if err == nil {
			continue // already applied
		}
		sqlBytes, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := s.DB.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(sqlBytes)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations(version) VALUES(?)`, name,
		); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/store/ -v`
Expected: PASS (both tests).

- [ ] **Step 6: Commit**

```bash
git add internal/store/
git commit -m "feat: sqlite store with embedded migrations"
```

---

## Task 5: Users DAO

**Files:**
- Create: `internal/store/users.go`, `internal/store/users_test.go`

- [ ] **Step 1: Write the failing test**

`internal/store/users_test.go`:
```go
package store

import (
	"context"
	"testing"
)

func TestUpsertUserInsertsThenUpdates(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	id, err := s.UpsertUser(ctx, &User{GitHubID: 42, Login: "octocat", AvatarURL: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}

	// Same github_id with changed login should update, not duplicate.
	id2, err := s.UpsertUser(ctx, &User{GitHubID: 42, Login: "octocat-renamed", AvatarURL: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if id2 != id {
		t.Fatalf("upsert created new row: %d != %d", id2, id)
	}

	u, err := s.GetUserByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if u.Login != "octocat-renamed" || u.AvatarURL != "b" {
		t.Fatalf("update not applied: %+v", u)
	}
}

func TestGetUserByIDNotFound(t *testing.T) {
	s := openTemp(t)
	_, err := s.GetUserByID(context.Background(), 999)
	if err != ErrNotFound {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestUpsertUser -v`
Expected: FAIL — `undefined: User`.

- [ ] **Step 3: Write minimal implementation**

`internal/store/users.go`:
```go
package store

import (
	"context"
	"database/sql"
	"time"
)

// User is an authenticated GitHub user.
type User struct {
	ID        int64
	GitHubID  int64
	Login     string
	AvatarURL string
	CreatedAt time.Time
}

// UpsertUser inserts or updates a user by github_id and returns the local id.
func (s *Store) UpsertUser(ctx context.Context, u *User) (int64, error) {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO users (github_id, login, avatar_url)
		VALUES (?, ?, ?)
		ON CONFLICT(github_id) DO UPDATE SET
			login = excluded.login,
			avatar_url = excluded.avatar_url`,
		u.GitHubID, u.Login, u.AvatarURL,
	)
	if err != nil {
		return 0, err
	}
	var id int64
	if err := s.DB.QueryRowContext(ctx,
		`SELECT id FROM users WHERE github_id = ?`, u.GitHubID,
	).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

// GetUserByID returns the user with the given local id, or ErrNotFound.
func (s *Store) GetUserByID(ctx context.Context, id int64) (*User, error) {
	var u User
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, github_id, login, avatar_url, created_at FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &u.GitHubID, &u.Login, &u.AvatarURL, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestUpsertUser -v && go test ./internal/store/ -run TestGetUserByIDNotFound -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/users.go internal/store/users_test.go
git commit -m "feat: users DAO with upsert"
```

---

## Task 6: Sessions DAO

**Files:**
- Create: `internal/store/sessions.go`, `internal/store/sessions_test.go`

- [ ] **Step 1: Write the failing test**

`internal/store/sessions_test.go`:
```go
package store

import (
	"context"
	"testing"
	"time"
)

func seedUser(t *testing.T, s *Store) int64 {
	t.Helper()
	id, err := s.UpsertUser(context.Background(), &User{GitHubID: 1, Login: "u"})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSessionCreateGetDelete(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	uid := seedUser(t, s)

	sess, err := s.CreateSession(ctx, uid, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if sess.ID == "" {
		t.Fatal("expected non-empty session id")
	}

	got, err := s.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.UserID != uid {
		t.Fatalf("UserID = %d, want %d", got.UserID, uid)
	}

	if err := s.DeleteSession(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSession(ctx, sess.ID); err != ErrNotFound {
		t.Fatalf("after delete got %v, want ErrNotFound", err)
	}
}

func TestGetSessionExpired(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	uid := seedUser(t, s)

	sess, err := s.CreateSession(ctx, uid, -time.Minute) // already expired
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSession(ctx, sess.ID); err != ErrNotFound {
		t.Fatalf("expired session got %v, want ErrNotFound", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestSession -v`
Expected: FAIL — `undefined: CreateSession`.

- [ ] **Step 3: Write minimal implementation**

`internal/store/sessions.go`:
```go
package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"time"
)

// Session is a server-side login session referenced by an httpOnly cookie.
type Session struct {
	ID        string
	UserID    int64
	ExpiresAt time.Time
}

func newSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// CreateSession creates a session valid for ttl.
func (s *Store) CreateSession(ctx context.Context, userID int64, ttl time.Duration) (*Session, error) {
	id, err := newSessionID()
	if err != nil {
		return nil, err
	}
	exp := time.Now().Add(ttl).UTC()
	if _, err := s.DB.ExecContext(ctx,
		`INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)`,
		id, userID, exp,
	); err != nil {
		return nil, err
	}
	return &Session{ID: id, UserID: userID, ExpiresAt: exp}, nil
}

// GetSession returns a non-expired session, or ErrNotFound.
func (s *Store) GetSession(ctx context.Context, id string) (*Session, error) {
	var sess Session
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, user_id, expires_at FROM sessions WHERE id = ?`, id,
	).Scan(&sess.ID, &sess.UserID, &sess.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if time.Now().After(sess.ExpiresAt) {
		_ = s.DeleteSession(ctx, id)
		return nil, ErrNotFound
	}
	return &sess, nil
}

// DeleteSession removes a session (logout).
func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	return err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestSession -v && go test ./internal/store/ -run TestGetSessionExpired -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/sessions.go internal/store/sessions_test.go
git commit -m "feat: sessions DAO with expiry"
```

---

## Task 7: Credentials DAO

**Files:**
- Create: `internal/store/credentials.go`, `internal/store/credentials_test.go`

- [ ] **Step 1: Write the failing test**

`internal/store/credentials_test.go`:
```go
package store

import (
	"context"
	"testing"
)

func TestCredentialUpsertGet(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()
	uid := seedUser(t, s)

	if err := s.UpsertCredential(ctx, &Credential{
		UserID: uid, Kind: "oauth", EncToken: "enc1", Scopes: "read:user",
	}); err != nil {
		t.Fatal(err)
	}

	// Upsert same (user, kind) replaces the token.
	if err := s.UpsertCredential(ctx, &Credential{
		UserID: uid, Kind: "oauth", EncToken: "enc2", Scopes: "repo",
	}); err != nil {
		t.Fatal(err)
	}

	c, err := s.GetCredential(ctx, uid, "oauth")
	if err != nil {
		t.Fatal(err)
	}
	if c.EncToken != "enc2" || c.Scopes != "repo" {
		t.Fatalf("upsert did not replace: %+v", c)
	}
}

func TestGetCredentialNotFound(t *testing.T) {
	s := openTemp(t)
	uid := seedUser(t, s)
	if _, err := s.GetCredential(context.Background(), uid, "pat"); err != ErrNotFound {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestCredential -v`
Expected: FAIL — `undefined: Credential`.

- [ ] **Step 3: Write minimal implementation**

`internal/store/credentials.go`:
```go
package store

import (
	"context"
	"database/sql"
)

// Credential is a stored GitHub credential (encrypted token) for a user.
type Credential struct {
	UserID   int64
	Kind     string // "oauth" | "pat"
	EncToken string
	Scopes   string
}

// UpsertCredential inserts or replaces a credential for (user_id, kind).
func (s *Store) UpsertCredential(ctx context.Context, c *Credential) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO credentials (user_id, kind, enc_token, scopes)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(user_id, kind) DO UPDATE SET
			enc_token = excluded.enc_token,
			scopes = excluded.scopes`,
		c.UserID, c.Kind, c.EncToken, c.Scopes,
	)
	return err
}

// GetCredential returns the credential for (user_id, kind), or ErrNotFound.
func (s *Store) GetCredential(ctx context.Context, userID int64, kind string) (*Credential, error) {
	var c Credential
	err := s.DB.QueryRowContext(ctx,
		`SELECT user_id, kind, enc_token, scopes FROM credentials WHERE user_id = ? AND kind = ?`,
		userID, kind,
	).Scan(&c.UserID, &c.Kind, &c.EncToken, &c.Scopes)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestCredential -v && go test ./internal/store/ -run TestGetCredentialNotFound -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/credentials.go internal/store/credentials_test.go
git commit -m "feat: credentials DAO for encrypted tokens"
```

---

## Task 8: GitHub OAuth client

**Files:**
- Create: `internal/auth/oauth.go`, `internal/auth/oauth_test.go`

- [ ] **Step 1: Write the failing test**

`internal/auth/oauth_test.go`:
```go
package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAuthorizeURL(t *testing.T) {
	c := &OAuthClient{ClientID: "cid", RedirectURL: "http://app/cb", OAuthBaseURL: "https://github.com"}
	got := c.AuthorizeURL("xyz", "read:user public_repo")
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("client_id") != "cid" || q.Get("state") != "xyz" ||
		q.Get("redirect_uri") != "http://app/cb" || q.Get("scope") != "read:user public_repo" {
		t.Fatalf("bad authorize url: %s", got)
	}
}

func TestExchangeAndGetUser(t *testing.T) {
	// Fake GitHub OAuth token endpoint.
	oauthSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login/oauth/access_token" {
			t.Errorf("unexpected oauth path %s", r.URL.Path)
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("missing Accept json header")
		}
		_ = r.ParseForm()
		if r.Form.Get("code") != "thecode" {
			t.Errorf("code = %q", r.Form.Get("code"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"gho_tok","token_type":"bearer","scope":"repo"}`))
	}))
	defer oauthSrv.Close()

	// Fake GitHub API user endpoint.
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			t.Errorf("unexpected api path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer gho_tok" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":99,"login":"octocat","avatar_url":"http://a/x.png"}`))
	}))
	defer apiSrv.Close()

	c := &OAuthClient{
		ClientID:     "cid",
		ClientSecret: "sec",
		RedirectURL:  "http://app/cb",
		OAuthBaseURL: oauthSrv.URL,
		APIBaseURL:   apiSrv.URL,
		HTTP:         oauthSrv.Client(),
	}
	ctx := context.Background()
	tok, scope, err := c.Exchange(ctx, "thecode")
	if err != nil {
		t.Fatal(err)
	}
	if tok != "gho_tok" || scope != "repo" {
		t.Fatalf("exchange = %q scope %q", tok, scope)
	}
	u, err := c.GetUser(ctx, tok)
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != 99 || u.Login != "octocat" || !strings.HasSuffix(u.AvatarURL, "x.png") {
		t.Fatalf("user = %+v", u)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/auth/ -run 'TestAuthorizeURL|TestExchangeAndGetUser' -v`
Expected: FAIL — `undefined: OAuthClient`.

- [ ] **Step 3: Write minimal implementation**

`internal/auth/oauth.go`:
```go
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// OAuthClient performs the GitHub OAuth code exchange and user lookup.
// Base URLs are injectable so tests can point at httptest servers.
type OAuthClient struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	OAuthBaseURL string // e.g. https://github.com
	APIBaseURL   string // e.g. https://api.github.com
	HTTP         *http.Client
}

func (c *OAuthClient) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// GitHubUser is the subset of the GitHub user object we persist.
type GitHubUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
}

// AuthorizeURL builds the URL to redirect the user to for consent.
func (c *OAuthClient) AuthorizeURL(state, scopes string) string {
	q := url.Values{}
	q.Set("client_id", c.ClientID)
	q.Set("redirect_uri", c.RedirectURL)
	q.Set("scope", scopes)
	q.Set("state", state)
	return strings.TrimRight(c.OAuthBaseURL, "/") + "/login/oauth/authorize?" + q.Encode()
}

// Exchange swaps an authorization code for an access token; returns token and granted scope.
func (c *OAuthClient) Exchange(ctx context.Context, code string) (token, scope string, err error) {
	form := url.Values{}
	form.Set("client_id", c.ClientID)
	form.Set("client_secret", c.ClientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", c.RedirectURL)

	endpoint := strings.TrimRight(c.OAuthBaseURL, "/") + "/login/oauth/access_token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("oauth exchange: status %d", resp.StatusCode)
	}
	var body struct {
		AccessToken string `json:"access_token"`
		Scope       string `json:"scope"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", "", err
	}
	if body.Error != "" || body.AccessToken == "" {
		return "", "", fmt.Errorf("oauth exchange failed: %s", body.Error)
	}
	return body.AccessToken, body.Scope, nil
}

// GetUser fetches the authenticated user with the given token.
func (c *OAuthClient) GetUser(ctx context.Context, token string) (*GitHubUser, error) {
	endpoint := strings.TrimRight(c.APIBaseURL, "/") + "/user"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get user: status %d", resp.StatusCode)
	}
	var u GitHubUser
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return nil, err
	}
	return &u, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/auth/ -run 'TestAuthorizeURL|TestExchangeAndGetUser' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/oauth.go internal/auth/oauth_test.go
git commit -m "feat: github oauth client (exchange + get user)"
```

---

## Task 9: Auth service + session middleware

**Files:**
- Create: `internal/auth/service.go`, `internal/auth/middleware.go`, `internal/auth/middleware_test.go`

- [ ] **Step 1: Write the failing test**

`internal/auth/middleware_test.go`:
```go
package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github-stats/internal/config"
	"github-stats/internal/crypto"
	"github-stats/internal/store"
)

func testService(t *testing.T) *Service {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	key := make([]byte, 32)
	cph, _ := crypto.NewCipher(key)
	cfg := config.Config{SessionTTL: time.Hour, BaseURL: "http://localhost:8080"}
	return NewService(cfg, st, &OAuthClient{}, cph)
}

func TestRequireUserRejectsNoCookie(t *testing.T) {
	svc := testService(t)
	h := svc.RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not run without session")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRequireUserLoadsUser(t *testing.T) {
	svc := testService(t)
	ctx := context.Background()
	uid, _ := svc.Store.UpsertUser(ctx, &store.User{GitHubID: 7, Login: "neo"})
	sess, _ := svc.Store.CreateSession(ctx, uid, time.Hour)

	var seen *store.User
	h := svc.RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := UserFromContext(r.Context())
		if !ok {
			t.Fatal("user not in context")
		}
		seen = u
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: sess.ID})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if seen == nil || seen.Login != "neo" {
		t.Fatalf("loaded user = %+v", seen)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/auth/ -run TestRequireUser -v`
Expected: FAIL — `undefined: NewService`.

- [ ] **Step 3: Write the service + middleware**

`internal/auth/service.go`:
```go
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
```

`internal/auth/middleware.go`:
```go
package auth

import (
	"context"
	"net/http"

	"github-stats/internal/store"
)

type ctxKey int

const userCtxKey ctxKey = 0

// UserFromContext returns the authenticated user set by RequireUser.
func UserFromContext(ctx context.Context) (*store.User, bool) {
	u, ok := ctx.Value(userCtxKey).(*store.User)
	return u, ok
}

// RequireUser is middleware that loads the session user or returns 401.
func (s *Service) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		sess, err := s.Store.GetSession(r.Context(), c.Value)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		user, err := s.Store.GetUserByID(r.Context(), sess.UserID)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), userCtxKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/auth/ -run TestRequireUser -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/service.go internal/auth/middleware.go internal/auth/middleware_test.go
git commit -m "feat: auth service and session middleware"
```

---

## Task 10: Auth HTTP handlers (login / callback / logout)

**Files:**
- Create: `internal/auth/handlers.go`, `internal/auth/handlers_test.go`

- [ ] **Step 1: Write the failing test**

`internal/auth/handlers_test.go`:
```go
package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginRedirectsWithStateCookie(t *testing.T) {
	svc := testService(t)
	svc.OAuth = &OAuthClient{ClientID: "cid", RedirectURL: "http://app/cb", OAuthBaseURL: "https://github.com"}

	rec := httptest.NewRecorder()
	svc.Login(rec, httptest.NewRequest(http.MethodGet, "/auth/github", nil))

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "login/oauth/authorize") || !strings.Contains(loc, "state=") {
		t.Fatalf("bad redirect: %s", loc)
	}
	var stateSet bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == stateCookie && c.Value != "" {
			stateSet = true
		}
	}
	if !stateSet {
		t.Fatal("state cookie not set")
	}
}

func TestCallbackCreatesUserAndSession(t *testing.T) {
	svc := testService(t)

	// Fake GitHub: token endpoint + user endpoint.
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/access_token"):
			w.Write([]byte(`{"access_token":"gho_tok","scope":"read:user"}`))
		case strings.HasSuffix(r.URL.Path, "/user"):
			w.Write([]byte(`{"id":555,"login":"trinity","avatar_url":"http://a/t.png"}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer gh.Close()
	svc.OAuth = &OAuthClient{
		ClientID: "cid", ClientSecret: "sec", RedirectURL: "http://app/cb",
		OAuthBaseURL: gh.URL, APIBaseURL: gh.URL, HTTP: gh.Client(),
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/github/callback?code=c&state=abc", nil)
	req.AddCookie(&http.Cookie{Name: stateCookie, Value: "abc"})
	rec := httptest.NewRecorder()
	svc.Callback(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302 (body: %s)", rec.Code, rec.Body.String())
	}
	var sessionSet bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie && c.Value != "" {
			sessionSet = true
		}
	}
	if !sessionSet {
		t.Fatal("session cookie not set")
	}
	// User persisted with encrypted credential.
	u, err := svc.Store.GetUserByID(req.Context(), 1)
	if err != nil || u.Login != "trinity" {
		t.Fatalf("user not persisted: %+v err=%v", u, err)
	}
	cred, err := svc.Store.GetCredential(req.Context(), u.ID, "oauth")
	if err != nil {
		t.Fatal(err)
	}
	dec, err := svc.Cipher.Decrypt(cred.EncToken)
	if err != nil || string(dec) != "gho_tok" {
		t.Fatalf("token not stored encrypted: %q err=%v", dec, err)
	}
}

func TestCallbackRejectsStateMismatch(t *testing.T) {
	svc := testService(t)
	req := httptest.NewRequest(http.MethodGet, "/auth/github/callback?code=c&state=evil", nil)
	req.AddCookie(&http.Cookie{Name: stateCookie, Value: "good"})
	rec := httptest.NewRecorder()
	svc.Callback(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/auth/ -run TestCallback -v`
Expected: FAIL — `undefined: stateCookie` / `svc.Login undefined`.

- [ ] **Step 3: Write minimal implementation**

`internal/auth/handlers.go`:
```go
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github-stats/internal/store"
)

const stateCookie = "gs_oauth_state"

func randomToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Service) secureCookies() bool {
	return strings.HasPrefix(s.Cfg.BaseURL, "https://")
}

// Login starts the OAuth flow: set a state cookie and redirect to GitHub.
func (s *Service) Login(w http.ResponseWriter, r *http.Request) {
	state := randomToken()
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookie,
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secureCookies(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})
	http.Redirect(w, r, s.OAuth.AuthorizeURL(state, s.Cfg.GitHubScopes), http.StatusFound)
}

// Callback completes the OAuth flow.
func (s *Service) Callback(w http.ResponseWriter, r *http.Request) {
	stateParam := r.URL.Query().Get("state")
	c, err := r.Cookie(stateCookie)
	if err != nil || stateParam == "" || c.Value != stateParam {
		http.Error(w, "invalid oauth state", http.StatusBadRequest)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	token, scope, err := s.OAuth.Exchange(r.Context(), code)
	if err != nil {
		http.Error(w, "oauth exchange failed", http.StatusBadGateway)
		return
	}
	ghUser, err := s.OAuth.GetUser(r.Context(), token)
	if err != nil {
		http.Error(w, "fetch user failed", http.StatusBadGateway)
		return
	}

	uid, err := s.Store.UpsertUser(r.Context(), &store.User{
		GitHubID: ghUser.ID, Login: ghUser.Login, AvatarURL: ghUser.AvatarURL,
	})
	if err != nil {
		http.Error(w, "persist user failed", http.StatusInternalServerError)
		return
	}
	encToken, err := s.Cipher.Encrypt([]byte(token))
	if err != nil {
		http.Error(w, "encrypt failed", http.StatusInternalServerError)
		return
	}
	if err := s.Store.UpsertCredential(r.Context(), &store.Credential{
		UserID: uid, Kind: "oauth", EncToken: encToken, Scopes: scope,
	}); err != nil {
		http.Error(w, "persist credential failed", http.StatusInternalServerError)
		return
	}
	sess, err := s.Store.CreateSession(r.Context(), uid, s.Cfg.SessionTTL)
	if err != nil {
		http.Error(w, "session failed", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    sess.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secureCookies(),
		SameSite: http.SameSiteLaxMode,
		Expires:  sess.ExpiresAt,
	})
	// Clear the state cookie.
	http.SetCookie(w, &http.Cookie{Name: stateCookie, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/", http.StatusFound)
}

// Logout deletes the session and clears the cookie.
func (s *Service) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = s.Store.DeleteSession(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/", http.StatusFound)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/auth/ -v`
Expected: PASS (all auth tests).

- [ ] **Step 5: Commit**

```bash
git add internal/auth/handlers.go internal/auth/handlers_test.go
git commit -m "feat: oauth login/callback/logout handlers"
```

---

## Task 11: Embedded static handler + API server + /api/me

**Files:**
- Create: `web/embed.go`, `web/dist/index.html` (placeholder), `internal/api/server.go`, `internal/api/me.go`, `internal/api/me_test.go` (server/SPA-fallback tests live here alongside the `/api/me` tests)

- [ ] **Step 1: Create the placeholder build output so the package compiles**

`web/dist/index.html`:
```html
<!doctype html>
<html><head><meta charset="utf-8"><title>github-stats</title></head>
<body><div id="root">loading…</div></body></html>
```

- [ ] **Step 2: Write the embedded static handler**

`web/embed.go`:
```go
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler serves the built SPA from the embedded dist directory, falling back
// to index.html for client-side routes (anything that is not a real file).
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If the requested file exists, serve it; otherwise serve index.html.
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(sub, p); err != nil {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 3: Write the failing /api/me test**

`internal/api/me_test.go`:
```go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github-stats/internal/auth"
	"github-stats/internal/config"
	"github-stats/internal/crypto"
	"github-stats/internal/store"
)

func testServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cph, _ := crypto.NewCipher(make([]byte, 32))
	cfg := config.Config{SessionTTL: time.Hour, BaseURL: "http://localhost:8080"}
	svc := auth.NewService(cfg, st, &auth.OAuthClient{}, cph)
	return NewServer(cfg, st, svc), st
}

func TestMeUnauthorized(t *testing.T) {
	srv, _ := testServer(t)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/me", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestMeReturnsUser(t *testing.T) {
	srv, st := testServer(t)
	ctx := context.Background()
	uid, _ := st.UpsertUser(ctx, &store.User{GitHubID: 3, Login: "morpheus", AvatarURL: "http://a/m.png"})
	sess, _ := st.CreateSession(ctx, uid, time.Hour)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.AddCookie(&http.Cookie{Name: "gs_session", Value: sess.ID})
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["login"] != "morpheus" {
		t.Fatalf("login = %v", body["login"])
	}
}

func TestSPAFallbackServesIndex(t *testing.T) {
	srv, _ := testServer(t)
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/owner/repo", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type = %q, want html", ct)
	}
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/api/ -v`
Expected: FAIL — `undefined: NewServer`.

- [ ] **Step 5: Write the server + me handler**

`internal/api/me.go`:
```go
package api

import (
	"encoding/json"
	"net/http"

	"github-stats/internal/auth"
)

// me handles GET /api/me, returning the authenticated user as JSON.
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":         u.ID,
		"github_id":  u.GitHubID,
		"login":      u.Login,
		"avatar_url": u.AvatarURL,
	})
}
```

`internal/api/server.go`:
```go
package api

import (
	"net/http"

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
	r.NotFound(web.Handler().ServeHTTP)

	s.router = r
	return s
}

// Router returns the chi router.
func (s *Server) Router() chi.Router { return s.router }

// ServeHTTP lets Server satisfy http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/api/ -v`
Expected: PASS (all four tests).

- [ ] **Step 7: Commit**

```bash
git add web/embed.go web/dist/index.html internal/api/
git commit -m "feat: api server, /api/me, embedded SPA fallback"
```

---

## Task 12: Wire main.go

**Files:**
- Modify: `cmd/server/main.go`

- [ ] **Step 1: Replace `cmd/server/main.go` with the real wiring**

```go
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
```

- [ ] **Step 2: Verify the whole module builds and all tests pass**

Run: `go build ./... && go test ./...`
Expected: build succeeds; all tests PASS.

- [ ] **Step 3: Commit**

```bash
git add cmd/server/main.go
git commit -m "feat: wire server entrypoint"
```

---

## Task 13: React + Vite frontend scaffold

**Files:**
- Create: `web/package.json`, `web/tsconfig.json`, `web/tsconfig.node.json`, `web/vite.config.ts`, `web/index.html`, `web/src/main.tsx`, `web/src/App.tsx`, `web/src/api.ts`

- [ ] **Step 1: Create `web/package.json`**

```json
{
  "name": "github-stats-web",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "preview": "vite preview"
  },
  "dependencies": {
    "react": "^18.3.1",
    "react-dom": "^18.3.1"
  },
  "devDependencies": {
    "@types/react": "^18.3.3",
    "@types/react-dom": "^18.3.0",
    "@vitejs/plugin-react": "^4.3.1",
    "typescript": "^5.5.4",
    "vite": "^5.4.2"
  }
}
```

- [ ] **Step 2: Create `web/tsconfig.json` and `web/tsconfig.node.json`**

`web/tsconfig.json`:
```json
{
  "compilerOptions": {
    "target": "ES2020",
    "useDefineForClassFields": true,
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "jsx": "react-jsx",
    "strict": true
  },
  "include": ["src"],
  "references": [{ "path": "./tsconfig.node.json" }]
}
```

`web/tsconfig.node.json`:
```json
{
  "compilerOptions": {
    "composite": true,
    "skipLibCheck": true,
    "module": "ESNext",
    "moduleResolution": "bundler",
    "allowSyntheticDefaultImports": true,
    "strict": true,
    "emitDeclarationOnly": true
  },
  "include": ["vite.config.ts"]
}
```

> Note: a `composite: true` project cannot also set `noEmit: true` under `tsc -b`
> (TS6310). Use `emitDeclarationOnly: true`; the emitted `*.tsbuildinfo` /
> `vite.config.d.ts` are gitignored (see Task 1's `.gitignore`).

- [ ] **Step 3: Create `web/vite.config.ts` with the dev proxy**

```ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// In dev, Vite serves the SPA on :5173 and proxies API/auth to the Go server.
export default defineConfig({
  plugins: [react()],
  build: { outDir: "dist", emptyOutDir: true },
  server: {
    port: 5173,
    proxy: {
      "/api": "http://localhost:8080",
      "/auth": "http://localhost:8080",
    },
  },
});
```

- [ ] **Step 4: Create `web/index.html`**

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>GitHub Stats</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

- [ ] **Step 5: Create `web/src/api.ts`**

```ts
export interface Me {
  id: number;
  github_id: number;
  login: string;
  avatar_url: string;
}

export async function fetchMe(): Promise<Me | null> {
  const res = await fetch("/api/me", { credentials: "same-origin" });
  if (res.status === 401) return null;
  if (!res.ok) throw new Error(`/api/me failed: ${res.status}`);
  return (await res.json()) as Me;
}
```

- [ ] **Step 6: Create `web/src/App.tsx`**

```tsx
import { useEffect, useState } from "react";
import { fetchMe, type Me } from "./api";

export default function App() {
  const [me, setMe] = useState<Me | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetchMe()
      .then(setMe)
      .finally(() => setLoading(false));
  }, []);

  if (loading) return <p>Loading…</p>;

  if (!me) {
    return (
      <main style={{ fontFamily: "system-ui", padding: 40 }}>
        <h1>GitHub Stats</h1>
        <a href="/auth/github">Sign in with GitHub</a>
      </main>
    );
  }

  return (
    <main style={{ fontFamily: "system-ui", padding: 40 }}>
      <h1>GitHub Stats</h1>
      <p>
        Signed in as <strong>{me.login}</strong>
        {me.avatar_url && (
          <img src={me.avatar_url} alt="" width={24} height={24}
               style={{ verticalAlign: "middle", marginLeft: 8, borderRadius: "50%" }} />
        )}
      </p>
      <a href="/auth/logout">Sign out</a>
    </main>
  );
}
```

- [ ] **Step 7: Create `web/src/main.tsx`**

```tsx
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
```

- [ ] **Step 8: Install deps and build the frontend**

Run: `cd web && npm install && npm run build`
Expected: `web/dist/` is regenerated with `index.html` + hashed `assets/`; no TypeScript errors.

- [ ] **Step 9: Verify the Go binary still builds with the real embed**

Run: `go build ./...`
Expected: succeeds (embeds the freshly built `dist/`).

- [ ] **Step 10: Commit**

```bash
git add web/package.json web/tsconfig.json web/tsconfig.node.json web/vite.config.ts web/index.html web/src/ web/package-lock.json
git commit -m "feat: react+vite frontend shell with login"
```

---

## Task 14: End-to-end manual smoke test + README

**Files:**
- Create: `README.md`

- [ ] **Step 1: Write `README.md` with self-host setup**

````markdown
# github-stats

Self-hosted GitHub statistics generator. Track public **and private** repo
analytics without GitHub premium.

## Setup (dev)

1. Register a GitHub OAuth App: https://github.com/settings/developers
   - Homepage URL: `http://localhost:8080`
   - Authorization callback URL: `http://localhost:8080/auth/github/callback`
2. `cp .env.example .env` and fill in `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`,
   and a key from `openssl rand -hex 32` as `ENCRYPTION_KEY`.
3. Run the API: `make dev-api` (serves on :8080)
4. In another terminal, run the frontend with HMR: `make dev-web` (serves on :5173,
   proxies `/api` and `/auth` to :8080). Open http://localhost:5173.

## Build (production single binary)

```bash
make build        # builds the React app, embeds it, compiles ./bin/github-stats
./bin/github-stats # serves API + SPA on :8080 from one binary
```
````

- [ ] **Step 2: Run the full test suite**

Run: `go test ./...`
Expected: all packages PASS.

- [ ] **Step 3: Manual smoke test (requires a real GitHub OAuth App)**

1. Build: `make build`
2. With `.env` populated, run `./bin/github-stats`.
3. Open `http://localhost:8080` → click "Sign in with GitHub" → authorize.
4. Expect redirect back to `/` showing "Signed in as <your-login>".
5. Confirm `app.db` exists and has a row: `sqlite3 app.db "SELECT login FROM users;"`.
6. Click "Sign out" → expect to return to the signed-out view.

Expected: full login round-trip works from the single binary.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: add setup and self-host instructions"
```

---

## Self-Review notes (already applied)

- **Spec coverage (M1 slice):** OAuth login (§9), encrypted token storage (§9), SQLite store + migrations (§5), embedded React + dev-proxy serving (§8), single-binary build (§8). Sync engine, metrics registry, and dashboard charts are **out of M1 scope** — they are M2–M5 and get their own plans.
- **Type consistency:** `Store`, `User`, `Session`, `Credential`, `OAuthClient{Exchange→(token,scope,err)}`, `Service`, `NewService`, `RequireUser`, `UserFromContext`, cookie names `gs_session`/`gs_oauth_state`, and `Server.Router()` are used identically across all tasks.
- **No placeholders:** every step ships complete, runnable code and exact commands.

## What M2 will add (next plan)

Collector (`githubapi`: GraphQL+REST, ETag conditional transport, dual rate-limit budget manager), the event/aggregate schema (`0002_*.sql`: repos, commits, pull_requests, issues, releases, daily_repo_stats, daily_contributor_stats, sync_jobs, sync_state, etags), and an end-to-end single-repo backfill writing daily aggregates.

## Post-review hardening (applied after milestone review)

Commit `fix: M1 security hardening (csrf entropy, secure-cookie, /api 404, env key)`:
1. `randomToken()` propagates the `crypto/rand` error and uses 32 bytes (was silently using an all-zero state token on entropy failure — the sole OAuth-callback CSRF defense).
2. `secureCookies(r)` derives the cookie `Secure` flag from `r.TLS` / `X-Forwarded-Proto` (falling back to the `BaseURL` scheme) instead of trusting only `BaseURL`.
3. Unknown `/api/*` paths return a JSON 404 (`{"error":"not found"}`) instead of the SPA HTML shell.
4. `.env.example` ships an empty `ENCRYPTION_KEY` so startup fails loudly rather than encrypting tokens under a public all-zero key.
5. Constant-time OAuth state comparison via `crypto/subtle`.

## Deferred review items (carry into later milestones)

- **M2**: verify/normalize the OAuth granted scope at exchange time; introduce a typed JSON response + shared `writeJSON` helper before handlers multiply; adopt `RETURNING id` and explicit (`errors.Is(sql.ErrNoRows)`) migration-runner error handling when `0002_*.sql` lands.
- **M3/M6**: session revocation ("log out everywhere" via `DeleteSessionsForUser`, the `idx_sessions_user` index already exists), idle/shorter TTL, and a periodic expired-session sweep.
- **M6 hardening**: GET→POST logout with a CSRF token.
