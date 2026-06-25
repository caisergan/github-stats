import { describe, it, expect } from "vitest";
import { sumSeries, mergeLeaderboards, sumHeatmaps, seriesToHeatmap } from "./aggregate";
import type { SeriesPoint, LeaderRow } from "./api";

describe("sumSeries", () => {
  it("sums values per date across series and sorts by date", () => {
    const a: SeriesPoint[] = [{ date: "2026-01-01", value: 2 }, { date: "2026-01-02", value: 3 }];
    const b: SeriesPoint[] = [{ date: "2026-01-02", value: 4 }, { date: "2026-01-03", value: 1 }];
    expect(sumSeries([a, b])).toEqual([
      { date: "2026-01-01", value: 2 },
      { date: "2026-01-02", value: 7 },
      { date: "2026-01-03", value: 1 },
    ]);
  });
  it("returns [] for no input", () => {
    expect(sumSeries([])).toEqual([]);
  });
});

describe("mergeLeaderboards", () => {
  it("merges rows by login and sorts by commits desc", () => {
    const a: LeaderRow[] = [{ login: "x", commits: 5, additions: 10, deletions: 1 }];
    const b: LeaderRow[] = [
      { login: "x", commits: 2, additions: 4, deletions: 0 },
      { login: "y", commits: 9, additions: 1, deletions: 1 },
    ];
    expect(mergeLeaderboards([a, b])).toEqual([
      { login: "y", commits: 9, additions: 1, deletions: 1 },
      { login: "x", commits: 7, additions: 14, deletions: 1 },
    ]);
  });
});

describe("sumHeatmaps", () => {
  it("element-wise sums equally-shaped grids", () => {
    expect(sumHeatmaps([[[1, 2]], [[3, 4]]])).toEqual([[4, 6]]);
  });
});

describe("seriesToHeatmap", () => {
  it("produces a 53x7 grid placing each date's value at [week][weekday]", () => {
    const grid = seriesToHeatmap([{ date: "2026-01-01", value: 5 }]); // Thu = weekday 4
    expect(grid).toHaveLength(53);
    expect(grid[0]).toHaveLength(7);
    const total = grid.flat().reduce((s, v) => s + v, 0);
    expect(total).toBe(5);
  });
});
