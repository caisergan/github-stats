import { describe, it, expect, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import RepoDetail from "./RepoDetail";
import * as D from "../data";

const MOCK_REPO: D.MockRepo = {
  id: 1,
  owner: "octocat",
  name: "hello",
  full_name: "octocat/hello",
  is_private: false,
  default_branch: "main",
  description: "Hi",
  lang: "Go",
  langColor: "#00ADD8",
  stargazers: 5,
  forks: 1,
  open_issues: 4,
  open_prs: 2,
  contributors: 3,
  releases: 7,
  commit_rate: 2.4,
  issue_rate: 0.5,
  pr_rate: 0.3,
  sync_status: "complete",
  last_synced_at: "2026-05-31T10:00:00Z",
  seed: 100,
};

function renderPage(repo = MOCK_REPO, onBack = vi.fn()) {
  return render(
    <MemoryRouter>
      <RepoDetail repo={repo} onBack={onBack} />
    </MemoryRouter>,
  );
}

describe("RepoDetail page", () => {
  it("renders detail sections", async () => {
    renderPage();
    await waitFor(() => expect(screen.getByRole("heading", { name: /octocat.*hello/i })).toBeInTheDocument());
    expect(screen.getAllByText("Contributors")[0]).toBeInTheDocument();
  });

  it("can switch tabs", async () => {
    renderPage();
    const btn = screen.getByRole("button", { name: /commits/i });
    await userEvent.click(btn);
    await waitFor(() => expect(screen.getByText("Latest commits")).toBeInTheDocument());
  });
});
