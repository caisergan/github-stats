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
