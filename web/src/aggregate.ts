import type { SeriesPoint, LeaderRow } from "./api";

/** Sum multiple daily series element-wise by date; result sorted ascending by date. */
export function sumSeries(seriesList: SeriesPoint[][]): SeriesPoint[] {
  const acc = new Map<string, number>();
  for (const series of seriesList) {
    for (const p of series) acc.set(p.date, (acc.get(p.date) ?? 0) + p.value);
  }
  return [...acc.entries()]
    .map(([date, value]) => ({ date, value }))
    .sort((a, b) => a.date.localeCompare(b.date));
}

/** Merge contributor leaderboards across repos by login; sorted by commits desc. */
export function mergeLeaderboards(boards: LeaderRow[][]): LeaderRow[] {
  const acc = new Map<string, LeaderRow>();
  for (const board of boards) {
    for (const r of board) {
      const cur = acc.get(r.login) ?? { login: r.login, commits: 0, additions: 0, deletions: 0 };
      cur.commits += r.commits;
      cur.additions += r.additions;
      cur.deletions += r.deletions;
      acc.set(r.login, cur);
    }
  }
  return [...acc.values()].sort((a, b) => b.commits - a.commits);
}

/** Element-wise sum of equally-shaped numeric grids. */
export function sumHeatmaps(grids: number[][][]): number[][] {
  if (grids.length === 0) return [];
  const rows = grids[0].length;
  const cols = grids[0][0]?.length ?? 0;
  const out: number[][] = Array.from({ length: rows }, () => Array(cols).fill(0));
  for (const g of grids) {
    for (let r = 0; r < rows; r++) for (let c = 0; c < cols; c++) out[r][c] += g[r]?.[c] ?? 0;
  }
  return out;
}

/**
 * Reshape a daily commit series into a 53-week x 7-day grid (week index 0..52,
 * weekday 0=Sun..6=Sat), placing the most recent date in the last column.
 */
export function seriesToHeatmap(series: SeriesPoint[]): number[][] {
  const grid: number[][] = Array.from({ length: 53 }, () => Array(7).fill(0));
  if (series.length === 0) return grid;
  const sorted = [...series].sort((a, b) => a.date.localeCompare(b.date));
  const last = new Date(sorted[sorted.length - 1].date + "T00:00:00Z");
  for (const p of sorted) {
    const d = new Date(p.date + "T00:00:00Z");
    const daysAgo = Math.round((last.getTime() - d.getTime()) / 86_400_000);
    const weekFromEnd = Math.floor(daysAgo / 7);
    if (weekFromEnd > 52) continue;
    const week = 52 - weekFromEnd;
    const weekday = d.getUTCDay();
    grid[week][weekday] += p.value;
  }
  return grid;
}
