# GitHub Stats — Design Spec

**Date:** 2026-05-30
**Status:** Approved design, pending spec review
**Author:** brainstormed with Claude

## 1. Summary

A self-hosted, modular GitHub statistics generator that lets users track analytics
for their repositories — **including private repos** — without paying for GitHub's
premium tiers. Inspired by [githubtracker.com](https://githubtracker.com/), but
engineered to stay fast and within rate limits on the **largest repositories**
(thousands of commits, PRs, issues, and comments).

The differentiator is the **backend**: rather than fetching live from GitHub on every
page load (what the reference does), this app does a one-time **incremental backfill**
into a local store, maintains **precomputed daily aggregates**, and serves the dashboard
entirely from the database. The result is a dashboard that is instant regardless of repo
size, and a sync engine that respects — and routes around — GitHub's API rate limits.

## 2. Goals & Non-Goals

### Goals
- Track public **and private** repo stats without GitHub premium.
- Stay performant and within rate limits on the biggest repos.
- Modular architecture: each statistic is a self-contained, testable unit.
- Self-hostable as a **single binary** (backend + embedded frontend).
- GitHub OAuth login; optional PAT as an alternate credential.
- Extended dev-analytics metrics (beyond the reference's basic counts).
- Background sync (warm data) + on-demand "refresh now".

### Non-Goals (YAGNI)
- Multi-tenant SaaS machinery: billing, plans, public sign-up funnel.
- GitLab/Bitbucket providers (architecture leaves room; not built now).
- Per-file / per-line diff breakdowns ("Deep" tier) — Extended tier only.
- Storing full comment bodies — we store counts/timestamps, not text.
- A GitHub App auth model — documented as a future upgrade, not built now.

## 3. Decisions (locked)

| Area | Decision |
|------|----------|
| Deployment | Self-hosted, lightweight. GitHub OAuth login; multi-user-capable but no billing/SaaS machinery. |
| GitHub access | OAuth Authorization-Code flow. Optional fine-grained PAT as alternate credential. |
| Backend | Go (Chi router, stdlib-centric). |
| Store | SQLite (pure-Go `modernc.org/sqlite`), WAL mode. Single file, no cgo. |
| Frontend | React + Vite SPA, embedded into the Go binary via `embed.FS`. Charts via uPlot (ECharts optional for richer types). |
| Metrics depth | Extended dev-analytics. |
| Sync model | Both: scheduled background sync + on-demand refresh. |
| Core data approach | Approach B — persistent store + incremental sync + precomputed aggregates. |

### Rate-limit reality (grounds the optimization plan)
- GitHub's **5,000 req/hr is a single per-user bucket**, shared across the user's OAuth
  authorization **and** all their PATs. → A PAT **cannot** buy more headroom; it is repurposed
  here as an alternate credential (headless/self-host setups, org repos an OAuth app isn't
  approved for), **not** a rate-limit bypass.
- **Conditional requests** (`If-None-Match` → `304`) **do not count** against the rate limit.
- **GraphQL** has a **separate** 5,000-**points**/hr bucket (2,000 pts/min vs REST's 900) and
  one query can replace dozens of REST calls.
- The only true way past per-user 5,000/hr is a **GitHub App** (per-installation pools) —
  noted as a future upgrade.

## 4. Architecture

### Module map (Go packages)
```
web/        embedded React dashboard (charts)
api/        Chi HTTP server · JSON API · SSE sync-progress
auth/       GitHub OAuth flow · sessions · encrypted token storage
sync/       scheduler · backfill+delta orchestration · worker pool · rate-limit budget manager
metrics/    registry of pluggable stat generators  ← "modular statistics generator"
githubapi/  REST + GraphQL clients · ETag conditional transport · retry/backoff · pagination
store/      SQLite · migrations · DAOs · raw events + precomputed daily aggregates
config/     env/file config · secrets · OAuth creds
```

Boundaries: `metrics` reads only from `store` (never touches GitHub or HTTP). `sync`
orchestrates but delegates fetching to `githubapi` and writing to `store`. A future
provider (GitLab) is a new collector behind the same `githubapi` interface.

### Data flow
```
OAuth login → token (AES-GCM encrypted) ─┐
                                         ▼
add repo ──► enqueue sync job ──► sync engine
                                   │ backfill: GraphQL paginated history ─► store (events + aggregates)
                                   │ delta:    since-cursor + ETag 304s ──► store
                                   ▼
dashboard ◄── JSON API ◄── metrics (reads aggregates) ◄── store   (no GitHub call on view)
"Refresh now" ──► immediate delta sync ──► SSE progress ──► UI
```

## 5. Data model (SQLite, WAL)

**Identity & access**
- `users` (id, github_id, login, avatar_url, created_at)
- `credentials` (user_id, kind `oauth|pat`, enc_token, scopes, created_at)
- `sessions` (id, user_id, expires_at)
- `etags` (user_id, url, etag, last_modified)

**Tracking**
- `repos` (id, github_id, full_name, is_private, default_branch, …)
- `collections` (id, user_id, name) · `collection_repos` (collection_id, repo_id)
- `repo_tracking` (user_id, repo_id)

**Event tables (lean — source of truth)**
- `commits` (repo_id, sha, author_login, committed_at, additions, deletions, is_bot, msg_first_line)
- `pull_requests` (repo_id, number, author_login, state, created_at, merged_at, closed_at,
  additions, deletions, changed_files, comments_count, first_review_at, is_bot, title)
- `issues` (repo_id, number, author_login, state, created_at, closed_at, comments_count, is_bot, title)
- `releases` (repo_id, tag, name, published_at, author_login)

**Materialized aggregates (charts read these)**
- `daily_repo_stats` PK(repo_id, date): commits, additions, deletions, prs_opened, prs_merged,
  prs_closed, issues_opened, issues_closed, comments, releases, active_contributors
- `daily_contributor_stats` PK(repo_id, date, login): commits, additions, deletions

**Sync bookkeeping**
- `sync_jobs` (id, repo_id, kind `backfill|delta`, status, cursor_state JSON, attempts, next_run_at, locked_at)
- `sync_state` (repo_id, last_commit_at, last_pr_cursor, last_issue_cursor, last_backfill_at, status)

Events are the **source of truth**; aggregates are recomputed for any date range a sync
touches, so late edits (a PR merging, an issue closing) stay correct.

## 6. Sync engine

- **Scheduler**: 1-min ticker → repos with `next_run_at ≤ now` get a `delta` job enqueued.
  Adding a repo enqueues a `backfill` job.
- **Worker pool**: bounded concurrency (config, default ~4–8). Each worker leases a job
  (`locked_at`), runs it, releases.
- **Budget manager** (shared): tracks REST (5,000/hr) and GraphQL (5,000 pts/hr) remaining
  from response headers. Workers request budget before a call; on exhaustion the job **saves
  its cursor and reschedules at the reset time**. Secondary limits (`403/429 + Retry-After`)
  → exponential backoff + concurrency throttle.
- **Backfill** (GraphQL, 100/page, cursor-paged): commit `history` (additions/deletions per
  node), `pullRequests` (`comments.totalCount`, first review for latency), `issues`, releases.
  Batched transaction inserts; aggregates upserted as pages land. **Resumable** from saved
  cursor across rate-limit windows.
- **Delta**: `orderBy UPDATED_AT DESC`, stop at `last_synced_at` minus a small overlap window
  (catches edits); commits via `history(since:)`. REST polls (releases, repo meta) use ETags
  → `304` is free. Touched dates get aggregates recomputed from events.

## 7. Metrics registry — the modular statistics generator

```go
type Metric interface {
    Key() string                                               // "time_to_merge"
    Compute(ctx, store, repoID, Window, opts) (Result, error)  // reads ONLY from store
}
```

A `Registry` maps `key → Metric`. API: `GET /api/repos/{id}/metrics?keys=…&window=30d&exclude_bots=true`.
Each metric is its own file, independently testable, ignorant of GitHub/HTTP. Adding a stat =
one new file + register.

**Extended set shipped:** `commit_rate`, `pr_throughput`, `time_to_merge`, `review_latency`,
`issue_lifetime`, `open_issue_age` (buckets), `code_churn`, `comment_volume`,
`contributor_leaderboard`, plus an `ema` helper (5d/14d smoothing).

**Result shapes (~4):** time-series (date→value), scalar, distribution/buckets, leaderboard rows.
The frontend has one renderer per shape.

`exclude_bots` toggles use the `is_bot` flag (login ends in `[bot]` or matches a known list).

## 8. Frontend

React + Vite SPA, built and embedded via `embed.FS` (single artifact). uPlot for heavy
time-series; ECharts optional for richer chart types.

- **Overview**: collections + repo cards (key metrics at a glance).
- **Repo detail**: window selector (30d/90d/6m/1y/all) · exclude-bots toggle · sections
  (Details · Insights · Commits · Issues · PRs · Contributors · Releases) with charts + latest
  lists · "Refresh now" with live SSE progress.
- **URL shortcut** `/owner/repo` → repo detail (the GithubTracker nicety).

### Serving model (dev vs prod)

The React SPA is always a **decoupled client** that talks to the Go backend purely over the
HTTP/JSON API — `embed.FS` is a *packaging/serving* choice, not an architectural coupling. The
same API boundary holds in both environments; only **who serves the static bundle** differs.

- **Development**: Vite dev server runs on its own port with HMR; it **proxies `/api` → the Go
  backend** (`server.proxy` in `vite.config.ts`). Two processes, instant frontend feedback, no
  Go rebuild on UI changes. Fully decoupled DX.
- **Production**: `npm run build` → the emitted `dist/` is embedded via `go:embed` → `go build`
  produces a **single binary** that serves both `/api/*` (JSON) and `/` + `/assets/*` (the SPA).
  Build pipeline order (Makefile/Taskfile): build frontend → embed → build Go.

**Why embed for self-hosted**: one artifact to ship and run (no Node runtime in prod), **same
origin → no CORS**, httpOnly session cookies work cleanly, and frontend/backend versions can
never skew. This is the standard Go-single-binary pattern (Gitea, Grafana, Syncthing). The
separate-host model (UI on a CDN/Vercel, API elsewhere) is only worth its CORS + dual-deploy
cost at SaaS/CDN scale, which is out of scope — so it is **not** used here. A SPA-fallback route
serves `index.html` for client-side routes (e.g. `/owner/repo`) while `/api/*` is handled first.

## 9. Auth & token handling

GitHub OAuth Authorization-Code flow. Minimal scopes by default (`read:user` + `public_repo`),
**escalating to `repo` only when the user opts into private tracking**. Sessions = httpOnly
cookie → DB. Tokens **AES-GCM encrypted at rest** (key from config). Self-host setup: user
registers their own GitHub OAuth App and supplies `client_id/secret` via config (documented).
Optional fine-grained **PAT** in settings as an alternate credential.

## 10. API surface (sketch)

- `GET /auth/github` · `GET /auth/github/callback`
- `GET /api/me`
- `GET/POST /api/collections` · `DELETE /api/collections/{id}`
- `GET/POST /api/repos` · `DELETE /api/repos/{id}`
- `GET /api/repos/{id}` (overview metrics bundle)
- `GET /api/repos/{id}/metrics?keys=&window=&exclude_bots=`
- `GET /api/repos/{id}/latest/{commits|prs|issues}`
- `POST /api/repos/{id}/refresh` (trigger delta sync)
- `GET /api/repos/{id}/sync/stream` (SSE progress)

## 11. Build milestones (each independently shippable)

1. **M1 — Skeleton & auth**: project, config, SQLite + migrations, OAuth login, sessions,
   embedded frontend shell (walking skeleton).
2. **M2 — Collector + single-repo backfill**: `githubapi` (GraphQL+REST, ETag transport, budget
   manager), event schema, end-to-end backfill of one repo → aggregates.
3. **M3 — Sync engine**: scheduler, job queue, delta sync, resumability, worker pool, refresh + SSE.
4. **M4 — Metrics registry**: Extended metric set as modular units + metrics API.
5. **M5 — Dashboard UI**: overview + repo detail, uPlot charts, toggles, URL shortcut, sync status.
6. **M6 — Polish & hardening**: collections, save/load, bot detection, rate-limit UX, self-host
   docs, optional PAT.

## 12. Key risks & mitigations

- **Rate-limit exhaustion on first backfill of a huge repo** → resumable cursor-based backfill;
  GraphQL + dual-budget scheduling; ETag-conditioned deltas.
- **Aggregate drift from late edits** → events are source of truth; aggregates recomputed for
  touched date ranges.
- **Token leakage** → AES-GCM encryption at rest, httpOnly sessions, minimal OAuth scopes.
- **SQLite write contention under concurrent syncs** → WAL mode, batched transactions, bounded
  worker concurrency. (Postgres is the escape hatch if many concurrent users emerge.)
