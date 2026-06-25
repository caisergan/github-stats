import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import RepoDetail from "./RepoDetail";
import * as api from "../api";

const REPO: api.Repo = {
  id: 1,
  full_name: "octocat/hello",
  is_private: false,
  default_branch: "main",
  description: "Hi",
  stargazers: 5,
  forks: 1,
  sync_status: "complete",
  last_synced_at: "2026-05-31T10:00:00Z",
};

beforeEach(() => {
  vi.spyOn(api, "fetchOverview").mockResolvedValue({
    id: 1,
    full_name: "octocat/hello",
    is_private: false,
    default_branch: "main",
    description: "Hi",
    stargazers: 5,
    forks: 1,
    open_issues: 4,
    open_prs: 2,
    contributors: 3,
    commit_rate: 2.4,
    issue_rate: 0.5,
    pr_rate: 0.3,
    releases: 7,
    sync_status: "complete",
    last_synced_at: "2026-05-31T10:00:00Z",
    window_from: "2026-03-01",
    window_to: "2026-05-31",
  });
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
  vi.spyOn(api, "fetchCommits").mockResolvedValue({
    items: [],
    stored: 0,
    total: 0,
    limit: 30,
    offset: 0,
  });
  vi.spyOn(api, "fetchSyncStatus").mockResolvedValue({
    kind: "",
    status: "",
    next_run_at: null,
    attempts: 0,
    last_error: "",
    active: false,
  });
});

function renderPage(repo = REPO, onBack = vi.fn()) {
  return render(
    <MemoryRouter>
      <RepoDetail repo={repo} onBack={onBack} onUntrack={vi.fn().mockResolvedValue(undefined)} />
    </MemoryRouter>,
  );
}

describe("RepoDetail page", () => {
  it("renders detail sections", async () => {
    renderPage();
    expect(screen.getByRole("heading", { name: /octocat.*hello/i })).toBeInTheDocument();
    await waitFor(() => expect(screen.getAllByText("Contributors")[0]).toBeInTheDocument());
  });

  it("contribution activity count + heatmap follow the selected Timing window", async () => {
    renderPage();
    // Default window is 90d — the label reflects it (it used to be a static "last year").
    await waitFor(() =>
      expect(screen.getByText(/contributions in the last 90 days/i)).toBeInTheDocument(),
    );

    // Switch the Window control to 1y.
    await userEvent.click(screen.getByRole("tab", { name: "1y" }));

    await waitFor(() =>
      expect(screen.getByText(/contributions in the last year/i)).toBeInTheDocument(),
    );
    // The heatmap series (commit_rate) was refetched for the new window — not pinned to "all".
    expect(api.fetchMetrics).toHaveBeenCalledWith(
      REPO.id,
      expect.objectContaining({ window: "1y", keys: ["commit_rate"] }),
    );
  });

  it("can switch tabs", async () => {
    renderPage();
    const btn = await screen.findByRole("button", { name: /commits/i });
    await userEvent.click(btn);
    await waitFor(() => expect(screen.getByText("Latest commits")).toBeInTheDocument());
  });

  it("shows stored-of-total header and links each commit to GitHub", async () => {
    vi.spyOn(api, "fetchCommits").mockResolvedValue({
      items: [
        {
          sha: "abc1234def567",
          author_login: "neo",
          committed_at: "2026-05-30T10:00:00Z",
          additions: 10,
          deletions: 2,
          is_bot: false,
          msg_first_line: "first commit",
        },
      ],
      stored: 200,
      total: 657,
      limit: 30,
      offset: 0,
    });
    renderPage();
    const btn = await screen.findByRole("button", { name: /commits/i });
    await userEvent.click(btn);
    await waitFor(() =>
      expect(screen.getByText("200 of 657 most recent")).toBeInTheDocument(),
    );
    const link = screen.getByRole("link", { name: /first commit/i });
    expect(link).toHaveAttribute(
      "href",
      "https://github.com/octocat/hello/commit/abc1234def567",
    );
    expect(link).toHaveAttribute("target", "_blank");
  });

  it("links each pull request to its GitHub page in a new tab", async () => {
    vi.spyOn(api, "fetchLatest").mockImplementation(async (_id, kind) => {
      if (kind === "prs") {
        return [
          {
            number: 42,
            author_login: "neo",
            state: "open",
            created_at: "2026-05-30T10:00:00Z",
            merged_at: null,
            closed_at: null,
            comments_count: 3,
            is_bot: false,
            title: "Add dark mode",
          },
        ] as api.LatestItem[];
      }
      return [];
    });
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /pull requests/i }));
    const link = await screen.findByRole("link", { name: /add dark mode/i });
    expect(link).toHaveAttribute("href", "https://github.com/octocat/hello/pull/42");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
  });

  it("links each issue to its GitHub page in a new tab", async () => {
    vi.spyOn(api, "fetchLatest").mockImplementation(async (_id, kind) => {
      if (kind === "issues") {
        return [
          {
            number: 7,
            author_login: "trinity",
            state: "open",
            created_at: "2026-05-29T10:00:00Z",
            closed_at: null,
            comments_count: 1,
            is_bot: false,
            title: "Crash on startup",
          },
        ] as api.LatestItem[];
      }
      return [];
    });
    renderPage();
    await userEvent.click(await screen.findByRole("button", { name: /issues/i }));
    const link = await screen.findByRole("link", { name: /crash on startup/i });
    expect(link).toHaveAttribute("href", "https://github.com/octocat/hello/issues/7");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
  });
});
