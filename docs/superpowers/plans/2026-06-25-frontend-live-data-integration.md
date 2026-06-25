# Frontend Live-Data Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the React dashboard's fabricated `data.ts` mock layer with live calls to the existing Go JSON API so the UI renders the user's real tracked-repo data.

**Architecture:** The backend (M1–M6, merged) is feature-complete and exposes every endpoint needed. The frontend already contains a correct, api-wired layer — `web/src/api.ts`, the hooks (`useRepos`, `useCollections`, `useAsync`), and api-typed leaf components (`RepoCard.tsx`, `RefreshButton.tsx`, `MetricView`, `TimeSeriesChart`, `ScalarStat`, `BucketsBar`, `Leaderboard`, `LatestList`, etc.) — but the **pages** instead import a parallel mock-design layer (`web/src/data.ts` and the `RepoCard`/`RefreshButton` variants exported from `web/src/components/Components.tsx`). This plan repoints each page at the real layer phase-by-phase, derives the few fields the backend doesn't store (avatars, contribution heatmap) on the client, and deletes the mock layer last. No backend changes (the language card, which has no backend source, is cut with an "unavailable" note per product decision; an optional backend phase to enable it is in Appendix A).

**Tech Stack:** React 18 + TypeScript + Vite 5, Vitest 2 + @testing-library/react (jsdom), uPlot charts, react-router-dom. Go 1.25 backend embeds `web/dist` via `go:embed`.

---

## Product Decisions (locked)

1. **Language breakdown card** (WorkspaceInsights "Activity by language"): **CUT**. No language is collected by the backend. Its slot renders a small "Language breakdown isn't available yet" empty state. Optional backend enablement is Appendix A (do not execute unless requested).
2. **WorkspaceInsights page**: **KEEP** via client-side fan-out (one `GET /api/repos/{id}/metrics` per tracked repo, aggregated in the browser).
3. **Avatars**: backend leaderboard/latest responses omit avatar URLs. Derive them deterministically client-side: `https://avatars.githubusercontent.com/${login}?size=48`.
4. **Releases history list** (RepoDetail "Releases" tab): no list endpoint exists (only a `releases` count via overview). Show the count + an "individual release history isn't available yet" note (same pattern as the language card). Do not fabricate dates.
5. **open_issue_age bucket labels** differ cosmetically from the mock (`<24h/<7d/<30d/<90d/<180d/older`). The real `BucketsBar` renders `bucket.label` verbatim, so no code change is needed — accept the backend labels.

---

## Capability Reference (what each view binds to)

| View / data | Endpoint (via `api.ts`) | Notes |
|---|---|---|
| Repo list | `listRepos()` → `GET /api/repos` | Lacks headline counts — fetch overview per repo for those. |
| Repo headline counts (stars, forks, open_issues, open_prs, contributors, releases, *_rate) | `fetchOverview(id, {window, excludeBots})` → `GET /api/repos/{id}` | Returns `Overview`. No `lang`. |
| Per-repo time series (commit_rate, pr_throughput, code_churn, comment_volume) | `fetchMetrics(id, {keys, window, excludeBots})` | `TimeSeriesResult` each. |
| Per-repo scalars (time_to_merge, review_latency, issue_lifetime) | same | `ScalarResult`, `unit:"hours"`. |
| Open issue age | `fetchMetrics(id,{keys:["open_issue_age"]})` | `BucketsResult`. |
| Contributor leaderboard | `fetchMetrics(id,{keys:["contributor_leaderboard"]})` | `LeaderboardResult`; no avatar → derive from login. |
| Latest commits/prs/issues | `fetchLatest(id, kind, limit)` | no avatar → derive from `author_login`. |
| Refresh + live progress | `RefreshButton` (real) → `refreshRepo()` + `openSyncStream()` | SSE. |
| Contribution heatmap (53×7) | derived client-side from `commit_rate` series (`window:"all"`) | helper `seriesToHeatmap`. |
| Workspace aggregate series / merged leaderboard / heatmap | fan-out `fetchMetrics` per repo, aggregate client-side | helpers in `aggregate.ts`. |
| Language breakdown | **none** | cut → "unavailable" note. |
| Collections CRUD | `useCollections()` | `Collection = {id:number,name,repo_ids:number[]}`; no `desc`/`emoji`. |
| Current user | `fetchMe()` → `Me` | no `name` field → display `login`. |

---

## Type Reconciliation (mock → real)

| Mock (`data.ts`) | Real (`api.ts`) | Action |
|---|---|---|
| `MockRepo.owner` / `.name` | — | derive via `splitRepo(full_name)` |
| `MockRepo.{open_issues,open_prs,contributors,releases,commit_rate,issue_rate,pr_rate}` | `Overview.*` | fetch overview per repo |
| `MockRepo.{lang,langColor}` | — | drop (cut language UI) |
| `MockRepo.seed` | — | drop (replaced by real fetches) |
| `MockMe.name` | — | display `me.login` |
| `MockCollection.{id:string,desc,emoji,repoIds}` | `Collection.{id:number,repo_ids}` | drop desc/emoji; rename repoIds→repo_ids; id is number |
| `MockMetricsMap.heatmap` | — | derive via `seriesToHeatmap(commit_rate.series)` |
| `LeaderRow.img` / `Mock*.author_img` | — | derive via `avatarURL(login)` |

---

## File Structure

**Create:**
- `web/src/aggregate.ts` — pure cross-repo/reshape helpers (`sumSeries`, `mergeLeaderboards`, `sumHeatmaps`, `seriesToHeatmap`) + types.
- `web/src/aggregate.test.ts` — unit tests for the above.

**Modify (pure helpers):**
- `web/src/format.ts` — add `avatarURL(login)` and `splitRepo(fullName)`.
- `web/src/format.test.ts` — tests for the two helpers.

**Modify (pages — repoint to real layer):**
- `web/src/App.tsx` — real auth/repos/collections via `fetchMe` + `useRepos` + `useCollections`.
- `web/src/pages/Overview.tsx` — real `Repo[]`; per-repo overview + sparkline fetch; real `RepoCard`.
- `web/src/pages/RepoDetail.tsx` — real metrics/overview/latest; real `RefreshButton`; derived heatmap; releases note.
- `web/src/pages/WorkspaceInsights.tsx` — client-side fan-out aggregation; language card → "unavailable".
- `web/src/pages/Collections.tsx` — real `Collection`/`Repo`; `useCollections` mutations; drop language footer.
- `web/src/components/Components.tsx` — remove the mock `RepoCard` and mock `RefreshButton` exports (and any other `data.ts`-bound exports) once no page imports them.

**Modify (tests):**
- `web/src/pages/Overview.test.tsx`, `web/src/pages/RepoDetail.test.tsx` — switch fixtures to api types; mock `api` fetches.

**Delete (final phase):**
- `web/src/data.ts` — the mock module (after grep confirms zero importers).

---

## Conventions for every task

- Run a single test: `cd web && npx vitest run src/<path>.test.tsx`
- Run all frontend tests: `cd web && npm test`
- Typecheck + bundle: `cd web && npm run build` (`tsc -b && vite build`)
- Test pattern: mock named api exports with `vi.spyOn(api, "fn").mockResolvedValue(...)` (NOT `vi.mock`); wrap router-using components in `<MemoryRouter>`; use `waitFor` for async settle; stub SSE with `vi.stubGlobal("EventSource", ...)`. The setup file `web/vitest.setup.ts` already seeds the `gs_csrf` cookie and runs `cleanup()` + `vi.restoreAllMocks()` after each test.
- Commit after each task with a `feat(web):` / `test(web):` / `refactor(web):` message.

---

## Phase 0 — Shared client helpers

### Task 0.1: `avatarURL` and `splitRepo` helpers

**Files:**
- Modify: `web/src/format.ts`
- Test: `web/src/format.test.ts`

- [ ] **Step 1: Write the failing tests** (append to `format.test.ts`)

```ts
import { avatarURL, splitRepo } from "./format";

describe("avatarURL", () => {
  it("builds a deterministic GitHub avatar URL from a login", () => {
    expect(avatarURL("octocat")).toBe("https://avatars.githubusercontent.com/octocat?size=48");
  });
});

describe("splitRepo", () => {
  it("splits owner/name from a full_name", () => {
    expect(splitRepo("octocat/Hello-World")).toEqual({ owner: "octocat", name: "Hello-World" });
  });
  it("tolerates names containing extra slashes by taking the first segment as owner", () => {
    expect(splitRepo("acme/edge/cache")).toEqual({ owner: "acme", name: "edge/cache" });
  });
});
```

- [ ] **Step 2: Run, expect failure**

Run: `cd web && npx vitest run src/format.test.ts`
Expected: FAIL — `avatarURL`/`splitRepo` are not exported.

- [ ] **Step 3: Implement** (add to `format.ts`)

```ts
export function avatarURL(login: string): string {
  return `https://avatars.githubusercontent.com/${login}?size=48`;
}

export function splitRepo(fullName: string): { owner: string; name: string } {
  const i = fullName.indexOf("/");
  if (i < 0) return { owner: fullName, name: "" };
  return { owner: fullName.slice(0, i), name: fullName.slice(i + 1) };
}
```

- [ ] **Step 4: Run, expect pass**

Run: `cd web && npx vitest run src/format.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/format.ts web/src/format.test.ts
git commit -m "feat(web): add avatarURL and splitRepo helpers"
```

---

### Task 0.2: Aggregation + reshape helpers (`aggregate.ts`)

These are pure functions operating on `api.ts` types — the real-data equivalents of the mock's `aggregateSeries`/`mergedLeaderboard`/`aggregateHeatmap`, plus the per-repo `seriesToHeatmap` reshaper used by RepoDetail and the workspace heatmap.

**Files:**
- Create: `web/src/aggregate.ts`
- Test: `web/src/aggregate.test.ts`

- [ ] **Step 1: Write the failing tests** (`aggregate.test.ts`)

```ts
import { describe, it, expect } from "vitest";
import { sumSeries, mergeLeaderboards, sumHeatmaps, seriesToHeatmap } from "./aggregate";
import type { SeriesPoint, LeaderRow } from "./api";

describe("sumSeries", () => {
  it("sums values per date across series and sorts by date", () => {
    const a: SeriesPoint[] = [{ date: "2026-01-01", value: 2 }, { date: "2026-01-02", value: 3 }];
    const b: SeriesPoint[] = [{ date: "2026-01-02", value: 4 }, { date: "2026-01-03", value: 1 }];
    expect(sumSeries([a, b])).toEqual([
      { date: "2026-01-01", value: 2 },
      { date: "2026-01-02", value: 7 },
      { date: "2026-01-03", value: 1 },
    ]);
  });
  it("returns [] for no input", () => {
    expect(sumSeries([])).toEqual([]);
  });
});

describe("mergeLeaderboards", () => {
  it("merges rows by login and sorts by commits desc", () => {
    const a: LeaderRow[] = [{ login: "x", commits: 5, additions: 10, deletions: 1 }];
    const b: LeaderRow[] = [
      { login: "x", commits: 2, additions: 4, deletions: 0 },
      { login: "y", commits: 9, additions: 1, deletions: 1 },
    ];
    expect(mergeLeaderboards([a, b])).toEqual([
      { login: "y", commits: 9, additions: 1, deletions: 1 },
      { login: "x", commits: 7, additions: 14, deletions: 1 },
    ]);
  });
});

describe("sumHeatmaps", () => {
  it("element-wise sums equally-shaped grids", () => {
    expect(sumHeatmaps([[[1, 2]], [[3, 4]]])).toEqual([[4, 6]]);
  });
});

describe("seriesToHeatmap", () => {
  it("produces a 53x7 grid placing each date's value at [week][weekday]", () => {
    const grid = seriesToHeatmap([{ date: "2026-01-01", value: 5 }]); // Thu = weekday 4
    expect(grid).toHaveLength(53);
    expect(grid[0]).toHaveLength(7);
    const total = grid.flat().reduce((s, v) => s + v, 0);
    expect(total).toBe(5);
  });
});
```

- [ ] **Step 2: Run, expect failure**

Run: `cd web && npx vitest run src/aggregate.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement** (`aggregate.ts`)

```ts
import type { SeriesPoint, LeaderRow } from "./api";

/** Sum multiple daily series element-wise by date; result sorted ascending by date. */
export function sumSeries(seriesList: SeriesPoint[][]): SeriesPoint[] {
  const acc = new Map<string, number>();
  for (const series of seriesList) {
    for (const p of series) acc.set(p.date, (acc.get(p.date) ?? 0) + p.value);
  }
  return [...acc.entries()]
    .map(([date, value]) => ({ date, value }))
    .sort((a, b) => a.date.localeCompare(b.date));
}

/** Merge contributor leaderboards across repos by login; sorted by commits desc. */
export function mergeLeaderboards(boards: LeaderRow[][]): LeaderRow[] {
  const acc = new Map<string, LeaderRow>();
  for (const board of boards) {
    for (const r of board) {
      const cur = acc.get(r.login) ?? { login: r.login, commits: 0, additions: 0, deletions: 0 };
      cur.commits += r.commits;
      cur.additions += r.additions;
      cur.deletions += r.deletions;
      acc.set(r.login, cur);
    }
  }
  return [...acc.values()].sort((a, b) => b.commits - a.commits);
}

/** Element-wise sum of equally-shaped numeric grids. */
export function sumHeatmaps(grids: number[][][]): number[][] {
  if (grids.length === 0) return [];
  const rows = grids[0].length;
  const cols = grids[0][0]?.length ?? 0;
  const out: number[][] = Array.from({ length: rows }, () => Array(cols).fill(0));
  for (const g of grids) {
    for (let r = 0; r < rows; r++) for (let c = 0; c < cols; c++) out[r][c] += g[r]?.[c] ?? 0;
  }
  return out;
}

/**
 * Reshape a daily commit series into a 53-week x 7-day grid (week index 0..52,
 * weekday 0=Sun..6=Sat), placing the most recent date in the last column.
 */
export function seriesToHeatmap(series: SeriesPoint[]): number[][] {
  const grid: number[][] = Array.from({ length: 53 }, () => Array(7).fill(0));
  if (series.length === 0) return grid;
  const sorted = [...series].sort((a, b) => a.date.localeCompare(b.date));
  const last = new Date(sorted[sorted.length - 1].date + "T00:00:00Z");
  for (const p of sorted) {
    const d = new Date(p.date + "T00:00:00Z");
    const daysAgo = Math.round((last.getTime() - d.getTime()) / 86_400_000);
    const weekFromEnd = Math.floor(daysAgo / 7);
    if (weekFromEnd > 52) continue;
    const week = 52 - weekFromEnd;
    const weekday = d.getUTCDay();
    grid[week][weekday] += p.value;
  }
  return grid;
}
```

- [ ] **Step 4: Run, expect pass**

Run: `cd web && npx vitest run src/aggregate.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/aggregate.ts web/src/aggregate.test.ts
git commit -m "feat(web): add pure aggregation + heatmap-reshape helpers"
```

---

## Phase 1 — App shell: real auth, repos, collections

`App.tsx` currently seeds `repos`/`collections`/`me` from `D.REPOS`/`D.COLLECTIONS`/`D.ME`. Replace with `useAsync(fetchMe, [])`, `useRepos()`, `useCollections()`. Keep prop-drilling repos/collections into pages (so page changes stay small and existing prop-driven tests survive).

**Files:**
- Modify: `web/src/App.tsx`

- [ ] **Step 1: Replace mock state with real data sources.** At the top of `App()`:

```tsx
import { useAsync } from "./hooks/useAsync";
import { useRepos } from "./hooks/useRepos";
import { useCollections } from "./hooks/useCollections";
import { fetchMe } from "./api";
import type { Repo, Me } from "./api";
// ...
const meState = useAsync<Me | null>(fetchMe, []);
const reposApi = useRepos();
const collectionsApi = useCollections();
```

Remove the three `useState(D.*)` lines (175–178) and the `import * as D from "./data"` once no `D.*` references remain in this file.

- [ ] **Step 2: Gate rendering on auth.** Where the component previously checked `me === null`:

```tsx
if (meState.loading) return <div className="app-loading">Loading…</div>;
const me = meState.data;
if (!me) return <SignIn />;
```

- [ ] **Step 3: Repoint props.** Pass `reposApi.repos` (type `Repo[]`) where `repos` was passed; pass `collectionsApi.collections` where `collections` was passed. Update `UserMenuProps.me` to `Me` and render `me.login` wherever `me.name` was shown (avatar from `me.avatar_url`).

- [ ] **Step 4: Wire mutations.**
  - `handleAddRepo(fullName)` → `await reposApi.add(fullName)` (drop the hand-built `nr` object literal at line 217).
  - `handleAddCollection(name)` → `await collectionsApi.create(name)`.
  - In `RepoDetailWrapper`, change `repos: D.MockRepo[]` → `repos: Repo[]` and resolve via `reposApi.resolve(owner, name)` (it already splits/compares on `full_name`). Pass the resolved `Repo` to `<RepoDetail repo={matched} … />`.

- [ ] **Step 5: Typecheck.**

Run: `cd web && npm run build`
Expected: `tsc -b` passes (no remaining `D.*` type errors in `App.tsx`). It is acceptable that other pages still reference `D` at this stage — only fix `App.tsx`'s own references here; if `tsc` errors originate in still-mock pages, that's expected until their phases. To keep the build green between phases, leave `data.ts` in place (it is deleted in Phase 6) and only remove `D` usage file-by-file.

- [ ] **Step 6: Smoke test.** Add `web/src/App.test.tsx`:

```tsx
import { describe, it, expect, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import App from "./App";
import * as api from "./api";

describe("App auth gate", () => {
  it("shows sign-in when unauthenticated", async () => {
    vi.spyOn(api, "fetchMe").mockResolvedValue(null);
    vi.spyOn(api, "listRepos").mockResolvedValue([]);
    vi.spyOn(api, "listCollections").mockResolvedValue([]);
    render(<MemoryRouter><App /></MemoryRouter>);
    await waitFor(() => expect(screen.getByText(/sign in/i)).toBeInTheDocument());
  });

  it("renders the shell when authenticated", async () => {
    vi.spyOn(api, "fetchMe").mockResolvedValue({ id: 1, github_id: 9, login: "maya", avatar_url: "" });
    vi.spyOn(api, "listRepos").mockResolvedValue([]);
    vi.spyOn(api, "listCollections").mockResolvedValue([]);
    render(<MemoryRouter><App /></MemoryRouter>);
    await waitFor(() => expect(screen.getByText(/maya/i)).toBeInTheDocument());
  });
});
```

> Note: if `App` already mounts its own `<BrowserRouter>`, drop the `<MemoryRouter>` wrapper here and rely on the internal router. Confirm by reading the bottom of `App.tsx`.

Run: `cd web && npx vitest run src/App.test.tsx`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add web/src/App.tsx web/src/App.test.tsx
git commit -m "feat(web): wire app shell to live auth, repos, and collections"
```

---

## Phase 2 — Overview page

`Overview.tsx` receives `repos: Repo[]` (from Phase 1) and must show, per card, the headline counts (from `fetchOverview`) and a commit sparkline (from `fetchMetrics` `commit_rate`). Switch to the real `RepoCard` (`web/src/components/RepoCard.tsx`, props `{ repo: Repo; overview: Overview | null }`).

**Files:**
- Modify: `web/src/pages/Overview.tsx`
- Modify: `web/src/pages/Overview.test.tsx`

- [ ] **Step 1: Update prop types.** `OverviewProps.repos: Repo[]`, `onOpen: (repo: Repo) => void`. Remove `import * as D`.

- [ ] **Step 2: Fetch per-repo overviews for the cards + KPI strip.**

```tsx
import { useAsync } from "../hooks/useAsync";
import { fetchOverview } from "../api";
import type { Overview as OverviewT, Repo } from "../api";

const ids = repos.map((r) => r.id).join(",");
const ovState = useAsync<Record<number, OverviewT>>(async () => {
  const list = await Promise.all(
    repos.map((r) => fetchOverview(r.id, { window: "90d", excludeBots: false })),
  );
  return Object.fromEntries(list.map((o) => [o.id, o]));
}, [ids]);
const overviews = ovState.data ?? {};
```

Replace the KPI-strip aggregation (lines ~62–65, which read `r.commit_rate` etc. off the mock repo) with sums over `overviews`:

```tsx
const agg = Object.values(overviews).reduce(
  (a, o) => ({
    commits: a.commits + o.commit_rate,
    contributors: a.contributors + o.contributors,
    openPrs: a.openPrs + o.open_prs,
    openIssues: a.openIssues + o.open_issues,
  }),
  { commits: 0, contributors: 0, openPrs: 0, openIssues: 0 },
);
```

- [ ] **Step 3: Render real `RepoCard`.** Replace the mock `RepoCard` import from `"../components/Components"` with `import RepoCard from "../components/RepoCard";` and render:

```tsx
<RepoCard key={r.id} repo={r} overview={overviews[r.id] ?? null} />
```

The real `RepoCard` links via `<Link to={"/" + repo.full_name}>` and reads counts from `overview`. (The mock sparkline is dropped here; the sparkline lives on RepoDetail. If a card sparkline is still desired, fetch `commit_rate` metrics in Step 2 and extend `RepoCard` — out of scope for first integration.)

- [ ] **Step 4: Keep the empty state.** The existing "No repositories match" branch stays; it now triggers when `repos.length === 0`.

- [ ] **Step 5: Update the test** (`Overview.test.tsx`): replace the `D.MockRepo` fixture with an `api.Repo` fixture and mock `fetchOverview`.

```tsx
import * as api from "../api";
const REPOS: api.Repo[] = [
  { id: 1, full_name: "octocat/hello", is_private: false, default_branch: "main", sync_status: "complete", last_synced_at: null },
];
beforeEach(() => {
  vi.spyOn(api, "fetchOverview").mockResolvedValue({
    id: 1, full_name: "octocat/hello", is_private: false, default_branch: "main", description: "Hi",
    stargazers: 1, forks: 0, open_issues: 4, open_prs: 1, contributors: 3, commit_rate: 2,
    issue_rate: 0.5, pr_rate: 0.3, releases: 0, sync_status: "complete", last_synced_at: null,
    window_from: "2026-01-01", window_to: "2026-03-31",
  });
});
```

Keep the three existing assertions (renders "hello"; typing `a/b` + "Track repo" calls `onAdd("a/b")`; `repos=[]` shows the empty state), wrapping the render in `<MemoryRouter>` (already present).

- [ ] **Step 6: Run tests + typecheck.**

Run: `cd web && npx vitest run src/pages/Overview.test.tsx && npm run build`
Expected: PASS / clean tsc.

- [ ] **Step 7: Commit**

```bash
git add web/src/pages/Overview.tsx web/src/pages/Overview.test.tsx
git commit -m "feat(web): render Overview from live repo overviews"
```

---

## Phase 3 — RepoDetail page

Bind metrics (all keys), overview, latest lists, refresh, and the derived heatmap. Honor the window + exclude-bots controls by feeding them into `useAsync` deps so changes refetch.

**Files:**
- Modify: `web/src/pages/RepoDetail.tsx`
- Modify: `web/src/pages/RepoDetail.test.tsx`

- [ ] **Step 1: Prop + imports.** `RepoDetailProps.repo: Repo`. Add:

```tsx
import { useAsync } from "../hooks/useAsync";
import { fetchMetrics, fetchOverview, fetchLatest } from "../api";
import type { MetricsMap, Overview as OverviewT, TimeSeriesResult, ScalarResult, BucketsResult, LeaderboardResult } from "../api";
import { seriesToHeatmap } from "../aggregate";
import RefreshButton from "../components/RefreshButton";
```

Header fields that used `repo.owner`/`repo.name` → `splitRepo(repo.full_name)`. Drop the `lang`/`langColor` badge.

- [ ] **Step 2: Fetch overview + metrics on window/excludeBots change.**

```tsx
const q = { window: win, excludeBots };
const ov = useAsync<OverviewT>(() => fetchOverview(repo.id, q), [repo.id, win, excludeBots]);
const metrics = useAsync<MetricsMap>(
  () => fetchMetrics(repo.id, { ...q, keys: [
    "commit_rate","pr_throughput","code_churn","comment_volume",
    "time_to_merge","review_latency","issue_lifetime",
    "open_issue_age","contributor_leaderboard",
  ]}),
  [repo.id, win, excludeBots],
);
const heat = useAsync<number[][]>(async () => {
  const m = await fetchMetrics(repo.id, { window: "all", excludeBots, keys: ["commit_rate"] });
  return seriesToHeatmap((m.commit_rate as TimeSeriesResult).series);
}, [repo.id, excludeBots]);
```

- [ ] **Step 3: Feed the existing presentational sub-components.** Keep all JSX; swap data sources. Cast tagged-union members from `metrics.data`:

```tsx
const m = metrics.data;
// guard while loading:
if (!m || !ov.data) return <div className="loading">Loading…</div>;
// series → existing BarSeries/AreaSeries (they take `series`):
<BarSeries series={(m.commit_rate as TimeSeriesResult).series} />
<AreaSeries series={(m.code_churn as TimeSeriesResult).series} />
<BarSeries series={(m.pr_throughput as TimeSeriesResult).series} />
<AreaSeries series={(m.comment_volume as TimeSeriesResult).series} />
// scalars → ScalarStat (takes `result: ScalarResult`):
<ScalarStat result={m.time_to_merge as ScalarResult} />
<ScalarStat result={m.review_latency as ScalarResult} />
<ScalarStat result={m.issue_lifetime as ScalarResult} />
// buckets / leaderboard:
<BucketsBar result={m.open_issue_age as BucketsResult} />
<Leaderboard result={m.contributor_leaderboard as LeaderboardResult} />
// heatmap (derived):
<ContributionHeatmap weeks={heat.data ?? []} />
```

KPI strip values come from `ov.data` (`commit_rate`, `open_prs`, `open_issues`, `contributors`, `releases`).

- [ ] **Step 4: Latest tabs.** Fetch per kind; pass to the existing `LatestList` (`{ kind, items }`):

```tsx
const commits = useAsync(() => fetchLatest(repo.id, "commits", 20), [repo.id]);
const prs = useAsync(() => fetchLatest(repo.id, "prs", 20), [repo.id]);
const issues = useAsync(() => fetchLatest(repo.id, "issues", 20), [repo.id]);
// <CommitsTab> → <LatestList kind="commits" items={commits.data ?? []} />  (etc.)
```

If `LatestList` renders avatars, have it call `avatarURL(item.author_login)` (the items carry no `author_img`). Confirm by reading `LatestList.tsx`; if it references a missing avatar field, switch that to `avatarURL(...)`.

- [ ] **Step 5: Real refresh.** Replace the mock `RefreshButton` (from `Components.tsx`, which animates `D.SYNC_PHASES`) with the real one and reload on completion:

```tsx
<RefreshButton repoID={repo.id} onComplete={() => { ov.reload(); metrics.reload(); heat.reload(); commits.reload(); prs.reload(); issues.reload(); }} />
```

Remove the `D.isoDaysAgo(0)` mutation at line ~145.

- [ ] **Step 6: Releases tab.** Replace the fabricated list (`D.isoDaysAgo(i*18+3)`, line ~418) with the count + note:

```tsx
<p className="metric-note">{ov.data.releases} releases total.</p>
<p className="empty">Individual release history isn’t available yet.</p>
```

- [ ] **Step 7: Update the test** (`RepoDetail.test.tsx`): api-typed `Repo` fixture; mock `fetchOverview`, `fetchMetrics`, `fetchLatest`; stub `EventSource` (per the RefreshButton pattern). Preserve the existing assertions (`/octocat.*hello/i` heading, "Contributors" text, Commits tab → "Latest commits"). Example metrics mock:

```tsx
vi.spyOn(api, "fetchMetrics").mockResolvedValue({
  commit_rate: { kind: "time_series", series: [] },
  pr_throughput: { kind: "time_series", series: [] },
  code_churn: { kind: "time_series", series: [] },
  comment_volume: { kind: "time_series", series: [] },
  time_to_merge: { kind: "scalar", value: 12, unit: "hours", count: 3 },
  review_latency: { kind: "scalar", value: 5, unit: "hours", count: 3 },
  issue_lifetime: { kind: "scalar", value: 48, unit: "hours", count: 2 },
  open_issue_age: { kind: "buckets", buckets: [] },
  contributor_leaderboard: { kind: "leaderboard", rows: [] },
} as api.MetricsMap);
vi.spyOn(api, "fetchLatest").mockResolvedValue([]);
```

- [ ] **Step 8: Run tests + typecheck.**

Run: `cd web && npx vitest run src/pages/RepoDetail.test.tsx && npm run build`
Expected: PASS / clean tsc.

- [ ] **Step 9: Commit**

```bash
git add web/src/pages/RepoDetail.tsx web/src/pages/RepoDetail.test.tsx
git commit -m "feat(web): render RepoDetail from live metrics, latest, and SSE refresh"
```

---

## Phase 4 — WorkspaceInsights (client-side fan-out)

Fan out per-repo metrics, aggregate with `aggregate.ts`, and replace the language card with an "unavailable" note.

**Files:**
- Modify: `web/src/pages/WorkspaceInsights.tsx`
- Test: `web/src/pages/WorkspaceInsights.test.tsx` (new)

- [ ] **Step 1: Prop + imports.** `repos: Repo[]`. Add:

```tsx
import { useAsync } from "../hooks/useAsync";
import { fetchMetrics, fetchOverview } from "../api";
import type { TimeSeriesResult, LeaderboardResult, Overview as OverviewT, Repo } from "../api";
import { sumSeries, mergeLeaderboards, sumHeatmaps, seriesToHeatmap } from "../aggregate";
```

- [ ] **Step 2: Fan-out fetch + aggregate.**

```tsx
const ids = repos.map((r) => r.id).join(",");
const agg = useAsync(async () => {
  const per = await Promise.all(
    repos.map((r) =>
      fetchMetrics(r.id, { window: win, excludeBots, keys: ["commit_rate", "pr_throughput", "contributor_leaderboard"] }),
    ),
  );
  const overviews = await Promise.all(
    repos.map((r) => fetchOverview(r.id, { window: win, excludeBots })),
  );
  const commitSeriesList = per.map((m) => (m.commit_rate as TimeSeriesResult).series);
  return {
    commitSeries: sumSeries(commitSeriesList),
    prSeries: sumSeries(per.map((m) => (m.pr_throughput as TimeSeriesResult).series)),
    heat: sumHeatmaps(commitSeriesList.map(seriesToHeatmap)),
    board: mergeLeaderboards(per.map((m) => (m.contributor_leaderboard as LeaderboardResult).rows)),
    overviews,
  };
}, [ids, win, excludeBots]);
```

- [ ] **Step 3: Feed existing components.** While `agg.loading || !agg.data`, render a loading state. Then:

```tsx
<BarSeries series={agg.data.commitSeries} />
<AreaSeries series={agg.data.prSeries} />
<ContributionHeatmap weeks={agg.data.heat} />
<Leaderboard result={{ kind: "leaderboard", rows: agg.data.board }} compact />
```

KPI cards (lines ~161–167) sum from `agg.data.overviews` (`commit_rate`, `open_prs`, `open_issues`, `releases`).

For the "Most Active Repositories" table sparklines (was `D.makeMetrics(r.seed,90).commit_rate.series`), reuse the per-repo `commit_rate` series already fetched — thread it through `agg.data` as a `Record<number, SeriesPoint[]>` if the table needs per-repo series, or drop the sparkline column for the first integration.

- [ ] **Step 4: Language card → unavailable.** Replace the `<LangBars rows={D.languageBreakdown(...)} />` block with:

```tsx
<section className="card">
  <h3>Activity by language</h3>
  <p className="empty">Language breakdown isn’t available yet — language data isn’t collected.</p>
</section>
```

Remove the `LangBars` component and `D.LanguageRow` type usage.

- [ ] **Step 5: Test** (`WorkspaceInsights.test.tsx`): mock `fetchMetrics`/`fetchOverview`, pass two api `Repo`s, assert the page renders without the language data and that aggregate sections appear. Keep it light (one render + a couple of `getByText` assertions wrapped in `waitFor`).

- [ ] **Step 6: Run tests + typecheck.**

Run: `cd web && npx vitest run src/pages/WorkspaceInsights.test.tsx && npm run build`
Expected: PASS / clean tsc.

- [ ] **Step 7: Commit**

```bash
git add web/src/pages/WorkspaceInsights.tsx web/src/pages/WorkspaceInsights.test.tsx
git commit -m "feat(web): aggregate WorkspaceInsights via client-side fan-out; cut language card"
```

---

## Phase 5 — Collections page

Bind to `useCollections()` and real `Collection`/`Repo`. Drop `desc`/`emoji` (no backend) and the language footer.

**Files:**
- Modify: `web/src/pages/Collections.tsx`

- [ ] **Step 1: Types.** `CollectionsProps.collections: Collection[]`, `repos: Repo[]`. Replace `col.repoIds` with `col.repo_ids` everywhere (membership filter `repos.filter(r => col.repo_ids.includes(r.id))`).

- [ ] **Step 2: Mutations.** Receive the `useCollections()` API from `App` as props (or call `useCollections()` here). Wire create/rename/delete/add-repo/remove-repo to `create`/`rename`/`remove`/`addRepo`/`removeRepo`. The `NewCollectionModal.onCreate` now takes just a `name: string`.

- [ ] **Step 3: Collection card aggregates.** Replace `D.aggregateSeries(members,"commit_rate",90)` with a fan-out over `members` using `fetchMetrics(id,{keys:["commit_rate"],window:"90d",excludeBots:false})` + `sumSeries(...)` inside a `useAsync` keyed on the member ids; feed the existing `<Sparkline series={spark}>`. Stat chips (commit_rate/contributors/open_prs) sum from per-member `fetchOverview` (or omit chips needing overview if you want fewer requests).

- [ ] **Step 4: Drop language footer.** Remove the `r.langColor`/`r.lang` dot row (lines ~86–95). Replace with member count or repo names (data that exists).

- [ ] **Step 5: Real RepoCard in detail view.** Replace the mock `RepoCard` (from `Components.tsx`) with `components/RepoCard.tsx`, passing `{ repo, overview }` (fetch overview per member as in Phase 2).

- [ ] **Step 6: Test.** Extend or add `Collections.test.tsx`: mock `listCollections`/`fetchMetrics`/`fetchOverview`; assert a collection name renders and "New collection" creates via the mocked `createCollection`.

- [ ] **Step 7: Run tests + typecheck.**

Run: `cd web && npx vitest run src/pages/Collections.test.tsx && npm run build`
Expected: PASS / clean tsc.

- [ ] **Step 8: Commit**

```bash
git add web/src/pages/Collections.tsx web/src/pages/Collections.test.tsx
git commit -m "feat(web): wire Collections to live collections + repo data"
```

---

## Phase 6 — Remove mock layer, rebuild, verify

- [ ] **Step 1: Remove orphaned mock exports** from `web/src/components/Components.tsx` (the mock `RepoCard` and mock `RefreshButton`, and any other export that imports from `../data`). Keep the genuinely shared presentational components (`BarSeries`, `AreaSeries`, `Sparkline`, `ContributionHeatmap`, etc.) — but ensure none still import `../data`.

- [ ] **Step 2: Confirm no importers of the mock module.**

Run: `cd web && grep -rn "from \"\.\./data\"\|from \"\./data\"\|from \"\.\./\.\./data\"" src || echo "NO IMPORTERS"`
Expected: `NO IMPORTERS`.

- [ ] **Step 3: Delete the mock module.**

```bash
git rm web/src/data.ts
```

- [ ] **Step 4: Full frontend gate.**

Run: `cd web && npm test && npm run build`
Expected: all Vitest suites PASS; `tsc -b && vite build` succeeds and emits a fresh `web/dist/` (new `index.html` + matching `assets/` hashes — this also fixes the stale committed `index.html`/asset-hash mismatch).

- [ ] **Step 5: Backend still green + full embed build.**

Run: `make test && make build`
Expected: `go test ./...` PASS; `bin/github-stats` builds with the freshly embedded `dist`.

- [ ] **Step 6: Commit.**

```bash
git add -A
git commit -m "refactor(web): delete mock data module after live-API integration"
```

- [ ] **Step 7: Manual verification (real data path).** Document/observe end-to-end:
  1. `make dev-api` (port 8080) and `make dev-web` (port 5173, proxies `/api` + `/auth`).
  2. Visit `http://localhost:5173`, sign in via GitHub OAuth → real `me.login` appears (not "maya-dev").
  3. Add a repo (`owner/name`) → a backfill job enqueues; the SSE refresh log streams real phases.
  4. After backfill, Overview cards show real counts; RepoDetail charts show real series; WorkspaceInsights aggregates across repos.
  5. Confirm an empty DB renders empty states (not fabricated repos), and the language card shows the "unavailable" note.

Use the `verify` skill / a browser-automation pass for screenshots if desired.

---

## Appendix A — OPTIONAL backend phase: enable the language card

Do NOT execute unless the user opts back into backend work. This re-enables the cut language breakdown.

- [ ] **A.1** Migration `internal/store/migrations/0006_repo_language.sql`:

```sql
ALTER TABLE repos ADD COLUMN language TEXT NOT NULL DEFAULT '';
ALTER TABLE repos ADD COLUMN language_color TEXT NOT NULL DEFAULT '';
```

- [ ] **A.2** Extend the `Repo` struct (`internal/store/repos.go`) with `Language` / `LanguageColor`; include both columns in `repoSelect`, `UpsertRepo` insert/conflict-update.
- [ ] **A.3** Populate from GitHub repo metadata in `FetchRepoMeta` (`internal/githubapi/fetch.go`) — GraphQL `primaryLanguage { name color }` — and pass through where `UpsertRepo` is called (add-repo handler + backfill).
- [ ] **A.4** Expose `language` / `language_color` in `repoJSON` (`internal/api/repos.go`) and `overviewJSON` (`internal/api/metrics.go`).
- [ ] **A.5** Add `language?` / `language_color?` to `Repo`/`Overview` in `web/src/api.ts`; restore a `languageBreakdown` aggregation in `aggregate.ts` (group repos by `language`, sum `commit_rate` from overviews) with a unit test; re-render `<LangBars>` in WorkspaceInsights.
- [ ] **A.6** `make test && cd web && npm test && make build`; verify the language card shows real proportions after a re-sync (existing repos backfill language on next metadata fetch).

---

## Self-Review

**Spec coverage** — every mock-driven view from the exploration is accounted for: per-repo time series / scalars / buckets / leaderboard / latest (Phases 2–3), overview headline counts (Phases 2–3), refresh+SSE (Phase 3), contribution heatmap via `seriesToHeatmap` (Phases 3–4), cross-repo aggregates + merged leaderboard (Phase 4), collections CRUD (Phase 5), auth/me/repos (Phase 1). Cut/deferred items are explicit: language card (Phase 4 note + Appendix A), releases history list (Phase 3 note), card-sparkline (optional). Mock module deleted (Phase 6).

**Placeholder scan** — pure helpers (`avatarURL`, `splitRepo`, `sumSeries`, `mergeLeaderboards`, `sumHeatmaps`, `seriesToHeatmap`) ship complete code + tests. Page tasks give exact imports, exact endpoint calls, exact field mappings, and exact props for existing components; remaining JSX is preserved by instruction (data-source swap), which is the intended minimal-diff approach for files too large to inline verbatim. Two reads are explicitly flagged for the executor to confirm before editing: whether `App.tsx` mounts its own router (Phase 1 Step 6) and whether `LatestList.tsx` renders an avatar field (Phase 3 Step 4).

**Type consistency** — names match `api.ts` exactly: `Repo`, `Me`, `Overview`, `Collection` (`repo_ids`), `MetricsMap`, `TimeSeriesResult.series`, `ScalarResult`, `BucketsResult.buckets`, `LeaderboardResult.rows`, `SeriesPoint{date,value}`, `LeaderRow{login,commits,additions,deletions}`, `fetchMe`/`listRepos`/`fetchOverview`/`fetchMetrics`/`fetchLatest`, hook shapes `useRepos().{repos,resolve,add,remove}` and `useCollections().{collections,create,rename,remove,addRepo,removeRepo}`, and `useAsync(fn,deps).{data,loading,error,reload}`. Helper names are identical between their definitions (Phase 0) and call sites (Phases 3–5).
