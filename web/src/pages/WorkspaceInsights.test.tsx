import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import WorkspaceInsights from "./WorkspaceInsights";
import * as api from "../api";

const REPOS: api.Repo[] = [
  {
    id: 1,
    full_name: "octocat/alpha",
    is_private: false,
    default_branch: "main",
    sync_status: "complete",
    last_synced_at: null,
  },
  {
    id: 2,
    full_name: "octocat/beta",
    is_private: true,
    default_branch: "main",
    sync_status: "complete",
    last_synced_at: null,
  },
];

beforeEach(() => {
  vi.spyOn(api, "fetchMetrics").mockResolvedValue({
    commit_rate: { kind: "time_series", series: [{ date: "2026-01-01", value: 2 }] },
    pr_throughput: { kind: "time_series", series: [] },
    contributor_leaderboard: {
      kind: "leaderboard",
      rows: [{ login: "x", commits: 3, additions: 1, deletions: 0 }],
    },
  } as api.MetricsMap);
  vi.spyOn(api, "fetchOverview").mockResolvedValue({
    id: 1,
    full_name: "octocat/alpha",
    is_private: false,
    default_branch: "main",
    description: "",
    stargazers: 0,
    forks: 0,
    open_issues: 1,
    open_prs: 1,
    contributors: 2,
    commit_rate: 2,
    issue_rate: 0,
    pr_rate: 0,
    releases: 1,
    sync_status: "complete",
    last_synced_at: null,
    window_from: "",
    window_to: "",
  });
});

describe("WorkspaceInsights page", () => {
  it("renders aggregate sections and a language-unavailable note", async () => {
    render(
      <MemoryRouter>
        <WorkspaceInsights repos={REPOS} onOpen={vi.fn()} />
      </MemoryRouter>,
    );
    await waitFor(() =>
      expect(screen.getByText(/language breakdown isn.t available yet/i)).toBeInTheDocument(),
    );
    expect(screen.getByText(/top contributors/i)).toBeInTheDocument();
    expect(screen.getByText(/most active repositories/i)).toBeInTheDocument();
  });
});
