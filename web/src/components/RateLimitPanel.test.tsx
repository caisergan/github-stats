import { describe, it, expect, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { RateLimitPanel } from "./RateLimitPanel";
import * as api from "../api";

describe("RateLimitPanel", () => {
  it("renders REST and GraphQL usage measured against the 5,000/hr cap", async () => {
    vi.spyOn(api, "fetchRateLimit").mockResolvedValue({
      rest: { remaining: 4321, reset: "2026-06-25T23:00:00Z" },
      graphql: { remaining: 250, reset: "2026-06-25T22:30:00Z" },
    });
    render(<RateLimitPanel />);

    await waitFor(() => expect(screen.getByText("REST")).toBeInTheDocument());
    expect(screen.getByText("GraphQL")).toBeInTheDocument();

    // used = 5000 - remaining, formatted with thousands separators
    expect(screen.getByText(/679 used this hour/)).toBeInTheDocument(); // 5000 - 4321
    expect(screen.getByText(/4,750 used this hour/)).toBeInTheDocument(); // 5000 - 250
  });

  it("shows an error state when the fetch fails", async () => {
    vi.spyOn(api, "fetchRateLimit").mockRejectedValue(new Error("boom"));
    render(<RateLimitPanel />);

    await waitFor(() =>
      expect(screen.getByText(/couldn't load rate limits/i)).toBeInTheDocument(),
    );
  });
});
