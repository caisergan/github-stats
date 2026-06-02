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
    await waitFor(() => expect(screen.getAllByText("Contributors")[0]).toBeInTheDocument());
    await waitFor(() => expect(screen.getByText("neo")).toBeInTheDocument()); // leaderboard
    expect(screen.getByText("12.5h")).toBeInTheDocument(); // scalar
  });

  it("shows an add-this-repo affordance when untracked", async () => {
    renderAt("/nobody/nothing");
    await waitFor(() => expect(screen.getByText(/not tracked/i)).toBeInTheDocument());
    expect(screen.getByRole("button", { name: /track this repo/i })).toBeInTheDocument();
  });

  it("changing the window triggers a refetch", async () => {
    renderAt("/octocat/hello");
    await waitFor(() => expect(api.fetchMetrics).toHaveBeenCalled());
    const callsBefore = (api.fetchMetrics as ReturnType<typeof vi.fn>).mock.calls.length;
    await userEvent.selectOptions(screen.getByRole("combobox"), "90d");
    await waitFor(() =>
      expect((api.fetchMetrics as ReturnType<typeof vi.fn>).mock.calls.length).toBeGreaterThan(callsBefore),
    );
  });
});
