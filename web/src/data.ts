// seeded PRNG (mulberry32)
export function rng(seed: number) {
  let a = seed >>> 0;
  return function () {
    a |= 0;
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

export function isoDaysAgo(n: number): string {
  const d = new Date("2026-05-31T12:00:00Z");
  d.setUTCDate(d.getUTCDate() - n);
  return d.toISOString();
}

export function dateDaysAgo(n: number): string {
  return isoDaysAgo(n).slice(0, 10);
}

export interface SeriesPoint {
  date: string;
  value: number;
}

export interface TimeSeriesResult {
  kind: "time_series";
  label?: string;
  unit?: string;
  series: SeriesPoint[];
  _area?: boolean;
}

export interface ScalarResult {
  kind: "scalar";
  label?: string;
  value: number;
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
  img: string;
  commits: number;
  additions: number;
  deletions: number;
}

export interface LeaderboardResult {
  kind: "leaderboard";
  label?: string;
  rows: LeaderRow[];
}

export type MockResult = TimeSeriesResult | ScalarResult | BucketsResult | LeaderboardResult;

export interface MockMetricsMap {
  commit_rate: TimeSeriesResult;
  pr_throughput: TimeSeriesResult;
  code_churn: TimeSeriesResult;
  comment_volume: TimeSeriesResult;
  time_to_merge: ScalarResult;
  review_latency: ScalarResult;
  issue_lifetime: ScalarResult;
  open_issue_age: BucketsResult;
  contributor_leaderboard: LeaderboardResult;
  heatmap: number[][];
}

// build a daily time series for `days`, with seed + base level + weekly rhythm
export function series(
  seed: number,
  days: number,
  base: number,
  spread: number,
  weekendDrop: boolean,
): SeriesPoint[] {
  const r = rng(seed);
  const out: SeriesPoint[] = [];
  let level = base;
  for (let i = days - 1; i >= 0; i--) {
    const date = new Date("2026-05-31T00:00:00Z");
    date.setUTCDate(date.getUTCDate() - i);
    const dow = date.getUTCDay();
    level += (r() - 0.48) * spread;
    level = Math.max(0, level);
    let v = level + (r() - 0.5) * spread * 0.8;
    if (weekendDrop && (dow === 0 || dow === 6)) {
      v *= 0.25 + r() * 0.2;
    }
    out.push({
      date: date.toISOString().slice(0, 10),
      value: Math.max(0, Math.round(v * 10) / 10),
    });
  }
  return out;
}

export interface MockMe {
  id: number;
  github_id: number;
  login: string;
  name: string;
  avatar_url: string;
}

export const ME: MockMe = {
  id: 1,
  github_id: 583231,
  login: "maya-dev",
  name: "Maya Chen",
  avatar_url: "https://i.pravatar.cc/96?img=47",
};

export interface MockContributor {
  login: string;
  img: string;
  bot?: boolean;
}

// contributor pool
export const PEOPLE: MockContributor[] = [
  { login: "maya-dev", img: "https://i.pravatar.cc/48?img=47" },
  { login: "tobias-w", img: "https://i.pravatar.cc/48?img=12" },
  { login: "priya.k", img: "https://i.pravatar.cc/48?img=32" },
  { login: "deepak-r", img: "https://i.pravatar.cc/48?img=15" },
  { login: "sofia-lng", img: "https://i.pravatar.cc/48?img=45" },
  { login: "jon-h", img: "https://i.pravatar.cc/48?img=8" },
  { login: "ling-x", img: "https://i.pravatar.cc/48?img=24" },
  { login: "dependabot[bot]", img: "", bot: true },
  { login: "renovate[bot]", img: "", bot: true },
];

const COMMIT_MSGS = [
  "fix: handle null cursor in pagination loop",
  "feat: add exclude-bots toggle to metrics query",
  "refactor: extract SSE handler into hook",
  "perf: batch insert commits in single tx",
  "chore: bump uplot to 1.6.31",
  "fix: window selector resets exclude_bots",
  "docs: document /api/repos/{id}/metrics keys",
  "test: cover owner/repo resolver casing",
  "feat: contribution heatmap on repo detail",
  "fix: tabular nums on leaderboard totals",
  "style: tighten card padding at compact density",
  "feat: live progress log for refresh stream",
];

const PR_TITLES = [
  "Add metrics registry kind-switching",
  "Wire BrowserRouter + deep-link fallback",
  "Race-safe useAsync hook",
  "Embed dist into Go binary",
  "Buckets renderer with CSS bars",
  "Leaderboard table + tabular nums",
  "Window controls + exclude-bots state",
  "SSE refresh button with live log",
];

const ISSUE_TITLES = [
  "Charts overflow container on narrow viewports",
  "Heatmap tooltip lags on rapid hover",
  "exclude_bots not persisted across reloads",
  "Add 6-month window option",
  "Leaderboard should link to contributor profile",
  "Dark theme: progress log contrast too low",
  "Release section empty for forks",
];

export interface MockCommit {
  sha: string;
  author_login: string;
  author_img: string;
  committed_at: string;
  additions: number;
  deletions: number;
  is_bot: boolean;
  msg_first_line: string;
}

export function latestCommits(seed: number, n: number): MockCommit[] {
  const r = rng(seed);
  const out: MockCommit[] = [];
  for (let i = 0; i < n; i++) {
    const p = PEOPLE[Math.floor(r() * (PEOPLE.length - 1))];
    out.push({
      sha: Math.floor(r() * 0xfffffff)
        .toString(16)
        .padStart(7, "0")
        .slice(0, 7),
      author_login: p.login,
      author_img: p.img,
      committed_at: isoDaysAgo(Math.floor(r() * 9) + i * 0.3),
      additions: Math.floor(r() * 240) + 2,
      deletions: Math.floor(r() * 120),
      is_bot: !!p.bot,
      msg_first_line: COMMIT_MSGS[Math.floor(r() * COMMIT_MSGS.length)],
    });
  }
  return out.sort((a, b) => b.committed_at.localeCompare(a.committed_at));
}

export interface MockPR {
  number: number;
  author_login: string;
  author_img: string;
  state: string;
  created_at: string;
  merged_at: string | null;
  closed_at: string | null;
  comments_count: number;
  is_bot: boolean;
  title: string;
}

export function latestPRs(seed: number, n: number): MockPR[] {
  const r = rng(seed);
  const out: MockPR[] = [];
  for (let i = 0; i < n; i++) {
    const p = PEOPLE[Math.floor(r() * (PEOPLE.length - 1))];
    const states = ["merged", "merged", "open", "merged", "closed"];
    const state = states[Math.floor(r() * states.length)];
    const created = Math.floor(r() * 14) + i;
    out.push({
      number: 480 - i * 3 - Math.floor(r() * 2),
      author_login: p.login,
      author_img: p.img,
      state,
      created_at: isoDaysAgo(created),
      merged_at: state === "merged" ? isoDaysAgo(created - 1) : null,
      closed_at: state === "closed" ? isoDaysAgo(created - 1) : null,
      comments_count: Math.floor(r() * 12),
      is_bot: !!p.bot,
      title: PR_TITLES[Math.floor(r() * PR_TITLES.length)],
    });
  }
  return out;
}

export interface MockIssue {
  number: number;
  author_login: string;
  author_img: string;
  state: string;
  created_at: string;
  closed_at: string | null;
  comments_count: number;
  is_bot: boolean;
  title: string;
}

export function latestIssues(seed: number, n: number): MockIssue[] {
  const r = rng(seed);
  const out: MockIssue[] = [];
  for (let i = 0; i < n; i++) {
    const p = PEOPLE[Math.floor(r() * (PEOPLE.length - 1))];
    const open = r() > 0.45;
    const created = Math.floor(r() * 20) + i;
    out.push({
      number: 312 - i * 2 - Math.floor(r() * 2),
      author_login: p.login,
      author_img: p.img,
      state: open ? "open" : "closed",
      created_at: isoDaysAgo(created),
      closed_at: open ? null : isoDaysAgo(Math.max(0, created - Math.floor(r() * 6) - 1)),
      comments_count: Math.floor(r() * 9),
      is_bot: !!p.bot,
      title: ISSUE_TITLES[Math.floor(r() * ISSUE_TITLES.length)],
    });
  }
  return out;
}

export function leaderboard(seed: number): LeaderRow[] {
  const r = rng(seed);
  const rows = PEOPLE.filter((p) => !p.bot).map((p) => ({
    login: p.login,
    img: p.img,
    commits: Math.floor(r() * 180) + 8,
    additions: Math.floor(r() * 14000) + 200,
    deletions: Math.floor(r() * 8000) + 100,
  }));
  return rows.sort((a, b) => b.commits - a.commits);
}

// contribution heatmap: 53 weeks * 7 days of intensity counts
export function heatmap(seed: number): number[][] {
  const r = rng(seed);
  const weeks: number[][] = [];
  for (let w = 0; w < 53; w++) {
    const days: number[] = [];
    for (let d = 0; d < 7; d++) {
      const weekend = d === 0 || d === 6;
      let base = r();
      if (weekend) base *= 0.4;
      const count = base < 0.22 ? 0 : Math.floor(base * 14);
      days.push(count);
    }
    weeks.push(days);
  }
  return weeks;
}

export function makeMetrics(seed: number, days: number): MockMetricsMap {
  return {
    commit_rate: {
      kind: "time_series",
      label: "Commits per day",
      unit: "commits",
      series: series(seed + 1, days, 6, 4, true),
    },
    pr_throughput: {
      kind: "time_series",
      label: "PRs merged per day",
      unit: "PRs",
      series: series(seed + 2, days, 1.6, 1.4, true),
    },
    code_churn: {
      kind: "time_series",
      label: "Lines changed per day",
      unit: "lines",
      series: series(seed + 3, days, 420, 280, true),
    },
    comment_volume: {
      kind: "time_series",
      label: "Comments per day",
      unit: "comments",
      series: series(seed + 4, days, 9, 6, true),
    },
    time_to_merge: {
      kind: "scalar",
      label: "Median time to merge",
      value: 12.5 + (seed % 5) * 2,
      unit: "hours",
      count: 84,
    },
    review_latency: {
      kind: "scalar",
      label: "Median first-review latency",
      value: 4.2 + (seed % 3),
      unit: "hours",
      count: 84,
    },
    issue_lifetime: {
      kind: "scalar",
      label: "Median issue lifetime",
      value: 58 + (seed % 7) * 6,
      unit: "hours",
      count: 47,
    },
    open_issue_age: {
      kind: "buckets",
      label: "Open issue age",
      buckets: [
        { label: "< 24h", count: 6 + (seed % 4) },
        { label: "1–7d", count: 14 + (seed % 6) },
        { label: "1–4w", count: 9 + (seed % 5) },
        { label: "1–3mo", count: 5 + (seed % 3) },
        { label: "> 3mo", count: 3 + (seed % 2) },
      ],
    },
    contributor_leaderboard: {
      kind: "leaderboard",
      label: "Top contributors",
      rows: leaderboard(seed + 9),
    },
    heatmap: heatmap(seed + 20),
  };
}

export interface MockRepo {
  id: number;
  owner: string;
  name: string;
  full_name: string;
  is_private: boolean;
  default_branch: string;
  description: string;
  lang: string;
  langColor: string;
  stargazers: number;
  forks: number;
  open_issues: number;
  open_prs: number;
  contributors: number;
  releases: number;
  commit_rate: number;
  issue_rate: number;
  pr_rate: number;
  sync_status: string;
  last_synced_at: string | null;
  seed: number;
}

export const REPOS: MockRepo[] = [
  {
    id: 1,
    owner: "acme",
    name: "atlas-api",
    full_name: "acme/atlas-api",
    is_private: true,
    default_branch: "main",
    description: "Core REST + gRPC API powering the Atlas platform. Go, Postgres, embedded SPA.",
    lang: "Go",
    langColor: "#00ADD8",
    stargazers: 1284,
    forks: 86,
    open_issues: 37,
    open_prs: 9,
    contributors: 24,
    releases: 42,
    commit_rate: 6.4,
    issue_rate: 1.2,
    pr_rate: 1.6,
    sync_status: "complete",
    last_synced_at: isoDaysAgo(0.02),
    seed: 100,
  },
  {
    id: 2,
    owner: "acme",
    name: "atlas-web",
    full_name: "acme/atlas-web",
    is_private: true,
    default_branch: "main",
    description: "React + Vite dashboard. The thing you're looking at, basically.",
    lang: "TypeScript",
    langColor: "#3178c6",
    stargazers: 642,
    forks: 31,
    open_issues: 22,
    open_prs: 5,
    contributors: 17,
    releases: 28,
    commit_rate: 4.1,
    issue_rate: 0.8,
    pr_rate: 1.1,
    sync_status: "running",
    last_synced_at: isoDaysAgo(0.3),
    seed: 200,
  },
  {
    id: 3,
    owner: "acme",
    name: "ingest-worker",
    full_name: "acme/ingest-worker",
    is_private: false,
    default_branch: "main",
    description: "Background sync engine — GitHub backfill, delta polling, SSE progress.",
    lang: "Go",
    langColor: "#00ADD8",
    stargazers: 318,
    forks: 12,
    open_issues: 8,
    open_prs: 2,
    contributors: 9,
    releases: 15,
    commit_rate: 2.3,
    issue_rate: 0.4,
    pr_rate: 0.6,
    sync_status: "complete",
    last_synced_at: isoDaysAgo(0.5),
    seed: 300,
  },
  {
    id: 4,
    owner: "acme",
    name: "design-tokens",
    full_name: "acme/design-tokens",
    is_private: false,
    default_branch: "main",
    description: "Shared design tokens + Tailwind preset for all Acme surfaces.",
    lang: "CSS",
    langColor: "#563d7c",
    stargazers: 89,
    forks: 7,
    open_issues: 3,
    open_prs: 1,
    contributors: 6,
    releases: 31,
    commit_rate: 0.9,
    issue_rate: 0.2,
    pr_rate: 0.3,
    sync_status: "complete",
    last_synced_at: isoDaysAgo(1.1),
    seed: 400,
  },
  {
    id: 5,
    owner: "octocat",
    name: "spec-kit",
    full_name: "octocat/spec-kit",
    is_private: false,
    default_branch: "main",
    description: "OpenAPI spec linting + codegen toolkit. Community-maintained.",
    lang: "Rust",
    langColor: "#dea584",
    stargazers: 4720,
    forks: 214,
    open_issues: 64,
    open_prs: 18,
    contributors: 71,
    releases: 53,
    commit_rate: 8.7,
    issue_rate: 2.1,
    pr_rate: 2.4,
    sync_status: "complete",
    last_synced_at: isoDaysAgo(0.1),
    seed: 500,
  },
  {
    id: 6,
    owner: "acme",
    name: "edge-cache",
    full_name: "acme/edge-cache",
    is_private: true,
    default_branch: "develop",
    description: "Distributed edge cache layer. Currently mid-backfill.",
    lang: "Go",
    langColor: "#00ADD8",
    stargazers: 56,
    forks: 3,
    open_issues: 11,
    open_prs: 0,
    contributors: 4,
    releases: 6,
    commit_rate: 1.4,
    issue_rate: 0.3,
    pr_rate: 0.2,
    sync_status: "pending",
    last_synced_at: null,
    seed: 600,
  },
];

export const WINDOW_DAYS: Record<string, number> = {
  "30d": 30,
  "90d": 90,
  "6m": 182,
  "1y": 365,
  all: 365,
};

export interface MockCollection {
  id: string;
  name: string;
  desc: string;
  emoji: string;
  repoIds: number[];
}

export const COLLECTIONS: MockCollection[] = [
  { id: "c1", name: "Backend services", desc: "Core APIs and workers", emoji: "server", repoIds: [1, 3, 6] },
  { id: "c2", name: "Frontend & design", desc: "Dashboards and the shared design system", emoji: "layout", repoIds: [2, 4] },
  { id: "c3", name: "Open source", desc: "Public, community-maintained repos", emoji: "globe", repoIds: [5, 3, 4] },
];

export interface MockSyncPhase {
  phase: string;
  message: string;
  done?: boolean;
}

export const SYNC_PHASES: MockSyncPhase[] = [
  { phase: "backfill", message: "starting backfill from cursor…" },
  { phase: "commits", message: "commits: page 1/4 (100 rows)" },
  { phase: "commits", message: "commits: page 2/4 (100 rows)" },
  { phase: "commits", message: "commits: page 3/4 (100 rows)" },
  { phase: "commits", message: "commits: page 4/4 (62 rows)" },
  { phase: "prs", message: "pull requests: 48 rows" },
  { phase: "issues", message: "issues: 37 rows" },
  { phase: "releases", message: "releases: 12 rows" },
  { phase: "metrics", message: "recomputing metrics registry…" },
  { phase: "done", message: "sync complete · 411 objects updated", done: true },
];

// sum a time-series metric (by key) element-wise across a set of repos
export function aggregateSeries(repos: MockRepo[], key: "commit_rate" | "pr_throughput" | "code_churn" | "comment_volume", days: number): SeriesPoint[] {
  const acc: Record<string, number> = {};
  repos.forEach((r) => {
    const s = makeMetrics(r.seed, days)[key].series;
    s.forEach((p) => {
      acc[p.date] = (acc[p.date] || 0) + p.value;
    });
  });
  return Object.keys(acc)
    .sort()
    .map((date) => ({ date, value: Math.round(acc[date] * 10) / 10 }));
}

// element-wise sum of weekly heatmaps across repos
export function aggregateHeatmap(repos: MockRepo[]): number[][] {
  const base = repos.map((r) => makeMetrics(r.seed, 365).heatmap);
  const out: number[][] = [];
  for (let w = 0; w < 53; w++) {
    const days: number[] = [];
    for (let d = 0; d < 7; d++) {
      days.push(base.reduce((a, h) => a + (h[w] ? h[w][d] : 0), 0));
    }
    out.push(days);
  }
  return out;
}

// merge contributor leaderboards across repos by login
export function mergedLeaderboard(repos: MockRepo[]): LeaderRow[] {
  const map: Record<string, LeaderRow> = {};
  repos.forEach((r) => {
    makeMetrics(r.seed + 9, 365).contributor_leaderboard.rows.forEach((row) => {
      const e = map[row.login] || (map[row.login] = { login: row.login, img: row.img, commits: 0, additions: 0, deletions: 0 });
      e.commits += row.commits;
      e.additions += row.additions;
      e.deletions += row.deletions;
    });
  });
  return Object.values(map).sort((a, b) => b.commits - a.commits);
}

export interface LanguageRow {
  label: string;
  color: string;
  count: number;
  repos: number;
}

// commits/day grouped by repo language
export function languageBreakdown(repos: MockRepo[]): LanguageRow[] {
  const map: Record<string, LanguageRow> = {};
  repos.forEach((r) => {
    const e = map[r.lang] || (map[r.lang] = { label: r.lang, color: r.langColor, count: 0, repos: 0 });
    e.count += r.commit_rate;
    e.repos += 1;
  });
  return Object.values(map)
    .map((e) => ({ ...e, count: Math.round(e.count * 10) / 10 }))
    .sort((a, b) => b.count - a.count);
}

export interface MockOverview {
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
  sync_status: string;
  last_synced_at: string | null;
  window_from: string;
  window_to: string;
}

// overview bundle for a repo within a window
export function overview(repo: MockRepo, win: string, excludeBots: boolean): MockOverview {
  const adj = excludeBots ? 0.86 : 1;
  return {
    id: repo.id,
    full_name: repo.full_name,
    is_private: repo.is_private,
    default_branch: repo.default_branch,
    description: repo.description,
    stargazers: repo.stargazers,
    forks: repo.forks,
    open_issues: repo.open_issues,
    open_prs: repo.open_prs,
    contributors: excludeBots ? Math.max(1, repo.contributors - 2) : repo.contributors,
    commit_rate: Math.round(repo.commit_rate * adj * 10) / 10,
    issue_rate: repo.issue_rate,
    pr_rate: Math.round(repo.pr_rate * adj * 10) / 10,
    releases: repo.releases,
    sync_status: repo.sync_status,
    last_synced_at: repo.last_synced_at,
    window_from: dateDaysAgo(WINDOW_DAYS[win] || 30),
    window_to: "2026-05-31",
  };
}
