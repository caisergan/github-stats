import { describe, it, expect, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import App from "./App";
import * as api from "./api";

describe("App auth gate", () => {
  it("shows sign-in when unauthenticated", async () => {
    vi.spyOn(api, "fetchMe").mockResolvedValue(null);
    vi.spyOn(api, "listRepos").mockResolvedValue([]);
    vi.spyOn(api, "listCollections").mockResolvedValue([]);
    render(
      <MemoryRouter>
        <App />
      </MemoryRouter>,
    );
    await waitFor(() => expect(screen.getByText(/sign in/i)).toBeInTheDocument());
  });

  it("renders the shell when authenticated", async () => {
    vi.spyOn(api, "fetchMe").mockResolvedValue({
      id: 1,
      github_id: 9,
      login: "maya",
      avatar_url: "",
    });
    vi.spyOn(api, "listRepos").mockResolvedValue([]);
    vi.spyOn(api, "listCollections").mockResolvedValue([]);
    render(
      <MemoryRouter>
        <App />
      </MemoryRouter>,
    );
    await waitFor(() => expect(screen.getByText(/maya/i)).toBeInTheDocument());
  });
});
