import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import Overview from "./Overview";
import * as api from "../api";

const REPOS: api.Repo[] = [
  {
    id: 1,
    full_name: "octocat/hello",
    is_private: false,
    default_branch: "main",
    description: "Hi",
    stargazers: 1,
    forks: 0,
    sync_status: "complete",
    last_synced_at: null,
  },
];

beforeEach(() => {
  vi.spyOn(api, "fetchOverview").mockResolvedValue({
    id: 1,
    full_name: "octocat/hello",
    is_private: false,
    default_branch: "main",
    description: "Hi",
    stargazers: 1,
    forks: 0,
    open_issues: 4,
    open_prs: 1,
    contributors: 3,
    commit_rate: 2,
    issue_rate: 0.5,
    pr_rate: 0.3,
    releases: 0,
    sync_status: "complete",
    last_synced_at: null,
    window_from: "2026-01-01",
    window_to: "2026-03-31",
  });
  vi.spyOn(api, "fetchMetrics").mockResolvedValue({
    commit_rate: { kind: "time_series", series: [] },
  } as api.MetricsMap);
  vi.spyOn(api, "listCollections").mockResolvedValue([]);
});

function renderPage(repos: api.Repo[] = REPOS, onAdd = vi.fn()) {
  return render(
    <MemoryRouter>
      <Overview repos={repos} onOpen={vi.fn()} onAdd={onAdd} />
    </MemoryRouter>,
  );
}

describe("Overview page", () => {
  it("renders a repo card with name", async () => {
    renderPage();
    await waitFor(() => expect(screen.getByText(/hello/)).toBeInTheDocument());
  });

  it("adding a repo calls onAdd and shows the action", async () => {
    const onAdd = vi.fn();
    renderPage(REPOS, onAdd);

    const input = screen.getByPlaceholderText("owner/name");
    await userEvent.type(input, "a/b");

    const btn = screen.getByRole("button", { name: /track repo/i });
    await userEvent.click(btn);

    await waitFor(() => expect(onAdd).toHaveBeenCalledWith("a/b"));
  });

  it("shows an empty state when no repos are tracked", async () => {
    renderPage([]);
    await waitFor(() => expect(screen.getByText(/no repositories match/i)).toBeInTheDocument());
  });

  it("surfaces an error when onAdd rejects instead of silently swallowing it", async () => {
    const onAdd = vi
      .fn()
      .mockRejectedValue(new Error("fetch repo failed: graphql: empty data"));
    renderPage(REPOS, onAdd);

    const input = screen.getByPlaceholderText("owner/name");
    await userEvent.type(input, "caisergan/trade-station");
    await userEvent.click(screen.getByRole("button", { name: /track repo/i }));

    await waitFor(() =>
      expect(screen.getByText(/fetch repo failed/i)).toBeInTheDocument(),
    );
  });

  it("offers a grant-access path when onAdd fails with a RepoAccessError", async () => {
    const onAdd = vi
      .fn()
      .mockRejectedValue(new api.RepoAccessError("Couldn't access caisergan/trade-station."));
    renderPage(REPOS, onAdd);

    await userEvent.type(
      screen.getByPlaceholderText("owner/name"),
      "caisergan/trade-station",
    );
    await userEvent.click(screen.getByRole("button", { name: /track repo/i }));

    await waitFor(() =>
      expect(screen.getByText(/couldn't access/i)).toBeInTheDocument(),
    );
    expect(screen.getByRole("link", { name: /reconnect github/i })).toHaveAttribute(
      "href",
      "/auth/github",
    );
    expect(screen.getByRole("link", { name: /settings/i })).toHaveAttribute(
      "href",
      "/settings",
    );
  });
});
