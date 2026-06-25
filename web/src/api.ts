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

// ---------------------------------------------------------------------------
// Auth (M1)
// ---------------------------------------------------------------------------

export interface Me {
  id: number;
  github_id: number;
  login: string;
  avatar_url: string;
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
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ full_name: fullName }),
  });
  if (!res.ok) throw await asError(res, "/api/repos");
  return (await res.json()) as Repo;
}

export async function deleteRepo(id: number): Promise<void> {
  const res = await fetch(`/api/repos/${id}`, {
    method: "DELETE",
    credentials: "same-origin",
  });
  if (!res.ok && res.status !== 204) throw await asError(res, `/api/repos/${id}`);
}

export async function refreshRepo(id: number): Promise<void> {
  const res = await fetch(`/api/repos/${id}/refresh`, {
    method: "POST",
    credentials: "same-origin",
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
