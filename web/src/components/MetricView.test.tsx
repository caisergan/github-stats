import { describe, it, expect, vi } from "vitest";
import { render } from "@testing-library/react";
import MetricView from "./MetricView";
import type { Result } from "../api";

// Stub the uPlot-backed chart so the time_series branch needs no canvas.
vi.mock("./TimeSeriesChart", () => ({
  default: ({ label }: { label?: string }) => <div data-testid="ts-chart">{label}</div>,
}));

describe("MetricView kind switching", () => {
  it("renders the chart for time_series", () => {
    const r: Result = { kind: "time_series", label: "Commits/day", series: [{ date: "2026-05-01", value: 1 }] };
    const { getByTestId } = render(<MetricView result={r} />);
    expect(getByTestId("ts-chart")).toHaveTextContent("Commits/day");
  });

  it("renders a formatted hours value for scalar", () => {
    const r: Result = { kind: "scalar", label: "median", value: 12.5, unit: "hours", count: 4 };
    const { getByText } = render(<MetricView result={r} />);
    expect(getByText("12.5h")).toBeInTheDocument();
    expect(getByText(/n = 4/)).toBeInTheDocument();
  });

  it("renders bucket rows for buckets", () => {
    const r: Result = { kind: "buckets", label: "Open issue age", buckets: [
      { label: "<24h", count: 2 },
      { label: "older", count: 9 },
    ] };
    const { getByText } = render(<MetricView result={r} />);
    expect(getByText("<24h")).toBeInTheDocument();
    expect(getByText("older")).toBeInTheDocument();
  });

  it("renders contributor rows for leaderboard", () => {
    const r: Result = { kind: "leaderboard", label: "Top contributors", rows: [
      { login: "neo", commits: 12, additions: 100, deletions: 5 },
    ] };
    const { getByText } = render(<MetricView result={r} />);
    expect(getByText("neo")).toBeInTheDocument();
    expect(getByText("12")).toBeInTheDocument();
  });

  it("falls back gracefully for an unknown kind", () => {
    const r = { kind: "mystery" } as unknown as Result;
    const { getByText } = render(<MetricView result={r} />);
    expect(getByText(/unsupported/i)).toBeInTheDocument();
  });
});
