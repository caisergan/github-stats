import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import Collections from "./Collections";
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
];

const COLLECTIONS: api.Collection[] = [{ id: 10, name: "Platform", repo_ids: [1] }];

beforeEach(() => {
  vi.spyOn(api, "fetchMetrics").mockResolvedValue({
    commit_rate: { kind: "time_series", series: [] },
  } as api.MetricsMap);
  vi.spyOn(api, "fetchOverview").mockResolvedValue({
    id: 1,
    full_name: "octocat/alpha",
    is_private: false,
    default_branch: "main",
    description: "",
    stargazers: 0,
    forks: 0,
    open_issues: 0,
    open_prs: 0,
    contributors: 1,
    commit_rate: 1,
    issue_rate: 0,
    pr_rate: 0,
    releases: 0,
    sync_status: "complete",
    last_synced_at: null,
    window_from: "",
    window_to: "",
  });
});

function renderPage(onCreate = vi.fn()) {
  return render(
    <MemoryRouter>
      <Collections
        repos={REPOS}
        collections={COLLECTIONS}
        onOpenRepo={vi.fn()}
        onCreate={onCreate}
      />
    </MemoryRouter>,
  );
}

describe("Collections page", () => {
  it("renders a collection name", async () => {
    renderPage();
    expect(screen.getByText("Platform")).toBeInTheDocument();
    // let the card's per-member fan-out settle so no state update escapes act()
    await waitFor(() => expect(api.fetchOverview).toHaveBeenCalled());
  });

  it("creates a collection by name via the modal", async () => {
    const onCreate = vi.fn();
    renderPage(onCreate);

    await userEvent.click(screen.getByRole("button", { name: /new collection/i }));
    await userEvent.type(screen.getByPlaceholderText(/platform team/i), "DevOps");
    await userEvent.click(screen.getByRole("button", { name: /create collection/i }));

    await waitFor(() => expect(onCreate).toHaveBeenCalledWith("DevOps"));
  });
});
