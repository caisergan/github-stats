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
