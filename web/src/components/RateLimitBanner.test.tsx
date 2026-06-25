import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { RateLimitBanner } from "./RateLimitBanner";

describe("RateLimitBanner", () => {
  it("shows nothing when budget is healthy", () => {
    render(<RateLimitBanner rateLimit={{ rest: { remaining: 4000, reset: "" }, graphql: { remaining: 4000, reset: "" } }} />);
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("warns with a reset time when throttled", () => {
    render(
      <RateLimitBanner
        rateLimit={{ rest: { remaining: 5, reset: "2026-05-31T12:00:00Z" }, graphql: { remaining: 4000, reset: "" } }}
        threshold={50}
      />,
    );
    const alert = screen.getByRole("alert");
    expect(alert.textContent).toMatch(/rate limit|throttl/i);
    expect(alert.textContent).toMatch(/reset/i);
  });
});
