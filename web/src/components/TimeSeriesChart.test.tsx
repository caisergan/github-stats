import { describe, it, expect, vi, beforeEach } from "vitest";
import { render } from "@testing-library/react";
import TimeSeriesChart from "./TimeSeriesChart";
import type { SeriesPoint } from "../api";

// Mock uPlot: it touches canvas, which jsdom lacks. Capture instance methods.
const instances: any[] = [];
vi.mock("uplot", () => {
  class MockUPlot {
    setData = vi.fn();
    setSize = vi.fn();
    destroy = vi.fn();
    constructor() {
      instances.push(this);
    }
  }
  return {
    default: MockUPlot,
  };
});

const SERIES: SeriesPoint[] = [
  { date: "2026-05-01", value: 3 },
  { date: "2026-05-02", value: 5 },
];

describe("TimeSeriesChart", () => {
  beforeEach(() => {
    instances.length = 0;
  });

  it("constructs a uPlot instance on mount", () => {
    render(<TimeSeriesChart series={SERIES} label="Commits/day" />);
    expect(instances).toHaveLength(1);
  });

  it("destroys the uPlot instance on unmount", () => {
    const { unmount } = render(<TimeSeriesChart series={SERIES} label="Commits/day" />);
    const inst = instances[0];
    unmount();
    expect(inst.destroy).toHaveBeenCalledTimes(1);
  });

  it("renders an empty state for an empty series without constructing uPlot", () => {
    const { getByText } = render(<TimeSeriesChart series={[]} label="Commits/day" />);
    expect(getByText(/no data/i)).toBeInTheDocument();
    expect(instances).toHaveLength(0);
  });

  it("pushes new data via setData when series changes", () => {
    const { rerender } = render(<TimeSeriesChart series={SERIES} label="x" />);
    const inst = instances[0];
    rerender(<TimeSeriesChart series={[...SERIES, { date: "2026-05-03", value: 9 }]} label="x" />);
    expect(inst.setData).toHaveBeenCalled();
  });
});
