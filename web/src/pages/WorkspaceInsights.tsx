import { useState } from "react";
import { I } from "../components/Icons";
import { WindowControls, Kpi, MetricCard } from "../components/Components";
import {
  ContributionHeatmap,
  BarSeries,
  AreaSeries,
  Leaderboard,
  Sparkline,
} from "../components/Charts";
import { useAsync } from "../hooks/useAsync";
import { fetchMetrics, fetchOverview } from "../api";
import type {
  MetricsMap,
  SeriesPoint,
  LeaderRow,
  Overview as OverviewT,
  Repo,
  WindowSpec,
} from "../api";
import { sumSeries, mergeLeaderboards, sumHeatmaps, seriesToHeatmap } from "../aggregate";
import * as F from "../format";

// --- MetricsMap is a tagged union keyed by `kind`; narrow per key. ----------
function seriesOf(m: MetricsMap, key: string): SeriesPoint[] {
  const r = m[key];
  return r && r.kind === "time_series" ? r.series : [];
}
function rowsOf(m: MetricsMap, key: string): LeaderRow[] {
  const r = m[key];
  return r && r.kind === "leaderboard" ? r.rows : [];
}

interface ActiveReposTableProps {
  repos: Repo[];
  overviews: Record<number, OverviewT>;
  sparks: Record<number, SeriesPoint[]>;
  onOpen: (repo: Repo) => void;
}

function ActiveReposTable({ repos, overviews, sparks, onOpen }: ActiveReposTableProps) {
  const ranked = [...repos].sort(
    (a, b) => (overviews[b.id]?.commit_rate ?? 0) - (overviews[a.id]?.commit_rate ?? 0),
  );

  return (
    <table className="tbl">
      <thead>
        <tr>
          <th style={{ width: 30 }}></th>
          <th>Repository</th>
          <th className="num">Commits/day</th>
          <th className="num">Contributors</th>
          <th style={{ width: 140 }}>Trend</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        {ranked.map((r, i) => {
          const { owner, name } = F.splitRepo(r.full_name);
          const o = overviews[r.id];
          return (
            <tr key={r.id} style={{ cursor: "pointer" }} onClick={() => onOpen(r)}>
              <td>
                <span className={"rank" + (i === 0 ? " r1" : "")}>{i + 1}</span>
              </td>
              <td>
                <span className="who">
                  {r.is_private ? (
                    <I.lock style={{ width: 14, height: 14, color: "var(--muted)" }} />
                  ) : (
                    <I.repo style={{ width: 14, height: 14, color: "var(--muted)" }} />
                  )}
                  <span style={{ fontWeight: 500 }}>
                    <span className="muted">{owner}/</span>
                    {name}
                  </span>
                </span>
              </td>
              <td className="num">
                <b className="tnum">{F.fmtRate(o?.commit_rate ?? 0).replace("/day", "")}</b>
              </td>
              <td className="num tnum">{o?.contributors ?? 0}</td>
              <td style={{ height: 1 }}>
                <div style={{ height: 30 }}>
                  <Sparkline series={sparks[r.id] ?? []} height={30} />
                </div>
              </td>
              <td className="num">
                <I.chevRight style={{ width: 15, height: 15, color: "var(--faint)" }} />
              </td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

interface WorkspaceInsightsProps {
  repos: Repo[];
  onOpen: (repo: Repo) => void;
}

export default function WorkspaceInsights({ repos, onOpen }: WorkspaceInsightsProps) {
  const [win, setWin] = useState<WindowSpec>("90d");
  const [excludeBots, setExcludeBots] = useState(false);

  const ids = repos.map((r) => r.id).join(",");
  const agg = useAsync(async () => {
    const per = await Promise.all(
      repos.map((r) =>
        fetchMetrics(r.id, {
          window: win,
          excludeBots,
          keys: ["commit_rate", "pr_throughput", "contributor_leaderboard"],
        }),
      ),
    );
    const overviews = await Promise.all(
      repos.map((r) => fetchOverview(r.id, { window: win, excludeBots })),
    );
    const commitSeriesList = per.map((m) => seriesOf(m, "commit_rate"));
    return {
      commitSeries: sumSeries(commitSeriesList),
      prSeries: sumSeries(per.map((m) => seriesOf(m, "pr_throughput"))),
      heat: sumHeatmaps(commitSeriesList.map(seriesToHeatmap)),
      board: mergeLeaderboards(per.map((m) => rowsOf(m, "contributor_leaderboard"))),
      overviews,
      perRepoCommits: Object.fromEntries(
        repos.map((r, i) => [r.id, commitSeriesList[i]]),
      ) as Record<number, SeriesPoint[]>,
    };
  }, [ids, win, excludeBots]);

  const data = agg.data;
  const overviewsMap: Record<number, OverviewT> = data
    ? Object.fromEntries(data.overviews.map((o) => [o.id, o]))
    : {};
  const kpi = data
    ? data.overviews.reduce(
        (a, o) => ({
          commits: a.commits + o.commit_rate,
          prs: a.prs + o.open_prs,
          issues: a.issues + o.open_issues,
          releases: a.releases + o.releases,
        }),
        { commits: 0, prs: 0, issues: 0, releases: 0 },
      )
    : { commits: 0, prs: 0, issues: 0, releases: 0 };

  return (
    <div className="page fade-in">
      <div className="page-head">
        <div>
          <h1>Insights</h1>
          <div className="sub">
            Cross-repository analytics across all {repos.length} tracked repositories
          </div>
        </div>
      </div>

      <WindowControls
        win={win}
        excludeBots={excludeBots}
        onWin={(w) => setWin(w as WindowSpec)}
        onBots={setExcludeBots}
      />

      {!data ? (
        <div className="empty" style={{ padding: 48, textAlign: "center" }}>
          Loading…
        </div>
      ) : (
        <>
          <div className="kpi-strip" style={{ marginTop: 8 }}>
            <Kpi
              icon={I.commit}
              label="Commits / day"
              value={Math.round(kpi.commits * 10) / 10}
              delta={11}
            />
            <Kpi
              icon={I.users}
              label="Contributors"
              value={data.board.length}
              delta={5}
            />
            <Kpi icon={I.pr} label="Open PRs" value={kpi.prs} delta={-4} />
            <Kpi icon={I.issue} label="Open issues" value={kpi.issues} delta={7} />
            <Kpi icon={I.tag} label="Releases" value={kpi.releases} delta={2} />
          </div>

          <div style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}>
            <MetricCard
              title="Workspace contribution activity"
              sub="Daily commits summed across every tracked repository"
            >
              <div style={{ marginTop: 10 }}>
                <ContributionHeatmap weeks={data.heat} />
              </div>
            </MetricCard>

            <div className="metric-grid two">
              <MetricCard title="Commits per day" sub="Aggregated across the workspace">
                <div style={{ marginTop: 10 }}>
                  <BarSeries series={data.commitSeries} unit="commits" />
                </div>
              </MetricCard>
              <MetricCard title="PRs merged per day" sub="Aggregated throughput">
                <div style={{ marginTop: 10 }}>
                  <AreaSeries series={data.prSeries} unit="PRs" />
                </div>
              </MetricCard>
            </div>

            <div className="metric-grid two">
              <MetricCard
                title="Activity by language"
                sub="Not available yet"
              >
                <p className="empty">
                  Language breakdown isn't available yet — language data isn't collected.
                </p>
              </MetricCard>
              <MetricCard title="Top contributors" sub="Ranked across all repositories">
                <div style={{ marginTop: 8 }}>
                  <Leaderboard
                    result={{ rows: data.board.map((r) => ({ ...r, img: F.avatarURL(r.login) })) }}
                    compact
                  />
                </div>
              </MetricCard>
            </div>

            <MetricCard
              title="Most active repositories"
              sub="Ranked by commit rate — click to open"
            >
              <div style={{ marginTop: 6 }}>
                <ActiveReposTable
                  repos={repos}
                  overviews={overviewsMap}
                  sparks={data.perRepoCommits}
                  onOpen={onOpen}
                />
              </div>
            </MetricCard>
          </div>
        </>
      )}
    </div>
  );
}
