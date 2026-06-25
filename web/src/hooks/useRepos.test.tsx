import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";
import { useRepos } from "./useRepos";
import * as api from "../api";

const REPOS: api.Repo[] = [
  { id: 1, full_name: "octocat/Hello-World", is_private: false, default_branch: "main", sync_status: "complete", last_synced_at: null },
  { id: 2, full_name: "facebook/react", is_private: false, default_branch: "main", sync_status: "running", last_synced_at: null },
];

describe("useRepos", () => {
  beforeEach(() => {
    vi.spyOn(api, "listRepos").mockResolvedValue(REPOS);
  });

  it("loads repos and resolves owner/repo case-insensitively", async () => {
    const { result } = renderHook(() => useRepos());
    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(result.current.repos).toHaveLength(2);
    expect(result.current.resolve("octocat", "hello-world")?.id).toBe(1);
    expect(result.current.resolve("FACEBOOK", "REACT")?.id).toBe(2);
    expect(result.current.resolve("nobody", "nothing")).toBeNull();
  });

  it("add() appends a repo via the API and updates the list", async () => {
    const added: api.Repo = { id: 3, full_name: "a/b", is_private: true, default_branch: "main", sync_status: "pending", last_synced_at: null };
    vi.spyOn(api, "addRepo").mockResolvedValue(added);

    const { result } = renderHook(() => useRepos());
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.add("a/b");
    });
    expect(api.addRepo).toHaveBeenCalledWith("a/b");
    expect(result.current.repos.map((r) => r.id)).toContain(3);
  });

  it("remove() deletes a repo and drops it from the list", async () => {
    vi.spyOn(api, "deleteRepo").mockResolvedValue();
    const { result } = renderHook(() => useRepos());
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.remove(1);
    });
    expect(api.deleteRepo).toHaveBeenCalledWith(1);
    expect(result.current.repos.map((r) => r.id)).toEqual([2]);
  });
});
