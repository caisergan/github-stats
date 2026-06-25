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

## Production deployment

Build the single binary (`make build`) and run it behind a TLS-terminating reverse
proxy. The binary serves both the JSON API and the embedded SPA on `ADDR`.

### Reverse proxy + TLS (so Secure cookies engage)

Session and CSRF cookies are flagged `Secure` only when the request is HTTPS. The
server derives this from `r.TLS` **or** the `X-Forwarded-Proto` header (falling back
to the `BASE_URL` scheme). Terminate TLS at the proxy and forward the scheme:

```nginx
server {
  listen 443 ssl;
  server_name stats.example.com;
  ssl_certificate     /etc/letsencrypt/live/stats.example.com/fullchain.pem;
  ssl_certificate_key /etc/letsencrypt/live/stats.example.com/privkey.pem;

  location / {
    proxy_pass         http://127.0.0.1:8080;
    proxy_set_header   Host              $host;
    proxy_set_header   X-Forwarded-Proto $scheme;   # <-- engages the Secure cookie flag
    proxy_set_header   X-Forwarded-For   $remote_addr;
  }
}
```

Set `BASE_URL=https://stats.example.com` and register that callback
(`https://stats.example.com/auth/github/callback`) in your GitHub OAuth App.

### Backing up the database

State lives in one SQLite file (WAL mode), so a backup must include the WAL/SHM
sidecars or use SQLite's online backup. Simplest safe options:

```bash
# Consistent online backup (preferred; no downtime):
sqlite3 app.db ".backup '/backups/app-$(date +%F).db'"

# Or stop the binary, then copy all three files together:
cp app.db app.db-wal app.db-shm /backups/
```

Restore by stopping the binary and putting the `.db` (and any `-wal`/`-shm`) back.

### Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ADDR` | `:8080` | Listen address. |
| `DATABASE_PATH` | `app.db` | SQLite file path. |
| `BASE_URL` | `http://localhost:8080` | Public origin; derives the OAuth callback + cookie scheme. |
| `GITHUB_CLIENT_ID` | — (required) | OAuth App client id. |
| `GITHUB_CLIENT_SECRET` | — (required) | OAuth App client secret. |
| `GITHUB_SCOPES` | `read:user public_repo` | Requested scopes (`repo` enables private tracking). |
| `ENCRYPTION_KEY` | — (required) | 32-byte key, hex-encoded (64 chars). `openssl rand -hex 32`. |
| `GITHUB_OAUTH_BASE_URL` | `https://github.com` | Override for testing/GHE. |
| `GITHUB_API_BASE_URL` | `https://api.github.com` | Override for testing/GHE. |

### Optional PAT and rate limits (honest note)

A fine-grained PAT (Settings page) is an **alternate credential** — useful for
headless setups or org repos your OAuth app isn't approved for. It does **not** raise
GitHub's per-user **5,000 requests/hour** limit: that bucket is shared across your
OAuth authorization and all your PATs. The app already minimizes usage via GraphQL
(separate 5,000-point bucket) and conditional `ETag`/`304` requests (which don't
count). **The only way past the per-user limit is a GitHub App** (per-installation
pools) — a documented future upgrade, not built here.
