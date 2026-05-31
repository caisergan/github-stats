# M5 — Dashboard UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the **React + Vite dashboard** (spec §8) that renders the stats served by M1/M3/M4 entirely from the JSON API. This delivers: a fully **typed API client** (`src/api.ts`) covering `GET /api/me`, the M3 repo endpoints (`GET/POST /api/repos`, `DELETE /api/repos/{id}`, `POST /api/repos/{id}/refresh`, the `GET /api/repos/{id}/sync/stream` SSE), and the M4 read endpoints (`GET /api/repos/{id}/metrics`, `GET /api/repos/{id}` overview bundle, `GET /api/repos/{id}/latest/{commits|prs|issues}`); **one renderer per `Result` kind** (`TimeSeriesChart` over uPlot, `ScalarStat`, `BucketsBar`, `Leaderboard`) behind a `MetricView` that switches on `result.kind`; **routing & pages** via `react-router-dom` — an **Overview** page (`/`: user header, add-repo form, repo-card grid) and a **Repo detail** page (`/:owner/:repo`: `owner/repo`→id resolution, window selector + exclude-bots toggle, the seven sections Details/Insights/Commits/Issues/PRs/Contributors/Releases with metrics + latest lists, and a "Refresh now" button that opens the SSE stream and shows live progress); and **build integration** so `npm run build` typechecks + bundles and `go build ./...` still embeds, restoring the committed placeholder `web/dist/index.html` afterwards. M5 is frontend-only: **no backend/Go changes** — every endpoint already exists.

**Architecture:** Builds directly on the M1 frontend foundation (`docs/superpowers/plans/2026-05-30-m1-skeleton-and-auth.md`, Task 13): the existing `web/` package — `package.json`, `vite.config.ts` (already proxies `/api` and `/auth` → `:8080`), `tsconfig.json` + `tsconfig.node.json` (the latter `emitDeclarationOnly`), `index.html`, `src/main.tsx`, `src/App.tsx`, `src/api.ts` (already has `Me`/`fetchMe`), and `web/embed.go` (Go embeds `web/dist`; the SPA-fallback in M1 already serves `index.html` for non-`/api` client routes like `/owner/repo`). The SPA remains a **decoupled client** talking to the Go backend purely over HTTP/JSON (spec §8 serving model). The JSON contracts are fixed by M3 (`docs/superpowers/plans/2026-05-31-m3-sync-engine.md`, "Public API surface M3 exposes") and M4 (`docs/superpowers/plans/2026-05-31-m4-metrics-registry.md`, "Public API surface M4 exposes"). M5 keeps the tree clean exactly as M1: real `dist/` assets are gitignored, and after building we restore the committed placeholder (`git checkout -- web/dist/index.html`).

**Tech Stack:** React 18 + Vite 5 + TypeScript 5 (existing). New: `react-router-dom` ^6.26 (routing), `uplot` ^1.6 (tiny, fast time-series charts). New dev/test: `vitest` ^2.0 + `@testing-library/react` ^16 + `@testing-library/jest-dom` ^6 + `@testing-library/user-event` ^14 + `jsdom` ^25. Logic-heavy units (api client URL building + Result typing, `MetricView` kind-switching, scalar/buckets/leaderboard rendering, window/exclude-bots state, `owner/repo`→id resolution, SSE event handling) are covered by Vitest + RTL tests written first (test → fail → implement → pass), mocking `fetch`/`EventSource`/`uplot`. Pure-scaffolding/layout tasks use a **typecheck + build passes** verification step instead of brittle DOM snapshots (called out explicitly where used).

---

## File Structure

```
github-stats/
└── web/
    ├── package.json                      # MODIFIED: +react-router-dom, +uplot, +vitest stack, +"test" script
    ├── vite.config.ts                    # MODIFIED: add vitest `test` block (jsdom, setup file, globals)
    ├── vitest.setup.ts                   # NEW: jest-dom matchers + per-test fetch/EventSource reset
    ├── tsconfig.json                      # MODIFIED: add "vitest/globals" + setup file include note
    ├── index.html                        # unchanged (M1)
    ├── embed.go                           # unchanged (M1)
    └── src/
        ├── main.tsx                       # MODIFIED: wrap <App/> in <BrowserRouter>
        ├── App.tsx                        # REPLACED: route table (Overview, RepoDetail, NotFound)
        ├── styles.css                     # NEW: complete app stylesheet (imported once in main.tsx)
        ├── api.ts                         # MODIFIED: typed client for every endpoint + SSE helper
        ├── api.test.ts                    # NEW: URL building + Result typing + SSE helper (Vitest)
        ├── format.ts                      # NEW: pure formatters (dates, hours, relative-time, numbers)
        ├── format.test.ts                 # NEW: formatter unit tests (Vitest)
        ├── hooks/
        │   ├── useAsync.ts                # NEW: generic async-state hook (loading/error/data)
        │   ├── useRepos.ts                # NEW: tracked-repos list + owner/repo→id resolver
        │   └── useRepos.test.tsx          # NEW: resolver + add/remove logic (RTL)
        ├── components/
        │   ├── TimeSeriesChart.tsx        # NEW: thin React wrapper around uPlot (mount/unmount lifecycle)
        │   ├── TimeSeriesChart.test.tsx   # NEW: uPlot mocked; mount/destroy assertions (RTL)
        │   ├── ScalarStat.tsx             # NEW: scalar renderer (value + unit + count)
        │   ├── BucketsBar.tsx             # NEW: buckets renderer (CSS bar chart)
        │   ├── Leaderboard.tsx            # NEW: leaderboard renderer (table)
        │   ├── MetricView.tsx             # NEW: switches on result.kind → the four renderers
        │   ├── MetricView.test.tsx        # NEW: kind-switching + scalar/buckets/leaderboard (RTL)
        │   ├── RepoCard.tsx               # NEW: overview repo card (key numbers + sync status)
        │   ├── AddRepoForm.tsx            # NEW: POST /api/repos form
        │   ├── UserBar.tsx                # NEW: avatar + login + sign-out
        │   ├── WindowControls.tsx         # NEW: window selector + exclude-bots toggle
        │   ├── SyncStatusBadge.tsx        # NEW: sync_status pill
        │   ├── LatestList.tsx             # NEW: latest commits/prs/issues list
        │   └── RefreshButton.tsx          # NEW: POST refresh + open SSE + live progress
        │   └── RefreshButton.test.tsx     # NEW: SSE event handling (EventSource mocked) (RTL)
        └── pages/
            ├── Overview.tsx               # NEW: route "/" — user, add-repo, repo grid
            └── RepoDetail.tsx             # NEW: route "/:owner/:repo" — sections + controls + refresh
```

> All new TS/TSX joins the existing `web/` Vite project. No file outside `web/` changes (no Go edits). Tests live next to their subjects (`*.test.ts(x)`) and are picked up by Vitest's default `include` (`**/*.{test,spec}.{ts,tsx}`). The M1 `src/api.ts` `Me`/`fetchMe` are **kept** and extended, not rewritten.

---

## Task 1: Dependencies & Vitest tooling

**Files:**
- Modify: `web/package.json`, `web/vite.config.ts`, `web/tsconfig.json`
- Create: `web/vitest.setup.ts`

This adds the runtime deps (`react-router-dom`, `uplot`) and the full test stack (`vitest`, `@testing-library/*`, `jsdom`), wires Vitest into `vite.config.ts`, and adds a `test` script. The `test` config uses `environment: "jsdom"`, `globals: true` (so tests call `describe/it/expect/vi` without imports), and a `setupFiles` that loads `@testing-library/jest-dom` and resets `fetch`/`EventSource` mocks between tests.

- [ ] **Step 1: Replace `web/package.json`**

`web/package.json`:
```json
{
  "name": "github-stats-web",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "preview": "vite preview",
    "test": "vitest run",
    "test:watch": "vitest"
  },
  "dependencies": {
    "react": "^18.3.1",
    "react-dom": "^18.3.1",
    "react-router-dom": "^6.26.2",
    "uplot": "^1.6.31"
  },
  "devDependencies": {
    "@testing-library/jest-dom": "^6.5.0",
    "@testing-library/react": "^16.0.1",
    "@testing-library/user-event": "^14.5.2",
    "@types/react": "^18.3.3",
    "@types/react-dom": "^18.3.0",
    "@vitejs/plugin-react": "^4.3.1",
    "jsdom": "^25.0.1",
    "typescript": "^5.5.4",
    "vite": "^5.4.2",
    "vitest": "^2.1.1"
  }
}
```

- [ ] **Step 2: Replace `web/vite.config.ts` (add the `test` block)**

`web/vite.config.ts`:
```ts
/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// In dev, Vite serves the SPA on :5173 and proxies API/auth to the Go server.
// In test, Vitest runs the same config with a jsdom environment.
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
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./vitest.setup.ts"],
    css: false,
    include: ["src/**/*.{test,spec}.{ts,tsx}"],
  },
});
```

- [ ] **Step 3: Create `web/vitest.setup.ts`**

`web/vitest.setup.ts`:
```ts
import "@testing-library/jest-dom/vitest";
import { afterEach, vi } from "vitest";
import { cleanup } from "@testing-library/react";

// Unmount any React tree and clear all mocks after every test for isolation.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
```

- [ ] **Step 4: Update `web/tsconfig.json` to include vitest globals + the setup file**

Replace `web/tsconfig.json`:
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
    "strict": true,
    "types": ["vite/client", "vitest/globals", "@testing-library/jest-dom"]
  },
  "include": ["src", "vitest.setup.ts"],
  "references": [{ "path": "./tsconfig.node.json" }]
}
```

> Note: `tsconfig.node.json` is unchanged from M1 (`emitDeclarationOnly: true`, includes only `vite.config.ts`). The emitted `*.tsbuildinfo` / `vite.config.d.ts` stay gitignored (M1's `.gitignore`). `noEmit: true` on this app project is compatible with `tsc -b` because only the referenced node project is `composite`.

- [ ] **Step 5: Install and verify the toolchain runs**

Run: `cd web && npm install && npx vitest run --passWithNoTests`
Expected: install completes; Vitest boots under jsdom and exits 0 (no test files yet — `--passWithNoTests`).

- [ ] **Step 6: Commit**

```bash
git add web/package.json web/package-lock.json web/vite.config.ts web/vitest.setup.ts web/tsconfig.json
git commit -m "chore(web): add react-router, uplot, and vitest/RTL test stack"
```

---

## Task 2: Pure formatters (`format.ts`)

**Files:**
- Create: `web/src/format.ts`, `web/src/format.test.ts`

Pure, dependency-free formatting helpers used across cards/sections: `fmtDate` (ISO date → short label), `fmtRelative` (timestamp → "3h ago"), `fmtHours` (hours float → "12.5h" / "1.3d"), `fmtRate` (per-day float → "2.4/day"), `fmtNumber` (thousands separators), `fmtNullableTs` (string|null → relative or "never"). All deterministic with an injectable `now` so tests never touch the wall clock.

- [ ] **Step 1: Write the failing test**

`web/src/format.test.ts`:
```ts
import { describe, it, expect } from "vitest";
import {
  fmtDate,
  fmtRelative,
  fmtHours,
  fmtRate,
  fmtNumber,
  fmtNullableTs,
} from "./format";

const NOW = new Date("2026-05-31T12:00:00Z").getTime();

describe("format", () => {
  it("fmtDate renders a short YYYY-MM-DD-ish label", () => {
    expect(fmtDate("2026-05-09")).toBe("May 9, 2026");
  });

  it("fmtRelative renders coarse buckets", () => {
    expect(fmtRelative("2026-05-31T11:00:00Z", NOW)).toBe("1h ago");
    expect(fmtRelative("2026-05-31T11:59:30Z", NOW)).toBe("just now");
    expect(fmtRelative("2026-05-29T12:00:00Z", NOW)).toBe("2d ago");
  });

  it("fmtHours switches to days past 48h", () => {
    expect(fmtHours(12.5)).toBe("12.5h");
    expect(fmtHours(0)).toBe("0h");
    expect(fmtHours(72)).toBe("3.0d");
  });

  it("fmtRate formats per-day rates", () => {
    expect(fmtRate(2.444)).toBe("2.4/day");
    expect(fmtRate(0)).toBe("0/day");
  });

  it("fmtNumber adds thousands separators", () => {
    expect(fmtNumber(1234567)).toBe("1,234,567");
    expect(fmtNumber(42)).toBe("42");
  });

  it("fmtNullableTs handles null as 'never'", () => {
    expect(fmtNullableTs(null, NOW)).toBe("never");
    expect(fmtNullableTs("2026-05-31T11:00:00Z", NOW)).toBe("1h ago");
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/format.test.ts`
Expected: FAIL — cannot resolve `./format`.

- [ ] **Step 3: Write the implementation**

`web/src/format.ts`:
```ts
// Pure formatting helpers. `now` is injectable for deterministic tests.

const MONTHS = [
  "Jan", "Feb", "Mar", "Apr", "May", "Jun",
  "Jul", "Aug", "Sep", "Oct", "Nov", "Dec",
];

const MONTHS_LONG = [
  "January", "February", "March", "April", "May", "June",
  "July", "August", "September", "October", "November", "December",
];

/** "2026-05-09" -> "May 9, 2026". Falls back to the raw string if unparseable. */
export function fmtDate(iso: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(iso);
  if (!m) return iso;
  const year = Number(m[1]);
  const month = Number(m[2]) - 1;
  const day = Number(m[3]);
  if (month < 0 || month > 11) return iso;
  return `${MONTHS_LONG[month]} ${day}, ${year}`;
}

/** Short month-day for chart axes: "2026-05-09" -> "May 9". */
export function fmtDateShort(iso: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(iso);
  if (!m) return iso;
  const month = Number(m[2]) - 1;
  const day = Number(m[3]);
  if (month < 0 || month > 11) return iso;
  return `${MONTHS[month]} ${day}`;
}

/** Coarse relative time: "just now", "5m ago", "3h ago", "2d ago", "4mo ago". */
export function fmtRelative(iso: string, now: number = Date.now()): string {
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return iso;
  const secs = Math.max(0, Math.floor((now - then) / 1000));
  if (secs < 60) return "just now";
  const mins = Math.floor(secs / 60);
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  const days = Math.floor(hrs / 24);
  if (days < 30) return `${days}d ago`;
  const months = Math.floor(days / 30);
  if (months < 12) return `${months}mo ago`;
  return `${Math.floor(months / 12)}y ago`;
}

/** Hours float -> "12.5h"; switches to days at 48h: "3.0d". */
export function fmtHours(hours: number): string {
  if (!Number.isFinite(hours)) return "—";
  if (hours >= 48) return `${(hours / 24).toFixed(1)}d`;
  const rounded = Math.round(hours * 10) / 10;
  return Number.isInteger(rounded) ? `${rounded}h` : `${rounded.toFixed(1)}h`;
}

/** Per-day rate -> "2.4/day". */
export function fmtRate(perDay: number): string {
  if (!Number.isFinite(perDay)) return "—";
  const rounded = Math.round(perDay * 10) / 10;
  return Number.isInteger(rounded) ? `${rounded}/day` : `${rounded.toFixed(1)}/day`;
}

/** Integer with thousands separators. */
export function fmtNumber(n: number): string {
  return Math.round(n).toLocaleString("en-US");
}

/** string|null timestamp -> relative time, or "never" when null/empty. */
export function fmtNullableTs(iso: string | null, now: number = Date.now()): string {
  if (!iso) return "never";
  return fmtRelative(iso, now);
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/format.test.ts`
Expected: PASS (all six cases).

- [ ] **Step 5: Commit**

```bash
git add web/src/format.ts web/src/format.test.ts
git commit -m "feat(web): pure formatting helpers with tests"
```

---

## Task 3: Typed API client + SSE helper (`api.ts`)

**Files:**
- Modify: `web/src/api.ts` (keep `Me`/`fetchMe`, add everything else)
- Create: `web/src/api.test.ts`

Types every endpoint precisely against the M3/M4 contracts and exposes one typed function per endpoint plus an SSE helper wrapping `EventSource`. The pure logic — URL building (CSV `keys`, `window`, `exclude_bots`, `limit`), JSON typing of the `Result` union, and the SSE `onEvent` plumbing — is unit-tested with `fetch`/`EventSource` mocked.

- [ ] **Step 1: Write the failing test**

`web/src/api.test.ts`:
```ts
import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  fetchMe,
  listRepos,
  addRepo,
  deleteRepo,
  refreshRepo,
  fetchOverview,
  fetchMetrics,
  fetchLatest,
  openSyncStream,
  metricsURL,
  overviewURL,
  latestURL,
  type Result,
} from "./api";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("URL builders", () => {
  it("metricsURL encodes keys csv, window, exclude_bots", () => {
    const u = metricsURL(7, {
      keys: ["commit_rate", "time_to_merge"],
      window: "90d",
      excludeBots: true,
    });
    expect(u).toBe(
      "/api/repos/7/metrics?keys=commit_rate%2Ctime_to_merge&window=90d&exclude_bots=true",
    );
  });

  it("metricsURL omits keys when empty (server returns all)", () => {
    expect(metricsURL(3, { keys: [], window: "30d", excludeBots: false })).toBe(
      "/api/repos/3/metrics?window=30d&exclude_bots=false",
    );
  });

  it("overviewURL encodes window + exclude_bots", () => {
    expect(overviewURL(5, { window: "all", excludeBots: true })).toBe(
      "/api/repos/5?window=all&exclude_bots=true",
    );
  });

  it("latestURL encodes kind + limit", () => {
    expect(latestURL(9, "prs", 50)).toBe("/api/repos/9/latest/prs?limit=50");
  });
});

describe("fetch wrappers", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  it("fetchMe returns null on 401", async () => {
    (fetch as ReturnType<typeof vi.fn>).mockResolvedValue(new Response("", { status: 401 }));
    expect(await fetchMe()).toBeNull();
  });

  it("listRepos parses the repo array", async () => {
    (fetch as ReturnType<typeof vi.fn>).mockResolvedValue(
      jsonResponse([
        {
          id: 1,
          full_name: "octocat/hello",
          is_private: false,
          default_branch: "main",
          sync_status: "complete",
          last_synced_at: "2026-05-31T10:00:00Z",
        },
      ]),
    );
    const repos = await listRepos();
    expect(repos).toHaveLength(1);
    expect(repos[0].full_name).toBe("octocat/hello");
  });

  it("addRepo POSTs the full_name body", async () => {
    const f = fetch as ReturnType<typeof vi.fn>;
    f.mockResolvedValue(
      jsonResponse({ id: 2, full_name: "a/b", is_private: true, default_branch: "main", sync_status: "pending", last_synced_at: null }, 201),
    );
    const repo = await addRepo("a/b");
    expect(repo.id).toBe(2);
    const [url, init] = f.mock.calls[0];
    expect(url).toBe("/api/repos");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body)).toEqual({ full_name: "a/b" });
  });

  it("addRepo throws with the server message on error", async () => {
    (fetch as ReturnType<typeof vi.fn>).mockResolvedValue(
      new Response("full_name must be owner/name", { status: 400 }),
    );
    await expect(addRepo("bad")).rejects.toThrow(/owner\/name/);
  });

  it("deleteRepo issues DELETE and resolves on 204", async () => {
    const f = fetch as ReturnType<typeof vi.fn>;
    f.mockResolvedValue(new Response(null, { status: 204 }));
    await deleteRepo(4);
    expect(f.mock.calls[0][0]).toBe("/api/repos/4");
    expect(f.mock.calls[0][1].method).toBe("DELETE");
  });

  it("refreshRepo issues POST and resolves on 202", async () => {
    const f = fetch as ReturnType<typeof vi.fn>;
    f.mockResolvedValue(new Response(null, { status: 202 }));
    await refreshRepo(6);
    expect(f.mock.calls[0][1].method).toBe("POST");
  });

  it("fetchMetrics returns a keyed Result map", async () => {
    const body: Record<string, Result> = {
      commit_rate: { kind: "time_series", label: "Commits/day", series: [{ date: "2026-05-01", value: 3 }] },
      time_to_merge: { kind: "scalar", label: "median", value: 12.5, unit: "hours", count: 4 },
    };
    (fetch as ReturnType<typeof vi.fn>).mockResolvedValue(jsonResponse(body));
    const out = await fetchMetrics(7, { keys: ["commit_rate", "time_to_merge"], window: "30d", excludeBots: false });
    expect(out.commit_rate.kind).toBe("time_series");
    expect(out.time_to_merge.kind).toBe("scalar");
  });

  it("fetchOverview parses the bundle", async () => {
    (fetch as ReturnType<typeof vi.fn>).mockResolvedValue(
      jsonResponse({
        id: 7, full_name: "o/r", is_private: false, default_branch: "main", description: "",
        stargazers: 10, forks: 2, open_issues: 5, open_prs: 1, contributors: 3,
        commit_rate: 2.4, issue_rate: 0.5, pr_rate: 0.3, releases: 2,
        sync_status: "complete", last_synced_at: null,
        window_from: "2026-05-01", window_to: "2026-05-31",
      }),
    );
    const ov = await fetchOverview(7, { window: "30d", excludeBots: false });
    expect(ov.contributors).toBe(3);
    expect(ov.window_to).toBe("2026-05-31");
  });

  it("fetchLatest parses commit rows", async () => {
    (fetch as ReturnType<typeof vi.fn>).mockResolvedValue(
      jsonResponse([
        { sha: "abc", author_login: "neo", committed_at: "2026-05-30T00:00:00Z", additions: 4, deletions: 1, is_bot: false, msg_first_line: "fix" },
      ]),
    );
    const rows = await fetchLatest(7, "commits", 20);
    expect(rows).toHaveLength(1);
    expect((rows[0] as { sha: string }).sha).toBe("abc");
  });
});

describe("openSyncStream", () => {
  it("wires EventSource onmessage to onEvent and close() to it", () => {
    const closed = { v: false };
    const fake = {
      onmessage: null as ((e: MessageEvent) => void) | null,
      onerror: null as ((e: Event) => void) | null,
      close() { closed.v = true; },
    };
    const ctor = vi.fn(() => fake);
    vi.stubGlobal("EventSource", ctor as unknown as typeof EventSource);

    const events: unknown[] = [];
    const handle = openSyncStream(7, {
      onEvent: (ev) => events.push(ev),
    });
    expect(ctor).toHaveBeenCalledWith("/api/repos/7/sync/stream");

    fake.onmessage?.({ data: JSON.stringify({ repo_id: 7, phase: "commits", message: "page 1", done: false }) } as MessageEvent);
    expect(events).toEqual([{ repo_id: 7, phase: "commits", message: "page 1", done: false }]);

    handle.close();
    expect(closed.v).toBe(true);
  });

  it("auto-closes and calls onDone when a done event arrives", () => {
    const closed = { v: false };
    const fake = {
      onmessage: null as ((e: MessageEvent) => void) | null,
      onerror: null as ((e: Event) => void) | null,
      close() { closed.v = true; },
    };
    vi.stubGlobal("EventSource", vi.fn(() => fake) as unknown as typeof EventSource);

    let done = false;
    openSyncStream(7, { onEvent: () => {}, onDone: () => { done = true; } });
    fake.onmessage?.({ data: JSON.stringify({ repo_id: 7, phase: "done", message: "complete", done: true }) } as MessageEvent);

    expect(done).toBe(true);
    expect(closed.v).toBe(true);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/api.test.ts`
Expected: FAIL — `metricsURL` / `listRepos` / `openSyncStream` are not exported.

- [ ] **Step 3: Write the implementation**

`web/src/api.ts`:
```ts
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/api.test.ts`
Expected: PASS (all builder, fetch, and SSE cases).

- [ ] **Step 5: Commit**

```bash
git add web/src/api.ts web/src/api.test.ts
git commit -m "feat(web): typed API client for repos/metrics/overview/latest + SSE helper"
```

---

## Task 4: App stylesheet (`styles.css`)

**Files:**
- Create: `web/src/styles.css`

A single complete stylesheet (imported once from `main.tsx` in Task 13) covering the app shell, cards, grid, forms, sections, tables, chart container, sync badges, and the live-progress log. No framework — plain CSS with a small token set. **Verification for this task is "typecheck + build passes"** (this is pure presentation; a DOM snapshot would be brittle), exercised at the end of Task 14.

- [ ] **Step 1: Write `web/src/styles.css`**

`web/src/styles.css`:
```css
:root {
  --bg: #0d1117;
  --surface: #161b22;
  --surface-2: #21262d;
  --border: #30363d;
  --text: #e6edf3;
  --muted: #8b949e;
  --accent: #2f81f7;
  --accent-2: #1f6feb;
  --green: #2ea043;
  --amber: #d29922;
  --red: #f85149;
  --radius: 8px;
  --gap: 16px;
  font-family: system-ui, -apple-system, "Segoe UI", Roboto, sans-serif;
}

* { box-sizing: border-box; }

body {
  margin: 0;
  background: var(--bg);
  color: var(--text);
  line-height: 1.5;
}

a { color: var(--accent); text-decoration: none; }
a:hover { text-decoration: underline; }

button {
  font: inherit;
  cursor: pointer;
  border-radius: 6px;
  border: 1px solid var(--border);
  background: var(--surface-2);
  color: var(--text);
  padding: 6px 12px;
}
button:hover:not(:disabled) { border-color: var(--accent); }
button:disabled { opacity: 0.5; cursor: default; }

button.primary { background: var(--accent-2); border-color: var(--accent-2); color: #fff; }
button.primary:hover:not(:disabled) { background: var(--accent); }

input, select {
  font: inherit;
  background: var(--surface);
  color: var(--text);
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 6px 10px;
}

.app-shell { max-width: 1100px; margin: 0 auto; padding: 24px 20px 64px; }

/* User bar */
.user-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 24px;
  padding-bottom: 16px;
  border-bottom: 1px solid var(--border);
}
.user-bar .who { display: flex; align-items: center; gap: 10px; }
.user-bar img { width: 32px; height: 32px; border-radius: 50%; }
.brand { font-size: 18px; font-weight: 700; }

/* Add-repo form */
.add-repo {
  display: flex;
  gap: 8px;
  margin: 20px 0;
}
.add-repo input { flex: 1; min-width: 0; }
.form-error { color: var(--red); margin: 4px 0 0; font-size: 13px; }

/* Repo grid + cards */
.repo-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: var(--gap);
}
.repo-card {
  display: block;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 16px;
  color: var(--text);
}
.repo-card:hover { border-color: var(--accent); text-decoration: none; }
.repo-card h3 { margin: 0 0 4px; font-size: 15px; word-break: break-all; }
.repo-card .desc { color: var(--muted); font-size: 13px; margin: 0 0 12px; min-height: 18px; }
.repo-card .stats { display: grid; grid-template-columns: repeat(3, 1fr); gap: 8px; margin-bottom: 12px; }
.repo-card .stat { background: var(--surface-2); border-radius: 6px; padding: 8px; text-align: center; }
.repo-card .stat .n { font-size: 18px; font-weight: 700; display: block; }
.repo-card .stat .l { font-size: 11px; color: var(--muted); }
.repo-card .card-foot { display: flex; align-items: center; justify-content: space-between; font-size: 12px; color: var(--muted); }

/* Sync status badge */
.badge {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  font-size: 12px;
  font-weight: 600;
  padding: 2px 8px;
  border-radius: 999px;
  border: 1px solid var(--border);
  background: var(--surface-2);
}
.badge .dot { width: 7px; height: 7px; border-radius: 50%; background: var(--muted); }
.badge.complete .dot { background: var(--green); }
.badge.running .dot { background: var(--accent); animation: pulse 1.2s infinite; }
.badge.pending .dot { background: var(--amber); }
.badge.error .dot { background: var(--red); }
@keyframes pulse { 0%,100% { opacity: 1; } 50% { opacity: 0.3; } }

/* Repo detail */
.detail-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; margin: 16px 0 8px; flex-wrap: wrap; }
.detail-head h1 { margin: 0; font-size: 22px; word-break: break-all; }
.detail-head .sub { color: var(--muted); font-size: 13px; }

.controls { display: flex; align-items: center; gap: 16px; flex-wrap: wrap; margin: 16px 0; }
.controls label { display: inline-flex; align-items: center; gap: 6px; color: var(--muted); font-size: 13px; }

.section { margin: 28px 0; }
.section > h2 { font-size: 16px; border-bottom: 1px solid var(--border); padding-bottom: 6px; margin: 0 0 16px; }

.metric-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(260px, 1fr)); gap: var(--gap); }

.metric-card {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 14px 16px;
}
.metric-card > h3 { margin: 0 0 10px; font-size: 13px; color: var(--muted); font-weight: 600; text-transform: uppercase; letter-spacing: 0.03em; }

/* Scalar stat */
.scalar { display: flex; align-items: baseline; gap: 8px; }
.scalar .value { font-size: 28px; font-weight: 700; }
.scalar .unit { color: var(--muted); font-size: 14px; }
.scalar .count { color: var(--muted); font-size: 12px; margin-top: 4px; }

/* Chart container */
.chart { width: 100%; }
.chart .uplot { width: 100% !important; }
.empty { color: var(--muted); font-size: 13px; font-style: italic; padding: 12px 0; }

/* Buckets bar chart */
.buckets { display: flex; flex-direction: column; gap: 6px; }
.buckets .row { display: grid; grid-template-columns: 64px 1fr 40px; align-items: center; gap: 8px; font-size: 12px; }
.buckets .bar { background: var(--surface-2); border-radius: 4px; height: 14px; overflow: hidden; }
.buckets .bar > span { display: block; height: 100%; background: var(--accent); }
.buckets .label { color: var(--muted); }
.buckets .n { text-align: right; }

/* Leaderboard + latest tables */
table.data { width: 100%; border-collapse: collapse; font-size: 13px; }
table.data th, table.data td { text-align: left; padding: 6px 8px; border-bottom: 1px solid var(--border); }
table.data th { color: var(--muted); font-weight: 600; }
table.data td.num { text-align: right; font-variant-numeric: tabular-nums; }

.latest-item { display: flex; gap: 10px; padding: 8px 0; border-bottom: 1px solid var(--border); font-size: 13px; }
.latest-item .meta { color: var(--muted); font-size: 12px; white-space: nowrap; }
.latest-item .title { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

/* Refresh + live progress */
.refresh-box { display: flex; flex-direction: column; gap: 8px; }
.progress-log {
  background: #010409;
  border: 1px solid var(--border);
  border-radius: 6px;
  padding: 8px 10px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  max-height: 160px;
  overflow-y: auto;
}
.progress-log .line { color: var(--muted); }
.progress-log .line.error { color: var(--red); }
.progress-log .line.done { color: var(--green); }

/* States */
.state { padding: 40px 0; text-align: center; color: var(--muted); }
.state.error { color: var(--red); }
.notice { background: var(--surface-2); border: 1px solid var(--border); border-radius: var(--radius); padding: 16px; }
```

- [ ] **Step 2: Commit (verified by build in Task 14)**

```bash
git add web/src/styles.css
git commit -m "feat(web): app stylesheet"
```

---

## Task 5: `useAsync` hook

**Files:**
- Create: `web/src/hooks/useAsync.ts`

A tiny generic hook that runs an async function and tracks `{ data, error, loading }`, re-running when its dependency key changes, and ignoring stale resolutions (race-safe via a per-run token). Used by every data-loading view. **Verification: typecheck (compiled by the consuming tests in Tasks 6/11/12 and the Task 14 build).**

- [ ] **Step 1: Write `web/src/hooks/useAsync.ts`**

`web/src/hooks/useAsync.ts`:
```ts
import { useEffect, useState, useCallback } from "react";

export interface AsyncState<T> {
  data: T | null;
  error: Error | null;
  loading: boolean;
  reload: () => void;
}

/**
 * useAsync runs `fn` whenever any value in `deps` changes (and on a manual
 * reload()), exposing loading/error/data. Stale results are discarded so a
 * fast-changing key never renders an out-of-order response.
 */
export function useAsync<T>(fn: () => Promise<T>, deps: unknown[]): AsyncState<T> {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<Error | null>(null);
  const [loading, setLoading] = useState(true);
  const [nonce, setNonce] = useState(0);

  const reload = useCallback(() => setNonce((n) => n + 1), []);

  useEffect(() => {
    let active = true;
    setLoading(true);
    setError(null);
    fn()
      .then((res) => {
        if (active) {
          setData(res);
          setLoading(false);
        }
      })
      .catch((e: unknown) => {
        if (active) {
          setError(e instanceof Error ? e : new Error(String(e)));
          setLoading(false);
        }
      });
    return () => {
      active = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, nonce]);

  return { data, error, loading, reload };
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/hooks/useAsync.ts
git commit -m "feat(web): race-safe useAsync hook"
```

---

## Task 6: `useRepos` hook — list + `owner/repo`→id resolver

**Files:**
- Create: `web/src/hooks/useRepos.ts`, `web/src/hooks/useRepos.test.tsx`

`useRepos` loads the tracked-repos list (`listRepos`) and exposes a `resolve(owner, repo)` that maps a `full_name` ("owner/repo", case-insensitive) to its tracked `Repo` (or `null` if untracked — the RepoDetail page uses this to show an "add this repo" affordance). It also exposes `add`/`remove` that mutate the list optimistically and `reload`. The resolver and add/remove logic are the tested units.

- [ ] **Step 1: Write the failing test**

`web/src/hooks/useRepos.test.tsx`:
```tsx
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";
import { useRepos } from "./useRepos";
import * as api from "../api";

const REPOS: api.Repo[] = [
  { id: 1, full_name: "octocat/Hello-World", is_private: false, default_branch: "main", sync_status: "complete", last_synced_at: null },
  { id: 2, full_name: "facebook/react", is_private: false, default_branch: "main", sync_status: "running", last_synced_at: null },
];

describe("useRepos", () => {
  beforeEach(() => {
    vi.spyOn(api, "listRepos").mockResolvedValue(REPOS);
  });

  it("loads repos and resolves owner/repo case-insensitively", async () => {
    const { result } = renderHook(() => useRepos());
    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(result.current.repos).toHaveLength(2);
    expect(result.current.resolve("octocat", "hello-world")?.id).toBe(1);
    expect(result.current.resolve("FACEBOOK", "REACT")?.id).toBe(2);
    expect(result.current.resolve("nobody", "nothing")).toBeNull();
  });

  it("add() appends a repo via the API and updates the list", async () => {
    const added: api.Repo = { id: 3, full_name: "a/b", is_private: true, default_branch: "main", sync_status: "pending", last_synced_at: null };
    vi.spyOn(api, "addRepo").mockResolvedValue(added);

    const { result } = renderHook(() => useRepos());
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.add("a/b");
    });
    expect(api.addRepo).toHaveBeenCalledWith("a/b");
    expect(result.current.repos.map((r) => r.id)).toContain(3);
  });

  it("remove() deletes a repo and drops it from the list", async () => {
    vi.spyOn(api, "deleteRepo").mockResolvedValue();
    const { result } = renderHook(() => useRepos());
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.remove(1);
    });
    expect(api.deleteRepo).toHaveBeenCalledWith(1);
    expect(result.current.repos.map((r) => r.id)).toEqual([2]);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/hooks/useRepos.test.tsx`
Expected: FAIL — cannot resolve `./useRepos`.

- [ ] **Step 3: Write the implementation**

`web/src/hooks/useRepos.ts`:
```ts
import { useCallback, useEffect, useState } from "react";
import { listRepos, addRepo, deleteRepo, type Repo } from "../api";

export interface UseRepos {
  repos: Repo[];
  loading: boolean;
  error: Error | null;
  reload: () => void;
  resolve: (owner: string, repo: string) => Repo | null;
  add: (fullName: string) => Promise<Repo>;
  remove: (id: number) => Promise<void>;
}

export function useRepos(): UseRepos {
  const [repos, setRepos] = useState<Repo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [nonce, setNonce] = useState(0);

  const reload = useCallback(() => setNonce((n) => n + 1), []);

  useEffect(() => {
    let active = true;
    setLoading(true);
    setError(null);
    listRepos()
      .then((rs) => {
        if (active) {
          setRepos(rs);
          setLoading(false);
        }
      })
      .catch((e: unknown) => {
        if (active) {
          setError(e instanceof Error ? e : new Error(String(e)));
          setLoading(false);
        }
      });
    return () => {
      active = false;
    };
  }, [nonce]);

  const resolve = useCallback(
    (owner: string, repo: string): Repo | null => {
      const target = `${owner}/${repo}`.toLowerCase();
      return repos.find((r) => r.full_name.toLowerCase() === target) ?? null;
    },
    [repos],
  );

  const add = useCallback(async (fullName: string): Promise<Repo> => {
    const created = await addRepo(fullName);
    setRepos((prev) => {
      const without = prev.filter((r) => r.id !== created.id);
      return [created, ...without];
    });
    return created;
  }, []);

  const remove = useCallback(async (id: number): Promise<void> => {
    await deleteRepo(id);
    setRepos((prev) => prev.filter((r) => r.id !== id));
  }, []);

  return { repos, loading, error, reload, resolve, add, remove };
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/hooks/useRepos.test.tsx`
Expected: PASS (resolve case-insensitivity, add, remove).

- [ ] **Step 5: Commit**

```bash
git add web/src/hooks/useRepos.ts web/src/hooks/useRepos.test.tsx
git commit -m "feat(web): useRepos hook with owner/repo resolver"
```

---

## Task 7: `TimeSeriesChart` — uPlot wrapper

**Files:**
- Create: `web/src/components/TimeSeriesChart.tsx`, `web/src/components/TimeSeriesChart.test.tsx`

A thin React wrapper around uPlot: it constructs a uPlot instance into a container `div` on mount, updates `setData` on prop change, resizes to the container width, and **destroys** the instance on unmount (the lifecycle bug class this test guards against). The test mocks the `uplot` default export so it runs in jsdom (which has no canvas), asserting the constructor is called once, `setData` runs on data change, and `destroy` runs on unmount.

- [ ] **Step 1: Write the failing test**

`web/src/components/TimeSeriesChart.test.tsx`:
```tsx
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, cleanup } from "@testing-library/react";
import TimeSeriesChart from "./TimeSeriesChart";
import type { SeriesPoint } from "../api";

// Mock uPlot: it touches canvas, which jsdom lacks. Capture instance methods.
const instances: Array<{ setData: ReturnType<typeof vi.fn>; setSize: ReturnType<typeof vi.fn>; destroy: ReturnType<typeof vi.fn> }> = [];
vi.mock("uplot", () => {
  return {
    default: vi.fn().mockImplementation(() => {
      const inst = { setData: vi.fn(), setSize: vi.fn(), destroy: vi.fn() };
      instances.push(inst);
      return inst;
    }),
  };
});

const SERIES: SeriesPoint[] = [
  { date: "2026-05-01", value: 3 },
  { date: "2026-05-02", value: 5 },
];

describe("TimeSeriesChart", () => {
  beforeEach(() => {
    instances.length = 0;
  });

  it("constructs a uPlot instance on mount", () => {
    render(<TimeSeriesChart series={SERIES} label="Commits/day" />);
    expect(instances).toHaveLength(1);
  });

  it("destroys the uPlot instance on unmount", () => {
    const { unmount } = render(<TimeSeriesChart series={SERIES} label="Commits/day" />);
    const inst = instances[0];
    unmount();
    expect(inst.destroy).toHaveBeenCalledTimes(1);
  });

  it("renders an empty state for an empty series without constructing uPlot", () => {
    const { getByText } = render(<TimeSeriesChart series={[]} label="Commits/day" />);
    expect(getByText(/no data/i)).toBeInTheDocument();
    expect(instances).toHaveLength(0);
  });

  it("pushes new data via setData when series changes", () => {
    const { rerender } = render(<TimeSeriesChart series={SERIES} label="x" />);
    const inst = instances[0];
    rerender(<TimeSeriesChart series={[...SERIES, { date: "2026-05-03", value: 9 }]} label="x" />);
    expect(inst.setData).toHaveBeenCalled();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/components/TimeSeriesChart.test.tsx`
Expected: FAIL — cannot resolve `./TimeSeriesChart`.

- [ ] **Step 3: Write the implementation**

`web/src/components/TimeSeriesChart.tsx`:
```tsx
import { useEffect, useRef } from "react";
import uPlot from "uplot";
import "uplot/dist/uPlot.min.css";
import type { SeriesPoint } from "../api";

interface Props {
  series: SeriesPoint[];
  label?: string;
  height?: number;
}

// Convert ISO date points into uPlot's columnar [xs, ys] with unix-second xs.
function toData(series: SeriesPoint[]): uPlot.AlignedData {
  const xs: number[] = [];
  const ys: number[] = [];
  for (const p of series) {
    xs.push(Date.parse(p.date + "T00:00:00Z") / 1000);
    ys.push(p.value);
  }
  return [xs, ys];
}

function makeOpts(width: number, height: number, label: string): uPlot.Options {
  return {
    width,
    height,
    cursor: { y: false },
    legend: { show: false },
    scales: { x: { time: true } },
    axes: [
      { stroke: "#8b949e", grid: { stroke: "#21262d" }, ticks: { stroke: "#30363d" } },
      { stroke: "#8b949e", grid: { stroke: "#21262d" }, ticks: { stroke: "#30363d" }, size: 44 },
    ],
    series: [
      {},
      { label, stroke: "#2f81f7", width: 2, fill: "rgba(47,129,247,0.12)", points: { show: false } },
    ],
  };
}

/**
 * TimeSeriesChart is a thin lifecycle wrapper around uPlot. It creates one
 * instance on mount, resizes it to the container, updates data when `series`
 * changes, and destroys it on unmount. An empty series renders a placeholder
 * (and never constructs uPlot) so jsdom/test and empty-data paths are safe.
 */
export default function TimeSeriesChart({ series, label = "", height = 200 }: Props) {
  const ref = useRef<HTMLDivElement | null>(null);
  const plotRef = useRef<uPlot | null>(null);

  // Create on mount; destroy on unmount. Re-create if it was previously empty.
  useEffect(() => {
    if (series.length === 0) return;
    const el = ref.current;
    if (!el) return;
    const width = el.clientWidth || 600;
    const plot = new uPlot(makeOpts(width, height, label), toData(series), el);
    plotRef.current = plot;

    const onResize = () => plot.setSize({ width: el.clientWidth || width, height });
    window.addEventListener("resize", onResize);
    return () => {
      window.removeEventListener("resize", onResize);
      plot.destroy();
      plotRef.current = null;
    };
    // Recreate only when the series identity transitions to/from empty or label/height change.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [series.length === 0, label, height]);

  // Push data updates without tearing down the instance.
  useEffect(() => {
    if (plotRef.current && series.length > 0) {
      plotRef.current.setData(toData(series));
    }
  }, [series]);

  if (series.length === 0) {
    return <p className="empty">No data for this window.</p>;
  }
  return <div className="chart" ref={ref} />;
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/components/TimeSeriesChart.test.tsx`
Expected: PASS — construct on mount, destroy on unmount, empty-state placeholder, setData on change.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/TimeSeriesChart.tsx web/src/components/TimeSeriesChart.test.tsx
git commit -m "feat(web): uPlot TimeSeriesChart wrapper with lifecycle test"
```

---

## Task 8: `ScalarStat`, `BucketsBar`, `Leaderboard` renderers

**Files:**
- Create: `web/src/components/ScalarStat.tsx`, `web/src/components/BucketsBar.tsx`, `web/src/components/Leaderboard.tsx`

The three non-chart renderers, each taking exactly the matching `Result` subtype. `ScalarStat` shows value + unit (hours formatted via `fmtHours` when `unit==="hours"`) and the sample `count`. `BucketsBar` draws a CSS bar chart (bar width ∝ count/max). `Leaderboard` renders a ranked table. Tested together with `MetricView` in Task 9.

- [ ] **Step 1: Write `web/src/components/ScalarStat.tsx`**

```tsx
import type { ScalarResult } from "../api";
import { fmtHours, fmtNumber } from "../format";

interface Props {
  result: ScalarResult;
}

export default function ScalarStat({ result }: Props) {
  const hasValue = typeof result.value === "number";
  const isHours = result.unit === "hours";
  const display = !hasValue
    ? "—"
    : isHours
      ? fmtHours(result.value as number)
      : fmtNumber(result.value as number);
  const unitLabel = isHours ? "" : (result.unit ?? "");
  return (
    <div className="scalar-wrap">
      <div className="scalar">
        <span className="value">{display}</span>
        {unitLabel && <span className="unit">{unitLabel}</span>}
      </div>
      {typeof result.count === "number" && (
        <div className="count">n = {fmtNumber(result.count)}</div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Write `web/src/components/BucketsBar.tsx`**

```tsx
import type { BucketsResult } from "../api";
import { fmtNumber } from "../format";

interface Props {
  result: BucketsResult;
}

export default function BucketsBar({ result }: Props) {
  const buckets = result.buckets ?? [];
  if (buckets.length === 0) return <p className="empty">No data for this window.</p>;
  const max = Math.max(1, ...buckets.map((b) => b.count));
  return (
    <div className="buckets">
      {buckets.map((b) => (
        <div className="row" key={b.label}>
          <span className="label">{b.label}</span>
          <span className="bar">
            <span style={{ width: `${(b.count / max) * 100}%` }} />
          </span>
          <span className="n">{fmtNumber(b.count)}</span>
        </div>
      ))}
    </div>
  );
}
```

- [ ] **Step 3: Write `web/src/components/Leaderboard.tsx`**

```tsx
import type { LeaderboardResult } from "../api";
import { fmtNumber } from "../format";

interface Props {
  result: LeaderboardResult;
}

export default function Leaderboard({ result }: Props) {
  const rows = result.rows ?? [];
  if (rows.length === 0) return <p className="empty">No contributors in this window.</p>;
  return (
    <table className="data">
      <thead>
        <tr>
          <th>#</th>
          <th>Contributor</th>
          <th className="num">Commits</th>
          <th className="num">+</th>
          <th className="num">−</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((r, i) => (
          <tr key={r.login}>
            <td>{i + 1}</td>
            <td>{r.login}</td>
            <td className="num">{fmtNumber(r.commits)}</td>
            <td className="num">{fmtNumber(r.additions)}</td>
            <td className="num">{fmtNumber(r.deletions)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
```

- [ ] **Step 4: Commit**

```bash
git add web/src/components/ScalarStat.tsx web/src/components/BucketsBar.tsx web/src/components/Leaderboard.tsx
git commit -m "feat(web): scalar/buckets/leaderboard result renderers"
```

---

## Task 9: `MetricView` — switch on `result.kind`

**Files:**
- Create: `web/src/components/MetricView.tsx`, `web/src/components/MetricView.test.tsx`

`MetricView` is the single dispatch point: given a `Result`, it renders `TimeSeriesChart` / `ScalarStat` / `BucketsBar` / `Leaderboard` based on `result.kind`, with an exhaustive `switch` and a fallback for unknown kinds. The test asserts the switching plus the scalar/buckets/leaderboard rendered output (the chart branch is covered by Task 7's mounted-uPlot test, so here it's only asserted that the chart container appears for `time_series`).

- [ ] **Step 1: Write the failing test**

`web/src/components/MetricView.test.tsx`:
```tsx
import { describe, it, expect, vi } from "vitest";
import { render } from "@testing-library/react";
import MetricView from "./MetricView";
import type { Result } from "../api";

// Stub the uPlot-backed chart so the time_series branch needs no canvas.
vi.mock("./TimeSeriesChart", () => ({
  default: ({ label }: { label?: string }) => <div data-testid="ts-chart">{label}</div>,
}));

describe("MetricView kind switching", () => {
  it("renders the chart for time_series", () => {
    const r: Result = { kind: "time_series", label: "Commits/day", series: [{ date: "2026-05-01", value: 1 }] };
    const { getByTestId } = render(<MetricView result={r} />);
    expect(getByTestId("ts-chart")).toHaveTextContent("Commits/day");
  });

  it("renders a formatted hours value for scalar", () => {
    const r: Result = { kind: "scalar", label: "median", value: 12.5, unit: "hours", count: 4 };
    const { getByText } = render(<MetricView result={r} />);
    expect(getByText("12.5h")).toBeInTheDocument();
    expect(getByText(/n = 4/)).toBeInTheDocument();
  });

  it("renders bucket rows for buckets", () => {
    const r: Result = { kind: "buckets", label: "Open issue age", buckets: [
      { label: "<24h", count: 2 },
      { label: "older", count: 9 },
    ] };
    const { getByText } = render(<MetricView result={r} />);
    expect(getByText("<24h")).toBeInTheDocument();
    expect(getByText("older")).toBeInTheDocument();
  });

  it("renders contributor rows for leaderboard", () => {
    const r: Result = { kind: "leaderboard", label: "Top contributors", rows: [
      { login: "neo", commits: 12, additions: 100, deletions: 5 },
    ] };
    const { getByText } = render(<MetricView result={r} />);
    expect(getByText("neo")).toBeInTheDocument();
    expect(getByText("12")).toBeInTheDocument();
  });

  it("falls back gracefully for an unknown kind", () => {
    const r = { kind: "mystery" } as unknown as Result;
    const { getByText } = render(<MetricView result={r} />);
    expect(getByText(/unsupported/i)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/components/MetricView.test.tsx`
Expected: FAIL — cannot resolve `./MetricView`.

- [ ] **Step 3: Write the implementation**

`web/src/components/MetricView.tsx`:
```tsx
import type { Result } from "../api";
import TimeSeriesChart from "./TimeSeriesChart";
import ScalarStat from "./ScalarStat";
import BucketsBar from "./BucketsBar";
import Leaderboard from "./Leaderboard";

interface Props {
  result: Result;
}

/** MetricView dispatches a Result to the one renderer matching its `kind`. */
export default function MetricView({ result }: Props) {
  switch (result.kind) {
    case "time_series":
      return <TimeSeriesChart series={result.series} label={result.label} />;
    case "scalar":
      return <ScalarStat result={result} />;
    case "buckets":
      return <BucketsBar result={result} />;
    case "leaderboard":
      return <Leaderboard result={result} />;
    default:
      return <p className="empty">Unsupported metric type.</p>;
  }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/components/MetricView.test.tsx`
Expected: PASS — all five branches.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/MetricView.tsx web/src/components/MetricView.test.tsx
git commit -m "feat(web): MetricView dispatch on result.kind"
```

---

## Task 10: Presentational components — `UserBar`, `SyncStatusBadge`, `AddRepoForm`, `RepoCard`

**Files:**
- Create: `web/src/components/UserBar.tsx`, `web/src/components/SyncStatusBadge.tsx`, `web/src/components/AddRepoForm.tsx`, `web/src/components/RepoCard.tsx`

The Overview building blocks. `UserBar`: brand + avatar + login + sign-out link. `SyncStatusBadge`: a colored pill from `sync_status`. `AddRepoForm`: a controlled `owner/name` input that calls an injected `onAdd` and surfaces errors. `RepoCard`: a `<Link>` card showing description, key overview numbers (open issues/PRs, contributors), sync badge, and last-synced relative time. **Verification: typecheck + build (Task 14)** — these are presentational; their behavior is exercised by the Overview page test in Task 11.

- [ ] **Step 1: Write `web/src/components/SyncStatusBadge.tsx`**

```tsx
import type { SyncStatus } from "../api";

interface Props {
  status: SyncStatus;
}

const LABELS: Record<string, string> = {
  complete: "Synced",
  running: "Syncing",
  pending: "Queued",
  error: "Error",
};

export default function SyncStatusBadge({ status }: Props) {
  const key = status || "pending";
  const cls = ["complete", "running", "pending", "error"].includes(key) ? key : "pending";
  return (
    <span className={`badge ${cls}`}>
      <span className="dot" />
      {LABELS[cls] ?? key}
    </span>
  );
}
```

- [ ] **Step 2: Write `web/src/components/UserBar.tsx`**

```tsx
import type { Me } from "../api";

interface Props {
  me: Me;
}

export default function UserBar({ me }: Props) {
  return (
    <header className="user-bar">
      <span className="brand">GitHub Stats</span>
      <span className="who">
        {me.avatar_url && <img src={me.avatar_url} alt="" />}
        <strong>{me.login}</strong>
        <a href="/auth/logout">Sign out</a>
      </span>
    </header>
  );
}
```

- [ ] **Step 3: Write `web/src/components/AddRepoForm.tsx`**

```tsx
import { useState, type FormEvent } from "react";

interface Props {
  onAdd: (fullName: string) => Promise<unknown>;
}

const FULL_NAME = /^[\w.-]+\/[\w.-]+$/;

export default function AddRepoForm({ onAdd }: Props) {
  const [value, setValue] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    const trimmed = value.trim();
    if (!FULL_NAME.test(trimmed)) {
      setError("Enter a repository as owner/name.");
      return;
    }
    setError(null);
    setBusy(true);
    try {
      await onAdd(trimmed);
      setValue("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to add repository.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit}>
      <div className="add-repo">
        <input
          aria-label="Repository (owner/name)"
          placeholder="owner/name"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          disabled={busy}
        />
        <button type="submit" className="primary" disabled={busy}>
          {busy ? "Adding…" : "Track repo"}
        </button>
      </div>
      {error && <p className="form-error">{error}</p>}
    </form>
  );
}
```

- [ ] **Step 4: Write `web/src/components/RepoCard.tsx`**

```tsx
import { Link } from "react-router-dom";
import type { Repo, Overview } from "../api";
import SyncStatusBadge from "./SyncStatusBadge";
import { fmtNullableTs, fmtNumber } from "../format";

interface Props {
  repo: Repo;
  overview: Overview | null;
}

export default function RepoCard({ repo, overview }: Props) {
  const to = `/${repo.full_name}`;
  return (
    <Link to={to} className="repo-card">
      <h3>{repo.full_name}</h3>
      <p className="desc">{overview?.description || (repo.is_private ? "Private repository" : "")}</p>
      <div className="stats">
        <div className="stat">
          <span className="n">{overview ? fmtNumber(overview.open_issues) : "—"}</span>
          <span className="l">issues</span>
        </div>
        <div className="stat">
          <span className="n">{overview ? fmtNumber(overview.open_prs) : "—"}</span>
          <span className="l">PRs</span>
        </div>
        <div className="stat">
          <span className="n">{overview ? fmtNumber(overview.contributors) : "—"}</span>
          <span className="l">authors</span>
        </div>
      </div>
      <div className="card-foot">
        <SyncStatusBadge status={repo.sync_status} />
        <span>synced {fmtNullableTs(repo.last_synced_at)}</span>
      </div>
    </Link>
  );
}
```

- [ ] **Step 5: Commit**

```bash
git add web/src/components/UserBar.tsx web/src/components/SyncStatusBadge.tsx web/src/components/AddRepoForm.tsx web/src/components/RepoCard.tsx
git commit -m "feat(web): overview building-block components"
```

---

## Task 11: Overview page (`/`)

**Files:**
- Create: `web/src/pages/Overview.tsx`, `web/src/pages/Overview.test.tsx`

The `/` route: renders `UserBar`, the `AddRepoForm` (wired to `useRepos().add`), and a responsive grid of `RepoCard`s (one per tracked repo). Each card hydrates its key numbers from `GET /api/repos/{id}` (a per-repo overview fetch with the default `30d`/`exclude_bots=false`). Loading and empty states are explicit. The test mounts the page inside a `MemoryRouter`, mocks the api module, and asserts cards render and the add form calls the API.

- [ ] **Step 1: Write the failing test**

`web/src/pages/Overview.test.tsx`:
```tsx
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import Overview from "./Overview";
import * as api from "../api";

const ME: api.Me = { id: 1, github_id: 9, login: "neo", avatar_url: "" };
const REPOS: api.Repo[] = [
  { id: 1, full_name: "octocat/hello", is_private: false, default_branch: "main", sync_status: "complete", last_synced_at: null },
];
function ov(id: number): api.Overview {
  return {
    id, full_name: "octocat/hello", is_private: false, default_branch: "main", description: "Hi",
    stargazers: 1, forks: 0, open_issues: 4, open_prs: 1, contributors: 3,
    commit_rate: 2, issue_rate: 0.5, pr_rate: 0.3, releases: 0,
    sync_status: "complete", last_synced_at: null, window_from: "2026-05-01", window_to: "2026-05-31",
  };
}

function renderPage() {
  return render(
    <MemoryRouter>
      <Overview me={ME} />
    </MemoryRouter>,
  );
}

describe("Overview page", () => {
  beforeEach(() => {
    vi.spyOn(api, "listRepos").mockResolvedValue(REPOS);
    vi.spyOn(api, "fetchOverview").mockResolvedValue(ov(1));
  });

  it("renders the user and a repo card with overview numbers", async () => {
    renderPage();
    expect(screen.getByText("neo")).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText("octocat/hello")).toBeInTheDocument());
    await waitFor(() => expect(screen.getByText("4")).toBeInTheDocument()); // open issues
  });

  it("adding a repo calls addRepo and shows the new card", async () => {
    const added: api.Repo = { id: 2, full_name: "a/b", is_private: false, default_branch: "main", sync_status: "pending", last_synced_at: null };
    vi.spyOn(api, "addRepo").mockResolvedValue(added);
    vi.spyOn(api, "fetchOverview").mockResolvedValue(ov(2));
    renderPage();

    await userEvent.type(screen.getByLabelText(/Repository/i), "a/b");
    await userEvent.click(screen.getByRole("button", { name: /track repo/i }));

    await waitFor(() => expect(api.addRepo).toHaveBeenCalledWith("a/b"));
  });

  it("shows an empty state when no repos are tracked", async () => {
    vi.spyOn(api, "listRepos").mockResolvedValue([]);
    renderPage();
    await waitFor(() => expect(screen.getByText(/no repositories tracked/i)).toBeInTheDocument());
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/pages/Overview.test.tsx`
Expected: FAIL — cannot resolve `./Overview`.

- [ ] **Step 3: Write the implementation**

`web/src/pages/Overview.tsx`:
```tsx
import { useEffect, useState } from "react";
import type { Me, Repo, Overview as OverviewBundle } from "../api";
import { fetchOverview } from "../api";
import { useRepos } from "../hooks/useRepos";
import UserBar from "../components/UserBar";
import AddRepoForm from "../components/AddRepoForm";
import RepoCard from "../components/RepoCard";

interface Props {
  me: Me;
}

// Hydrate each card's headline numbers from the per-repo overview bundle.
function useOverviews(repos: Repo[]): Record<number, OverviewBundle> {
  const [map, setMap] = useState<Record<number, OverviewBundle>>({});
  useEffect(() => {
    let active = true;
    for (const r of repos) {
      fetchOverview(r.id, { window: "30d", excludeBots: false })
        .then((ov) => {
          if (active) setMap((prev) => ({ ...prev, [r.id]: ov }));
        })
        .catch(() => {
          /* card falls back to placeholders */
        });
    }
    return () => {
      active = false;
    };
  }, [repos]);
  return map;
}

export default function Overview({ me }: Props) {
  const { repos, loading, error, add } = useRepos();
  const overviews = useOverviews(repos);

  return (
    <div className="app-shell">
      <UserBar me={me} />
      <AddRepoForm onAdd={add} />

      {loading && <p className="state">Loading repositories…</p>}
      {error && <p className="state error">Failed to load repositories: {error.message}</p>}
      {!loading && !error && repos.length === 0 && (
        <div className="notice">No repositories tracked yet. Add one above to get started.</div>
      )}

      <div className="repo-grid">
        {repos.map((r) => (
          <RepoCard key={r.id} repo={r} overview={overviews[r.id] ?? null} />
        ))}
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/pages/Overview.test.tsx`
Expected: PASS — user + card numbers, add flow, empty state.

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/Overview.tsx web/src/pages/Overview.test.tsx
git commit -m "feat(web): Overview page (user, add-repo, repo grid)"
```

---

## Task 12: Repo-detail support components — `WindowControls`, `LatestList`, `RefreshButton`

**Files:**
- Create: `web/src/components/WindowControls.tsx`, `web/src/components/LatestList.tsx`, `web/src/components/RefreshButton.tsx`, `web/src/components/RefreshButton.test.tsx`

`WindowControls`: the window `<select>` (30d/90d/6m/1y/all) + an exclude-bots checkbox, both controlled via injected setters. `LatestList`: renders a list of latest commits/prs/issues (discriminated by a `kind` prop). `RefreshButton`: a button that calls `refreshRepo(id)` then `openSyncStream(id, …)`, appending each `SyncEvent.message` to a live progress log, disabling itself while running, and notifying the parent via `onComplete` when the terminal `done` event arrives (so the parent can refetch). The SSE event handling is the tested unit (with `EventSource`/`refreshRepo` mocked).

- [ ] **Step 1: Write `web/src/components/WindowControls.tsx`**

```tsx
import type { WindowSpec } from "../api";

interface Props {
  window: WindowSpec;
  excludeBots: boolean;
  onWindow: (w: WindowSpec) => void;
  onExcludeBots: (v: boolean) => void;
}

const WINDOWS: { value: WindowSpec; label: string }[] = [
  { value: "30d", label: "30 days" },
  { value: "90d", label: "90 days" },
  { value: "6m", label: "6 months" },
  { value: "1y", label: "1 year" },
  { value: "all", label: "All time" },
];

export default function WindowControls({ window, excludeBots, onWindow, onExcludeBots }: Props) {
  return (
    <div className="controls">
      <label>
        Window
        <select value={window} onChange={(e) => onWindow(e.target.value as WindowSpec)}>
          {WINDOWS.map((w) => (
            <option key={w.value} value={w.value}>{w.label}</option>
          ))}
        </select>
      </label>
      <label>
        <input
          type="checkbox"
          checked={excludeBots}
          onChange={(e) => onExcludeBots(e.target.checked)}
        />
        Exclude bots
      </label>
    </div>
  );
}
```

- [ ] **Step 2: Write `web/src/components/LatestList.tsx`**

```tsx
import type { LatestItem, LatestCommit, LatestPR, LatestIssue, LatestKind } from "../api";
import { fmtRelative } from "../format";

interface Props {
  kind: LatestKind;
  items: LatestItem[];
}

function isCommit(i: LatestItem): i is LatestCommit {
  return (i as LatestCommit).sha !== undefined;
}
function isPR(i: LatestItem, kind: LatestKind): i is LatestPR {
  return kind === "prs";
}

export default function LatestList({ kind, items }: Props) {
  if (items.length === 0) return <p className="empty">Nothing yet.</p>;
  return (
    <div>
      {items.map((item) => {
        if (isCommit(item)) {
          return (
            <div className="latest-item" key={item.sha}>
              <span className="meta">{fmtRelative(item.committed_at)}</span>
              <span className="title">{item.msg_first_line}</span>
              <span className="meta">{item.author_login}</span>
            </div>
          );
        }
        if (isPR(item, kind)) {
          const pr = item as LatestPR;
          return (
            <div className="latest-item" key={`pr-${pr.number}`}>
              <span className="meta">#{pr.number}</span>
              <span className="title">{pr.title}</span>
              <span className="meta">{pr.state.toLowerCase()} · {fmtRelative(pr.created_at)}</span>
            </div>
          );
        }
        const iss = item as LatestIssue;
        return (
          <div className="latest-item" key={`iss-${iss.number}`}>
            <span className="meta">#{iss.number}</span>
            <span className="title">{iss.title}</span>
            <span className="meta">{iss.state.toLowerCase()} · {fmtRelative(iss.created_at)}</span>
          </div>
        );
      })}
    </div>
  );
}
```

- [ ] **Step 3: Write the failing `RefreshButton` test**

`web/src/components/RefreshButton.test.tsx`:
```tsx
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import RefreshButton from "./RefreshButton";
import * as api from "../api";

type Fake = {
  onmessage: ((e: MessageEvent) => void) | null;
  onerror: ((e: Event) => void) | null;
  close: ReturnType<typeof vi.fn>;
};

function installEventSource(): Fake {
  const fake: Fake = { onmessage: null, onerror: null, close: vi.fn() };
  vi.stubGlobal("EventSource", vi.fn(() => fake) as unknown as typeof EventSource);
  return fake;
}

function emit(fake: Fake, ev: api.SyncEvent) {
  act(() => {
    fake.onmessage?.({ data: JSON.stringify(ev) } as MessageEvent);
  });
}

describe("RefreshButton", () => {
  beforeEach(() => {
    vi.spyOn(api, "refreshRepo").mockResolvedValue();
  });

  it("triggers a refresh, streams progress, and calls onComplete on done", async () => {
    const fake = installEventSource();
    const onComplete = vi.fn();
    render(<RefreshButton repoID={7} onComplete={onComplete} />);

    await userEvent.click(screen.getByRole("button", { name: /refresh now/i }));
    await waitFor(() => expect(api.refreshRepo).toHaveBeenCalledWith(7));

    emit(fake, { repo_id: 7, phase: "commits", message: "commits: page 1", done: false });
    expect(screen.getByText(/commits: page 1/)).toBeInTheDocument();

    emit(fake, { repo_id: 7, phase: "done", message: "complete", done: true });
    expect(onComplete).toHaveBeenCalledTimes(1);
    expect(fake.close).toHaveBeenCalled();
  });

  it("disables the button while a refresh is in flight", async () => {
    installEventSource();
    render(<RefreshButton repoID={7} onComplete={() => {}} />);
    const btn = screen.getByRole("button", { name: /refresh now/i });
    await userEvent.click(btn);
    await waitFor(() => expect(btn).toBeDisabled());
  });
});
```

- [ ] **Step 4: Run test to verify it fails**

Run: `cd web && npx vitest run src/components/RefreshButton.test.tsx`
Expected: FAIL — cannot resolve `./RefreshButton`.

- [ ] **Step 5: Write `web/src/components/RefreshButton.tsx`**

```tsx
import { useEffect, useRef, useState } from "react";
import { refreshRepo, openSyncStream, type SyncEvent, type SyncStreamHandle } from "../api";

interface Props {
  repoID: number;
  onComplete: () => void;
}

interface LogLine {
  id: number;
  text: string;
  tone: "" | "error" | "done";
}

export default function RefreshButton({ repoID, onComplete }: Props) {
  const [running, setRunning] = useState(false);
  const [lines, setLines] = useState<LogLine[]>([]);
  const handleRef = useRef<SyncStreamHandle | null>(null);
  const seq = useRef(0);

  // Close any open stream on unmount.
  useEffect(() => () => handleRef.current?.close(), []);

  function push(text: string, tone: LogLine["tone"]) {
    seq.current += 1;
    const id = seq.current;
    setLines((prev) => [...prev.slice(-49), { id, text, tone }]);
  }

  async function start() {
    if (running) return;
    setRunning(true);
    setLines([]);
    try {
      await refreshRepo(repoID);
    } catch (e) {
      push(e instanceof Error ? e.message : "refresh failed", "error");
      setRunning(false);
      return;
    }
    handleRef.current = openSyncStream(repoID, {
      onEvent: (ev: SyncEvent) => {
        push(ev.message || ev.phase, ev.phase === "error" ? "error" : "");
      },
      onDone: (ev: SyncEvent) => {
        push(ev.message || "done", ev.phase === "error" ? "error" : "done");
        setRunning(false);
        handleRef.current = null;
        onComplete();
      },
      onError: () => {
        push("stream interrupted", "error");
        setRunning(false);
        handleRef.current = null;
      },
    });
  }

  return (
    <div className="refresh-box">
      <button className="primary" onClick={start} disabled={running}>
        {running ? "Refreshing…" : "Refresh now"}
      </button>
      {lines.length > 0 && (
        <div className="progress-log">
          {lines.map((l) => (
            <div key={l.id} className={`line ${l.tone}`}>{l.text}</div>
          ))}
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `cd web && npx vitest run src/components/RefreshButton.test.tsx`
Expected: PASS — refresh + SSE progress + onComplete, and disabled-while-running.

- [ ] **Step 7: Commit**

```bash
git add web/src/components/WindowControls.tsx web/src/components/LatestList.tsx web/src/components/RefreshButton.tsx web/src/components/RefreshButton.test.tsx
git commit -m "feat(web): window controls, latest lists, SSE refresh button"
```

---

## Task 13: Repo-detail page (`/:owner/:repo`)

**Files:**
- Create: `web/src/pages/RepoDetail.tsx`, `web/src/pages/RepoDetail.test.tsx`

The detail route. It reads `:owner`/`:repo` from the URL, resolves them to a tracked repo id via `useRepos().resolve` (showing an **"add this repo"** affordance when untracked), and renders: a header (full name + privacy + `SyncStatusBadge` + last-synced); `WindowControls` + `RefreshButton`; and the seven **sections** — **Details** (overview numbers), **Insights** (`commit_rate`, `pr_throughput`, `code_churn`, `comment_volume`, the four scalars, `open_issue_age`), **Commits** (`commit_rate` chart + latest commits), **Issues** (`issue_lifetime`, `open_issue_age` + latest issues), **PRs** (`pr_throughput`, `time_to_merge`, `review_latency` + latest PRs), **Contributors** (`contributor_leaderboard`), **Releases** (release count from overview). Changing the window or exclude-bots refetches metrics/overview; "Refresh now" → `onComplete` refetches everything. The test mounts under a router with mocked api, asserts metric cards render, that an untracked repo shows the add affordance, and that switching the window triggers a refetch.

- [ ] **Step 1: Write the failing test**

`web/src/pages/RepoDetail.test.tsx`:
```tsx
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import RepoDetail from "./RepoDetail";
import * as api from "../api";

// Stub the chart so no canvas is needed.
vi.mock("../components/TimeSeriesChart", () => ({
  default: ({ label }: { label?: string }) => <div data-testid="ts-chart">{label}</div>,
}));

const REPOS: api.Repo[] = [
  { id: 1, full_name: "octocat/hello", is_private: false, default_branch: "main", sync_status: "complete", last_synced_at: "2026-05-31T10:00:00Z" },
];
const OVERVIEW: api.Overview = {
  id: 1, full_name: "octocat/hello", is_private: false, default_branch: "main", description: "Hi",
  stargazers: 5, forks: 1, open_issues: 4, open_prs: 2, contributors: 3,
  commit_rate: 2.4, issue_rate: 0.5, pr_rate: 0.3, releases: 7,
  sync_status: "complete", last_synced_at: "2026-05-31T10:00:00Z", window_from: "2026-05-01", window_to: "2026-05-31",
};
const METRICS: api.MetricsMap = {
  commit_rate: { kind: "time_series", label: "Commits/day", series: [{ date: "2026-05-01", value: 3 }] },
  time_to_merge: { kind: "scalar", label: "median", value: 12.5, unit: "hours", count: 4 },
  open_issue_age: { kind: "buckets", label: "Open issue age", buckets: [{ label: "<24h", count: 1 }] },
  contributor_leaderboard: { kind: "leaderboard", label: "Top", rows: [{ login: "neo", commits: 9, additions: 1, deletions: 0 }] },
};

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/:owner/:repo" element={<RepoDetail />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("RepoDetail page", () => {
  beforeEach(() => {
    vi.spyOn(api, "listRepos").mockResolvedValue(REPOS);
    vi.spyOn(api, "fetchOverview").mockResolvedValue(OVERVIEW);
    vi.spyOn(api, "fetchMetrics").mockResolvedValue(METRICS);
    vi.spyOn(api, "fetchLatest").mockResolvedValue([]);
  });

  it("resolves owner/repo and renders sections + metrics", async () => {
    renderAt("/octocat/hello");
    await waitFor(() => expect(screen.getByRole("heading", { name: "octocat/hello" })).toBeInTheDocument());
    await waitFor(() => expect(screen.getByText("Contributors")).toBeInTheDocument());
    await waitFor(() => expect(screen.getByText("neo")).toBeInTheDocument()); // leaderboard
    expect(screen.getByText("12.5h")).toBeInTheDocument(); // scalar
  });

  it("shows an add-this-repo affordance when untracked", async () => {
    renderAt("/nobody/nothing");
    await waitFor(() => expect(screen.getByText(/not tracked/i)).toBeInTheDocument());
    expect(screen.getByRole("button", { name: /track this repo/i })).toBeInTheDocument();
  });

  it("changing the window refetches metrics", async () => {
    renderAt("/octocat/hello");
    await waitFor(() => expect(api.fetchMetrics).toHaveBeenCalled());
    const callsBefore = (api.fetchMetrics as ReturnType<typeof vi.fn>).mock.calls.length;
    await userEvent.selectOptions(screen.getByRole("combobox"), "90d");
    await waitFor(() =>
      expect((api.fetchMetrics as ReturnType<typeof vi.fn>).mock.calls.length).toBeGreaterThan(callsBefore),
    );
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npx vitest run src/pages/RepoDetail.test.tsx`
Expected: FAIL — cannot resolve `./RepoDetail`.

- [ ] **Step 3: Write the implementation**

`web/src/pages/RepoDetail.tsx`:
```tsx
import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import {
  fetchOverview,
  fetchMetrics,
  fetchLatest,
  type Result,
  type WindowSpec,
  type LatestItem,
} from "../api";
import { useRepos } from "../hooks/useRepos";
import { useAsync } from "../hooks/useAsync";
import MetricView from "../components/MetricView";
import WindowControls from "../components/WindowControls";
import RefreshButton from "../components/RefreshButton";
import LatestList from "../components/LatestList";
import SyncStatusBadge from "../components/SyncStatusBadge";
import { fmtNullableTs, fmtNumber, fmtRate } from "../format";

const INSIGHT_KEYS = [
  "commit_rate",
  "pr_throughput",
  "code_churn",
  "comment_volume",
  "time_to_merge",
  "review_latency",
  "issue_lifetime",
  "open_issue_age",
  "contributor_leaderboard",
];

function MetricCard({ title, result }: { title: string; result: Result | undefined }) {
  return (
    <div className="metric-card">
      <h3>{title}</h3>
      {result ? <MetricView result={result} /> : <p className="empty">No data.</p>}
    </div>
  );
}

export default function RepoDetail() {
  const { owner = "", repo = "" } = useParams();
  const { loading: reposLoading, resolve, add } = useRepos();
  const [windowSpec, setWindowSpec] = useState<WindowSpec>("30d");
  const [excludeBots, setExcludeBots] = useState(false);

  const tracked = resolve(owner, repo);
  const repoID = tracked?.id ?? 0;

  const overview = useAsync(
    () => fetchOverview(repoID, { window: windowSpec, excludeBots }),
    [repoID, windowSpec, excludeBots],
  );
  const metrics = useAsync(
    () => fetchMetrics(repoID, { keys: INSIGHT_KEYS, window: windowSpec, excludeBots }),
    [repoID, windowSpec, excludeBots],
  );
  const commits = useAsync<LatestItem[]>(() => fetchLatest(repoID, "commits", 20), [repoID]);
  const prs = useAsync<LatestItem[]>(() => fetchLatest(repoID, "prs", 20), [repoID]);
  const issues = useAsync<LatestItem[]>(() => fetchLatest(repoID, "issues", 20), [repoID]);

  function reloadAll() {
    overview.reload();
    metrics.reload();
    commits.reload();
    prs.reload();
    issues.reload();
  }

  if (reposLoading) {
    return <div className="app-shell"><p className="state">Loading…</p></div>;
  }

  if (!tracked) {
    return (
      <div className="app-shell">
        <p><Link to="/">← All repositories</Link></p>
        <div className="notice">
          <p><strong>{owner}/{repo}</strong> is not tracked yet.</p>
          <button
            className="primary"
            onClick={() => add(`${owner}/${repo}`)}
          >
            Track this repo
          </button>
        </div>
      </div>
    );
  }

  const ov = overview.data;
  const m = metrics.data ?? {};

  return (
    <div className="app-shell">
      <p><Link to="/">← All repositories</Link></p>

      <div className="detail-head">
        <div>
          <h1>{tracked.full_name}</h1>
          <div className="sub">
            {tracked.is_private ? "Private" : "Public"} · {tracked.default_branch} ·
            {" "}synced {fmtNullableTs(tracked.last_synced_at)}
          </div>
        </div>
        <div className="refresh-controls">
          <SyncStatusBadge status={tracked.sync_status} />
          <RefreshButton repoID={repoID} onComplete={reloadAll} />
        </div>
      </div>

      <WindowControls
        window={windowSpec}
        excludeBots={excludeBots}
        onWindow={setWindowSpec}
        onExcludeBots={setExcludeBots}
      />

      {metrics.error && (
        <p className="state error">Failed to load metrics: {metrics.error.message}</p>
      )}

      {/* Details */}
      <section className="section">
        <h2>Details</h2>
        <div className="metric-grid">
          <div className="metric-card"><h3>Open issues</h3><div className="scalar"><span className="value">{ov ? fmtNumber(ov.open_issues) : "—"}</span></div></div>
          <div className="metric-card"><h3>Open PRs</h3><div className="scalar"><span className="value">{ov ? fmtNumber(ov.open_prs) : "—"}</span></div></div>
          <div className="metric-card"><h3>Contributors</h3><div className="scalar"><span className="value">{ov ? fmtNumber(ov.contributors) : "—"}</span></div></div>
          <div className="metric-card"><h3>Commit rate</h3><div className="scalar"><span className="value">{ov ? fmtRate(ov.commit_rate) : "—"}</span></div></div>
        </div>
      </section>

      {/* Insights */}
      <section className="section">
        <h2>Insights</h2>
        <div className="metric-grid">
          <MetricCard title="Commit rate" result={m.commit_rate} />
          <MetricCard title="PR throughput" result={m.pr_throughput} />
          <MetricCard title="Code churn" result={m.code_churn} />
          <MetricCard title="Comment volume" result={m.comment_volume} />
        </div>
      </section>

      {/* Commits */}
      <section className="section">
        <h2>Commits</h2>
        <div className="metric-grid">
          <MetricCard title="Commits per day" result={m.commit_rate} />
          <div className="metric-card">
            <h3>Latest commits</h3>
            <LatestList kind="commits" items={commits.data ?? []} />
          </div>
        </div>
      </section>

      {/* Issues */}
      <section className="section">
        <h2>Issues</h2>
        <div className="metric-grid">
          <MetricCard title="Issue lifetime" result={m.issue_lifetime} />
          <MetricCard title="Open issue age" result={m.open_issue_age} />
          <div className="metric-card">
            <h3>Latest issues</h3>
            <LatestList kind="issues" items={issues.data ?? []} />
          </div>
        </div>
      </section>

      {/* PRs */}
      <section className="section">
        <h2>Pull requests</h2>
        <div className="metric-grid">
          <MetricCard title="PR throughput" result={m.pr_throughput} />
          <MetricCard title="Time to merge" result={m.time_to_merge} />
          <MetricCard title="Review latency" result={m.review_latency} />
          <div className="metric-card">
            <h3>Latest PRs</h3>
            <LatestList kind="prs" items={prs.data ?? []} />
          </div>
        </div>
      </section>

      {/* Contributors */}
      <section className="section">
        <h2>Contributors</h2>
        <div className="metric-grid">
          <MetricCard title="Leaderboard" result={m.contributor_leaderboard} />
        </div>
      </section>

      {/* Releases */}
      <section className="section">
        <h2>Releases</h2>
        <div className="metric-grid">
          <div className="metric-card">
            <h3>Total releases</h3>
            <div className="scalar"><span className="value">{ov ? fmtNumber(ov.releases) : "—"}</span></div>
          </div>
        </div>
      </section>
    </div>
  );
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npx vitest run src/pages/RepoDetail.test.tsx`
Expected: PASS — sections + metrics render, untracked affordance, window-change refetch.

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/RepoDetail.tsx web/src/pages/RepoDetail.test.tsx
git commit -m "feat(web): RepoDetail page with sections, controls, refresh"
```

---

## Task 14: Routing, `App`, `main.tsx` wiring + full build/embed

**Files:**
- Modify: `web/src/App.tsx`, `web/src/main.tsx`

`App` becomes the route table: it loads `me` once (M1's `fetchMe`), shows a sign-in screen when signed out, and otherwise renders `<Routes>` with `/` → `Overview` and `/:owner/:repo` → `RepoDetail`, plus a catch-all NotFound. `main.tsx` wraps `<App/>` in `<BrowserRouter>` and imports `styles.css`. The deep-link `/:owner/:repo` is already served by M1's SPA fallback. This task ends with the **build + embed** verification (typecheck, bundle, Go build, then restore the placeholder).

- [ ] **Step 1: Replace `web/src/App.tsx`**

`web/src/App.tsx`:
```tsx
import { useEffect, useState } from "react";
import { Routes, Route, Link } from "react-router-dom";
import { fetchMe, type Me } from "./api";
import Overview from "./pages/Overview";
import RepoDetail from "./pages/RepoDetail";

function SignIn() {
  return (
    <div className="app-shell">
      <header className="user-bar"><span className="brand">GitHub Stats</span></header>
      <div className="notice">
        <p>Track public and private repository analytics without GitHub premium.</p>
        <p><a className="primary-link" href="/auth/github">Sign in with GitHub</a></p>
      </div>
    </div>
  );
}

function NotFound() {
  return (
    <div className="app-shell">
      <div className="notice">
        <p>Page not found.</p>
        <p><Link to="/">← Back to overview</Link></p>
      </div>
    </div>
  );
}

export default function App() {
  const [me, setMe] = useState<Me | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetchMe()
      .then(setMe)
      .catch(() => setMe(null))
      .finally(() => setLoading(false));
  }, []);

  if (loading) return <div className="app-shell"><p className="state">Loading…</p></div>;
  if (!me) return <SignIn />;

  return (
    <Routes>
      <Route path="/" element={<Overview me={me} />} />
      <Route path="/:owner/:repo" element={<RepoDetail />} />
      <Route path="*" element={<NotFound />} />
    </Routes>
  );
}
```

- [ ] **Step 2: Replace `web/src/main.tsx`**

`web/src/main.tsx`:
```tsx
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import App from "./App";
import "./styles.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </StrictMode>,
);
```

- [ ] **Step 3: Run the whole Vitest suite**

Run: `cd web && npx vitest run`
Expected: PASS — `format`, `api`, `useRepos`, `TimeSeriesChart`, `MetricView`, `RefreshButton`, `Overview`, `RepoDetail`.

- [ ] **Step 4: Typecheck + production build**

Run: `cd web && npm run build`
Expected: `tsc -b` reports no type errors and Vite emits `web/dist/index.html` + hashed `web/dist/assets/*` with no errors. (This is the "typecheck + build passes" verification covering the presentational/scaffolding tasks 4, 5, 8, 10, 12.)

- [ ] **Step 5: Verify the Go binary still embeds the freshly built dist**

Run: `go build ./...`
Expected: succeeds — `web/embed.go` embeds the regenerated `web/dist`.

- [ ] **Step 6: Restore the committed placeholder so the tree stays clean (exactly as M1)**

Run: `git checkout -- web/dist/index.html`
Expected: the real (hashed-asset-referencing) `web/dist/index.html` is reverted to the committed placeholder; the gitignored `web/dist/assets/` and any new `web/dist/*` remain untracked. Confirm with `git status --short` that **no `web/dist/` change is staged**.

- [ ] **Step 7: Commit (source only — never dist artifacts or node_modules)**

```bash
git add web/src/App.tsx web/src/main.tsx
git commit -m "feat(web): route table, BrowserRouter, global styles"
```

---

## Task 15: Final verification

**Files:** none (verification only)

- [ ] **Step 1: Full frontend test suite is green**

Run: `cd web && npm run test`
Expected: all suites PASS (`vitest run`).

- [ ] **Step 2: Full build + embed + Go test, then restore placeholder**

Run: `cd web && npm run build && cd .. && go build ./... && go test ./... && git checkout -- web/dist/index.html`
Expected: frontend typecheck + bundle succeed; Go builds with the embedded SPA; the full Go suite (M1–M4) still PASSES; the placeholder is restored.

- [ ] **Step 3: Confirm the working tree is clean of build artifacts**

Run: `git status --short`
Expected: no `web/dist/` entries staged or modified; `web/node_modules/` is ignored. Only intended source files were committed across Tasks 1–14.

- [ ] **Step 4: Manual smoke test (requires a running backend with data)**

1. Start the API (`make dev-api`) and the frontend (`make dev-web`); open `http://localhost:5173`.
2. Sign in → land on `/` showing your login + the add-repo form.
3. Add `owner/name` → a card appears with a sync badge; the URL `/owner/name` opens the detail page (also reachable directly — SPA fallback).
4. On detail: switch the window (30d→90d→all) and toggle exclude-bots → charts/scalars refetch and update.
5. Click "Refresh now" → the progress log streams SSE events and the sync badge updates; on completion the numbers refresh.

Expected: full overview → detail → refresh round-trip works against the single binary / dev proxy.

---

## Self-Review notes

- **Spec §8 coverage — each item → a task:**
  - **Overview** (user + add-repo + repo cards w/ key metrics + sync status) → Tasks 10–11 (`UserBar`, `AddRepoForm`, `RepoCard`, `SyncStatusBadge`, `Overview`).
  - **Repo detail** (sections Details · Insights · Commits · Issues · PRs · Contributors · Releases) → Task 13 (`RepoDetail`), rendering metrics via Tasks 7–9 + latest lists via Task 12.
  - **Window selector (30d/90d/6m/1y/all) + exclude-bots toggle** that refetch → Task 12 (`WindowControls`) + Task 13 (state + `useAsync` deps refetch; covered by the window-change test).
  - **URL shortcut `/owner/repo`** → Task 14 routing + Task 6 (`resolve`); untracked → "add this repo" affordance (Task 13).
  - **Refresh now + live SSE progress + sync status update** → Task 12 (`RefreshButton` + `openSyncStream`) with the SSE-handling test.
  - **uPlot time-series + one renderer per Result kind** → Tasks 7–9 (`TimeSeriesChart`/`ScalarStat`/`BucketsBar`/`Leaderboard`/`MetricView`).
  - **Embedded single-binary build** → Task 14/15 (`npm run build` → `go build` embeds `web/dist`; placeholder restored).
- **Types match M3/M4 JSON exactly:** `Repo`/`Overview`/`LatestCommit|PR|Issue`/`SyncEvent` field names and the `Result` union (`time_series`/`scalar`/`buckets`/`leaderboard` with `series`/`value`+`unit`+`count`/`buckets`/`rows` and `LeaderRow{login,commits,additions,deletions}`) mirror the M4 "Public API surface" and the M3 `repoJSON` + `Event{repo_id,phase,message,done}` shapes. The metric key→kind map (Task 13's `INSIGHT_KEYS`) follows M4's table (`commit_rate`/`pr_throughput`/`code_churn`/`comment_volume`=time_series; `time_to_merge`/`review_latency`/`issue_lifetime`=scalar hours; `open_issue_age`=buckets; `contributor_leaderboard`=leaderboard).
- **No placeholders:** every code step is complete, compilable TS/TSX (and full CSS in Task 4). The four `Result` renderers, `MetricView` switch, both pages, the SSE helper, and the hooks are all whole.
- **Deterministic tests:** `fetch`, `EventSource`, and `uplot` are mocked; `format` uses an injected `now`; no real timers or network. The uPlot wrapper gets a mount/unmount lifecycle test (constructor called once, `destroy` on unmount). `MetricView` switching + scalar/buckets/leaderboard output, `useRepos.resolve`, window-change refetch, and SSE event handling each have a focused test written **before** the implementation (test → fail → implement → pass).
- **Honest verification seams:** pure-presentation tasks (styles, building-block components) are explicitly verified by "typecheck + build passes" (Task 14 Step 4), not faked DOM snapshots; behavior that matters is asserted through the page-level RTL tests instead.
- **Tree stays clean:** only source is committed; `web/dist/*` (except the placeholder) and `web/node_modules/` are gitignored (M1), and the placeholder is restored after every build (`git checkout -- web/dist/index.html`).
- **No backend changes:** every consumed endpoint already ships in M1/M3/M4; M5 adds zero Go files. (Had any tiny Go change been required it would be its own explicit task — none is.)

---

## What M6 will add (next plan)

- **Collections UI** (spec §10 `GET/POST /api/collections`, `DELETE /api/collections/{id}`): group repos into named collections on the Overview, filter the grid by collection.
- **Save/load** of dashboard configuration (selected window, exclude-bots default, section ordering) per user.
- **Dark/light theme** toggle (the M5 stylesheet ships dark-only tokens; M6 adds a light token set + switch).
- **PAT settings** page (the alternate credential, spec §9) and **rate-limit UX polish** (budget remaining, reset countdown, backoff surfacing).
- **Bot-detection management** UI and richer chart types (optional ECharts) per spec §8's "ECharts optional".

---

## Public API surface M5 exposes

For the M6 plan to extend the frontend precisely. All under `web/`.

**Routes (`react-router-dom`, mounted in `src/App.tsx` under `<BrowserRouter>`):**
```
/                 -> <Overview me={me} />     (signed-in; sign-in screen when me === null)
/:owner/:repo     -> <RepoDetail />           (resolves owner/repo -> tracked repo id)
*                 -> <NotFound />
```

**`src/api.ts` — typed client (every function `credentials:"same-origin"`):**
```ts
// Auth
fetchMe(): Promise<Me | null>                                  // 401 -> null
// Repos (M3)
listRepos(): Promise<Repo[]>
addRepo(fullName: string): Promise<Repo>                       // POST {full_name}
deleteRepo(id: number): Promise<void>                          // DELETE -> 204
refreshRepo(id: number): Promise<void>                         // POST refresh -> 202
// Metrics / overview / latest (M4)
fetchMetrics(repoID, { keys, window, excludeBots }): Promise<MetricsMap>   // Record<key, Result>
fetchOverview(repoID, { window, excludeBots }): Promise<Overview>
fetchLatest(repoID, kind: "commits"|"prs"|"issues", limit?): Promise<LatestItem[]>
// URL builders (pure)
metricsURL(repoID, MetricsOpts): string
overviewURL(repoID, QueryOpts): string
latestURL(repoID, kind, limit): string
// SSE (M3)
openSyncStream(repoID, { onEvent, onDone?, onError? }): SyncStreamHandle   // {close()}

// Types: Me, Repo, SyncStatus, WindowSpec ("30d"|"90d"|"6m"|"1y"|"all"),
//   QueryOpts{window,excludeBots}, MetricsOpts extends QueryOpts {keys:string[]},
//   Result = TimeSeriesResult | ScalarResult | BucketsResult | LeaderboardResult,
//   SeriesPoint, BucketRow, LeaderRow, MetricsMap = Record<string, Result>,
//   Overview, LatestCommit, LatestPR, LatestIssue, LatestKind, LatestItem,
//   SyncEvent{repo_id,phase,message,done}, SyncStreamHandlers, SyncStreamHandle
```

**Hooks (`src/hooks/`):**
```ts
useAsync<T>(fn: () => Promise<T>, deps: unknown[]): { data, error, loading, reload }
useRepos(): { repos, loading, error, reload, resolve(owner,repo): Repo|null, add(fullName), remove(id) }
```

**Components (`src/components/`) — contracts:**
```ts
<MetricView result={Result} />                          // dispatch on result.kind
<TimeSeriesChart series={SeriesPoint[]} label? height? />// uPlot wrapper; empty -> placeholder
<ScalarStat result={ScalarResult} />                    // hours via fmtHours when unit==="hours"
<BucketsBar result={BucketsResult} />                   // CSS bar chart
<Leaderboard result={LeaderboardResult} />              // ranked table
<RepoCard repo={Repo} overview={Overview|null} />       // <Link> to /:full_name
<AddRepoForm onAdd={(fullName)=>Promise<unknown>} />
<UserBar me={Me} />
<SyncStatusBadge status={SyncStatus} />
<WindowControls window excludeBots onWindow onExcludeBots />
<LatestList kind={LatestKind} items={LatestItem[]} />
<RefreshButton repoID={number} onComplete={()=>void} /> // refresh + SSE live log
```

**Formatters (`src/format.ts`):**
```ts
fmtDate(iso) · fmtDateShort(iso) · fmtRelative(iso, now?) · fmtHours(h) ·
fmtRate(perDay) · fmtNumber(n) · fmtNullableTs(iso|null, now?)
```

**Pages (`src/pages/`):** `Overview({ me })`, `RepoDetail()` (reads `:owner`/`:repo` via `useParams`).

**Build/embed contract (unchanged from M1):** `npm run build` → `web/dist/` → `go:embed` in `web/embed.go` → single binary serving `/api/*` (JSON) + SPA (incl. deep links via the SPA fallback). After building, restore the committed placeholder: `git checkout -- web/dist/index.html`. Real `web/dist/assets/*` are gitignored; only source is committed.
