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
});

function renderPage(repo = REPO, onBack = vi.fn()) {
  return render(
    <MemoryRouter>
      <RepoDetail repo={repo} onBack={onBack} />
    </MemoryRouter>,
  );
}

describe("RepoDetail page", () => {
  it("renders detail sections", async () => {
    renderPage();
    expect(screen.getByRole("heading", { name: /octocat.*hello/i })).toBeInTheDocument();
    await waitFor(() => expect(screen.getAllByText("Contributors")[0]).toBeInTheDocument());
  });

  it("can switch tabs", async () => {
    renderPage();
    const btn = await screen.findByRole("button", { name: /commits/i });
    await userEvent.click(btn);
    await waitFor(() => expect(screen.getByText("Latest commits")).toBeInTheDocument());
  });
});
