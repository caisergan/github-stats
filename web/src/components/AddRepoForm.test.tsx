import { describe, it, expect, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AddRepoForm } from "./Components";
import * as api from "../api";

describe("AddRepoForm picker", () => {
  it("lists the user's repos and tracks the one you pick", async () => {
    vi.spyOn(api, "fetchMyRepos").mockResolvedValue([
      { name_with_owner: "neo/alpha", is_private: false, description: "a", tracked: false },
      { name_with_owner: "neo/beta", is_private: true, description: "b", tracked: true },
    ]);
    const onAdd = vi.fn().mockResolvedValue(undefined);
    render(<AddRepoForm onAdd={onAdd} />);

    await userEvent.click(screen.getByLabelText(/repository/i));

    // The user's repos appear; the already-tracked one is labelled.
    await waitFor(() => expect(screen.getByText("neo/alpha")).toBeInTheDocument());
    expect(screen.getByText("Tracked")).toBeInTheDocument();

    // Picking the untracked repo tracks it by full name.
    await userEvent.click(screen.getByText("neo/alpha"));
    await waitFor(() => expect(onAdd).toHaveBeenCalledWith("neo/alpha"));
  });

  it("filters suggestions by what you type", async () => {
    vi.spyOn(api, "fetchMyRepos").mockResolvedValue([
      { name_with_owner: "neo/alpha", is_private: false, description: "", tracked: false },
      { name_with_owner: "neo/zeta", is_private: false, description: "", tracked: false },
    ]);
    render(<AddRepoForm onAdd={vi.fn().mockResolvedValue(undefined)} />);

    const input = screen.getByLabelText(/repository/i);
    await userEvent.click(input);
    await waitFor(() => expect(screen.getByText("neo/zeta")).toBeInTheDocument());

    await userEvent.type(input, "zeta");
    expect(screen.getByText("neo/zeta")).toBeInTheDocument();
    expect(screen.queryByText("neo/alpha")).not.toBeInTheDocument();
  });
});
