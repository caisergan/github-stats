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
  exportCollectionURL,
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

describe("collection export url (M6)", () => {
  it("builds the export path", () => {
    expect(exportCollectionURL(42)).toBe("/api/collections/42/export");
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
    // Mutating calls must carry the CSRF double-submit header (read from the
    // gs_csrf cookie seeded in the test setup, so no extra GET /api/csrf fires).
    expect(init.headers["X-CSRF-Token"]).toBe("test-csrf-token");
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
    expect(f.mock.calls[0][1].headers["X-CSRF-Token"]).toBe("test-csrf-token");
  });

  it("refreshRepo issues POST and resolves on 202", async () => {
    const f = fetch as ReturnType<typeof vi.fn>;
    f.mockResolvedValue(new Response(null, { status: 202 }));
    await refreshRepo(6);
    expect(f.mock.calls[0][1].method).toBe("POST");
    expect(f.mock.calls[0][1].headers["X-CSRF-Token"]).toBe("test-csrf-token");
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

describe("CSRF double-submit on mutating calls", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  it("reuses the gs_csrf cookie without an extra GET /api/csrf", async () => {
    document.cookie = "gs_csrf=cookie-tok";
    const f = fetch as ReturnType<typeof vi.fn>;
    f.mockResolvedValue(new Response(null, { status: 204 }));
    await deleteRepo(9);
    // Only the DELETE fires — the cookie short-circuits the token fetch.
    expect(f.mock.calls).toHaveLength(1);
    expect(f.mock.calls[0][0]).toBe("/api/repos/9");
    expect(f.mock.calls[0][1].headers["X-CSRF-Token"]).toBe("cookie-tok");
  });

  it("falls back to GET /api/csrf when the cookie is absent", async () => {
    // Expire the seeded cookie so the helper must fetch a fresh token.
    document.cookie = "gs_csrf=; expires=Thu, 01 Jan 1970 00:00:00 GMT";
    expect(document.cookie).not.toContain("gs_csrf");
    const f = fetch as ReturnType<typeof vi.fn>;
    f.mockImplementation((url: string) => {
      if (url === "/api/csrf") {
        return Promise.resolve(jsonResponse({ csrf_token: "fetched-tok" }));
      }
      return Promise.resolve(new Response(null, { status: 204 }));
    });
    await deleteRepo(11);
    // First call is the token fetch, then the mutating DELETE with the header.
    expect(f.mock.calls[0][0]).toBe("/api/csrf");
    const del = f.mock.calls.find((c) => c[0] === "/api/repos/11")!;
    expect(del[1].method).toBe("DELETE");
    expect(del[1].headers["X-CSRF-Token"]).toBe("fetched-tok");
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
