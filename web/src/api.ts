// Typed client for the github-stats JSON API. Same-origin in prod (embedded),
// proxied to :8080 in dev. Matches the M3/M4 wire contracts exactly.

// ---------------------------------------------------------------------------
// Shared
// ---------------------------------------------------------------------------

async function getJSON<T>(url: string): Promise<T> {
  const res = await fetch(url, { credentials: "same-origin" });
  if (!res.ok) throw await asError(res, url);
  return (await res.json()) as T;
}

async function asError(res: Response, url: string): Promise<Error> {
  let detail = "";
  try {
    detail = (await res.text()).trim();
  } catch {
    /* ignore */
  }
  return new Error(detail || `${url} failed: ${res.status}`);
}

/** Reads the non-httpOnly gs_csrf cookie value, or "" if absent. */
function readCsrfCookie(): string {
  const m = /(?:^|;\s*)gs_csrf=([^;]*)/.exec(document.cookie);
  return m ? decodeURIComponent(m[1]) : "";
}

/**
 * Returns the CSRF token to send as the X-CSRF-Token header on mutating
 * requests. Prefers the gs_csrf cookie (no network); falls back to
 * fetchCsrfToken() which sets the cookie and returns the token.
 */
async function csrfToken(): Promise<string> {
  const fromCookie = readCsrfCookie();
  if (fromCookie) return fromCookie;
  return fetchCsrfToken();
}

/** Builds request headers with the CSRF token attached for mutating calls. */
async function csrfHeaders(extra?: Record<string, string>): Promise<Record<string, string>> {
  return { ...(extra ?? {}), "X-CSRF-Token": await csrfToken() };
}

// ---------------------------------------------------------------------------
// Auth (M1)
// ---------------------------------------------------------------------------

export interface Me {
  id: number;
  github_id: number;
  login: string;
  avatar_url: string;
  scopes?: string;
  has_pat?: boolean;
}

export async function fetchMe(): Promise<Me | null> {
  const res = await fetch("/api/me", { credentials: "same-origin" });
  if (res.status === 401) return null;
  if (!res.ok) throw await asError(res, "/api/me");
  return (await res.json()) as Me;
}

// ---------------------------------------------------------------------------
// Repos (M3)
// ---------------------------------------------------------------------------

export type SyncStatus = "pending" | "running" | "complete" | "error" | "" | string;

export interface Repo {
  id: number;
  full_name: string;
  is_private: boolean;
  default_branch: string;
  description?: string;
  stargazers?: number;
  forks?: number;
  sync_status: SyncStatus;
  last_synced_at: string | null;
}

export function listRepos(): Promise<Repo[]> {
  return getJSON<Repo[]>("/api/repos");
}

export async function addRepo(fullName: string): Promise<Repo> {
  const res = await fetch("/api/repos", {
    method: "POST",
    credentials: "same-origin",
    headers: await csrfHeaders({ "Content-Type": "application/json" }),
    body: JSON.stringify({ full_name: fullName }),
  });
  if (!res.ok) throw await asError(res, "/api/repos");
  return (await res.json()) as Repo;
}

export async function deleteRepo(id: number): Promise<void> {
  const res = await fetch(`/api/repos/${id}`, {
    method: "DELETE",
    credentials: "same-origin",
    headers: await csrfHeaders(),
  });
  if (!res.ok && res.status !== 204) throw await asError(res, `/api/repos/${id}`);
}

export async function refreshRepo(id: number): Promise<void> {
  const res = await fetch(`/api/repos/${id}/refresh`, {
    method: "POST",
    credentials: "same-origin",
    headers: await csrfHeaders(),
  });
  if (!res.ok && res.status !== 202) throw await asError(res, `/api/repos/${id}/refresh`);
}

// ---------------------------------------------------------------------------
// Metrics / overview / latest (M4)
// ---------------------------------------------------------------------------

export type WindowSpec = "30d" | "90d" | "6m" | "1y" | "all";

export interface QueryOpts {
  window: WindowSpec;
  excludeBots: boolean;
}

export interface MetricsOpts extends QueryOpts {
  keys: string[];
}

/** Result is the tagged union M4 stamps with `kind`. */
export type Result = TimeSeriesResult | ScalarResult | BucketsResult | LeaderboardResult;

export interface SeriesPoint {
  date: string;
  value: number;
}
export interface TimeSeriesResult {
  kind: "time_series";
  label?: string;
  series: SeriesPoint[];
}

export interface ScalarResult {
  kind: "scalar";
  label?: string;
  value?: number;
  unit?: string;
  count?: number;
}

export interface BucketRow {
  label: string;
  count: number;
}
export interface BucketsResult {
  kind: "buckets";
  label?: string;
  buckets: BucketRow[];
}

export interface LeaderRow {
  login: string;
  commits: number;
  additions: number;
  deletions: number;
}
export interface LeaderboardResult {
  kind: "leaderboard";
  label?: string;
  rows: LeaderRow[];
}

export type MetricsMap = Record<string, Result>;

export interface Overview {
  id: number;
  full_name: string;
  is_private: boolean;
  default_branch: string;
  description: string;
  stargazers: number;
  forks: number;
  open_issues: number;
  open_prs: number;
  contributors: number;
  commit_rate: number;
  issue_rate: number;
  pr_rate: number;
  releases: number;
  sync_status: SyncStatus;
  last_synced_at: string | null;
  window_from: string;
  window_to: string;
}

export interface LatestCommit {
  sha: string;
  author_login: string;
  committed_at: string;
  additions: number;
  deletions: number;
  is_bot: boolean;
  msg_first_line: string;
}
export interface LatestPR {
  number: number;
  author_login: string;
  state: string;
  created_at: string;
  merged_at: string | null;
  closed_at: string | null;
  comments_count: number;
  is_bot: boolean;
  title: string;
}
export interface LatestIssue {
  number: number;
  author_login: string;
  state: string;
  created_at: string;
  closed_at: string | null;
  comments_count: number;
  is_bot: boolean;
  title: string;
}
export type LatestKind = "commits" | "prs" | "issues";
export type LatestItem = LatestCommit | LatestPR | LatestIssue;

// --- URL builders (pure; unit-tested) -------------------------------------

export function metricsURL(repoID: number, o: MetricsOpts): string {
  const q = new URLSearchParams();
  if (o.keys.length > 0) q.set("keys", o.keys.join(","));
  q.set("window", o.window);
  q.set("exclude_bots", String(o.excludeBots));
  return `/api/repos/${repoID}/metrics?${q.toString()}`;
}

export function overviewURL(repoID: number, o: QueryOpts): string {
  const q = new URLSearchParams();
  q.set("window", o.window);
  q.set("exclude_bots", String(o.excludeBots));
  return `/api/repos/${repoID}?${q.toString()}`;
}

export function latestURL(repoID: number, kind: LatestKind, limit: number): string {
  return `/api/repos/${repoID}/latest/${kind}?limit=${limit}`;
}

// --- fetch wrappers --------------------------------------------------------

export function fetchMetrics(repoID: number, o: MetricsOpts): Promise<MetricsMap> {
  return getJSON<MetricsMap>(metricsURL(repoID, o));
}

export function fetchOverview(repoID: number, o: QueryOpts): Promise<Overview> {
  return getJSON<Overview>(overviewURL(repoID, o));
}

export function fetchLatest(repoID: number, kind: LatestKind, limit = 20): Promise<LatestItem[]> {
  return getJSON<LatestItem[]>(latestURL(repoID, kind, limit));
}

// ---------------------------------------------------------------------------
// Sync SSE stream (M3)
// ---------------------------------------------------------------------------

export interface SyncEvent {
  repo_id: number;
  phase: string; // "backfill" | "delta" | "commits" | "prs" | "issues" | "releases" | "done" | "error"
  message: string;
  done: boolean;
}

export interface SyncStreamHandlers {
  onEvent: (ev: SyncEvent) => void;
  onDone?: (ev: SyncEvent) => void;
  onError?: (e: Event) => void;
}

export interface SyncStreamHandle {
  close: () => void;
}

/**
 * openSyncStream subscribes to GET /api/repos/{id}/sync/stream. Each `data:`
 * frame is parsed into a SyncEvent and passed to onEvent. On a terminal event
 * (done === true) it calls onDone and closes the EventSource automatically.
 * The returned handle's close() unsubscribes early (e.g. on unmount).
 */
export function openSyncStream(repoID: number, h: SyncStreamHandlers): SyncStreamHandle {
  const es = new EventSource(`/api/repos/${repoID}/sync/stream`);
  es.onmessage = (e: MessageEvent) => {
    let ev: SyncEvent;
    try {
      ev = JSON.parse(e.data) as SyncEvent;
    } catch {
      return;
    }
    h.onEvent(ev);
    if (ev.done) {
      h.onDone?.(ev);
      es.close();
    }
  };
  es.onerror = (e: Event) => {
    h.onError?.(e);
  };
  return { close: () => es.close() };
}

// ---------------------------------------------------------------------------
// Collections (M6)
// ---------------------------------------------------------------------------

export interface Collection {
  id: number;
  name: string;
  repo_ids: number[];
}

export function listCollections(): Promise<Collection[]> {
  return getJSON<Collection[]>("/api/collections");
}

export async function createCollection(name: string): Promise<Collection> {
  const res = await fetch("/api/collections", {
    method: "POST",
    credentials: "same-origin",
    headers: await csrfHeaders({ "Content-Type": "application/json" }),
    body: JSON.stringify({ name }),
  });
  if (!res.ok) throw await asError(res, "/api/collections");
  return (await res.json()) as Collection;
}

export async function renameCollection(id: number, name: string): Promise<void> {
  const res = await fetch(`/api/collections/${id}`, {
    method: "PATCH",
    credentials: "same-origin",
    headers: await csrfHeaders({ "Content-Type": "application/json" }),
    body: JSON.stringify({ name }),
  });
  if (!res.ok) throw await asError(res, `/api/collections/${id}`);
}

export async function deleteCollection(id: number): Promise<void> {
  const res = await fetch(`/api/collections/${id}`, {
    method: "DELETE",
    credentials: "same-origin",
    headers: await csrfHeaders(),
  });
  if (!res.ok && res.status !== 204) throw await asError(res, `/api/collections/${id}`);
}

export async function addRepoToCollection(collectionID: number, repoID: number): Promise<void> {
  const res = await fetch(`/api/collections/${collectionID}/repos/${repoID}`, {
    method: "POST",
    credentials: "same-origin",
    headers: await csrfHeaders(),
  });
  if (!res.ok && res.status !== 204) {
    throw await asError(res, `/api/collections/${collectionID}/repos/${repoID}`);
  }
}

export async function removeRepoFromCollection(collectionID: number, repoID: number): Promise<void> {
  const res = await fetch(`/api/collections/${collectionID}/repos/${repoID}`, {
    method: "DELETE",
    credentials: "same-origin",
    headers: await csrfHeaders(),
  });
  if (!res.ok && res.status !== 204) {
    throw await asError(res, `/api/collections/${collectionID}/repos/${repoID}`);
  }
}

export function exportCollectionURL(id: number): string {
  return `/api/collections/${id}/export`;
}

// ---------------------------------------------------------------------------
// Import (M6)
// ---------------------------------------------------------------------------

export interface ImportResult {
  resolved: string[];
  unresolved: string[];
}

export async function importManifest(
  kind: "package_json" | "requirements_txt" | "collection",
  body: string,
): Promise<ImportResult> {
  const res = await fetch(`/api/import?kind=${kind}`, {
    method: "POST",
    credentials: "same-origin",
    headers: await csrfHeaders(),
    body,
  });
  if (!res.ok) throw await asError(res, "/api/import");
  return (await res.json()) as ImportResult;
}

// ---------------------------------------------------------------------------
// PAT settings (M6)
// ---------------------------------------------------------------------------

export interface PatStatus {
  has_pat: boolean;
  login?: string;
}

export function getPatStatus(): Promise<PatStatus> {
  return getJSON<PatStatus>("/api/settings/pat");
}

export async function savePat(token: string): Promise<PatStatus> {
  const res = await fetch("/api/settings/pat", {
    method: "PUT",
    credentials: "same-origin",
    headers: await csrfHeaders({ "Content-Type": "application/json" }),
    body: JSON.stringify({ token }),
  });
  if (!res.ok) throw await asError(res, "/api/settings/pat");
  return (await res.json()) as PatStatus;
}

export async function deletePat(): Promise<void> {
  const res = await fetch("/api/settings/pat", {
    method: "DELETE",
    credentials: "same-origin",
    headers: await csrfHeaders(),
  });
  if (!res.ok && res.status !== 204) throw await asError(res, "/api/settings/pat");
}

// ---------------------------------------------------------------------------
// Rate limit (M6)
// ---------------------------------------------------------------------------

export interface RateBucket {
  remaining: number;
  reset: string;
}
export interface RateLimit {
  rest: RateBucket;
  graphql: RateBucket;
}

export function fetchRateLimit(): Promise<RateLimit> {
  return getJSON<RateLimit>("/api/rate-limit");
}

// ---------------------------------------------------------------------------
// CSRF + logout (M6)
// ---------------------------------------------------------------------------

export async function fetchCsrfToken(): Promise<string> {
  const res = await fetch("/api/csrf", { credentials: "same-origin" });
  if (!res.ok) throw await asError(res, "/api/csrf");
  return ((await res.json()) as { csrf_token: string }).csrf_token;
}

export async function logout(everywhere = false): Promise<void> {
  const path = everywhere ? "/auth/logout/all" : "/auth/logout";
  const res = await fetch(path, {
    method: "POST",
    credentials: "same-origin",
    headers: await csrfHeaders(),
  });
  if (!res.ok && res.status !== 204) throw await asError(res, path);
}
