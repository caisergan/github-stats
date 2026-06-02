import { describe, it, expect, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import Overview from "./Overview";
import * as D from "../data";

const MOCK_REPOS: D.MockRepo[] = [
  {
    id: 1,
    owner: "octocat",
    name: "hello",
    full_name: "octocat/hello",
    is_private: false,
    default_branch: "main",
    description: "Hi",
    lang: "Go",
    langColor: "#00ADD8",
    stargazers: 1,
    forks: 0,
    open_issues: 4,
    open_prs: 1,
    contributors: 3,
    releases: 0,
    commit_rate: 2,
    issue_rate: 0.5,
    pr_rate: 0.3,
    sync_status: "complete",
    last_synced_at: null,
    seed: 100,
  },
];

function renderPage(repos: D.MockRepo[] = MOCK_REPOS, onAdd = vi.fn()) {
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
    renderPage(MOCK_REPOS, onAdd);

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
});
