import React, { useState, useMemo } from "react";
import { I } from "../components/Icons";
import { WindowControls, Kpi, MetricCard } from "../components/Components";
import {
  ContributionHeatmap,
  BarSeries,
  AreaSeries,
  Leaderboard,
  Sparkline,
} from "../components/Charts";
import * as D from "../data";
import * as F from "../format";

interface LangBarsProps {
  rows: D.LanguageRow[];
}

function LangBars({ rows }: LangBarsProps) {
  const max = Math.max(1, ...rows.map((r) => r.count));
  return (
    <div className="buckets" style={{ marginTop: 12 }}>
      {rows.map((r) => (
        <div
          className="row"
          key={r.label}
          style={{ gridTemplateColumns: "108px 1fr 56px" }}
        >
          <span className="lang">
            <span className="d" style={{ background: r.color }} />
            {r.label}
          </span>
          <span className="bar">
            <span style={{ width: `${(r.count / max) * 100}%`, background: r.color }} />
          </span>
          <span className="bn tnum">
            {r.count}
            <span className="muted" style={{ fontWeight: 400 }}>
              /d
            </span>
          </span>
        </div>
      ))}
    </div>
  );
}

interface ActiveReposTableProps {
  repos: D.MockRepo[];
  onOpen: (repo: D.MockRepo) => void;
}

function ActiveReposTable({ repos, onOpen }: ActiveReposTableProps) {
  const ranked = useMemo(() => {
    return [...repos].sort((a, b) => b.commit_rate - a.commit_rate);
  }, [repos]);

  const sparks = useMemo(() => {
    const m: Record<number, { date: string; value: number }[]> = {};
    repos.forEach((r) => {
      m[r.id] = D.makeMetrics(r.seed, 90).commit_rate.series;
    });
    return m;
  }, [repos]);

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
        {ranked.map((r, i) => (
          <tr
            key={r.id}
            style={{ cursor: "pointer" }}
            onClick={() => onOpen(r)}
          >
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
                  <span className="muted">{r.owner}/</span>
                  {r.name}
                </span>
                <span className="lang" style={{ marginLeft: 4 }}>
                  <span className="d" style={{ background: r.langColor }} />
                </span>
              </span>
            </td>
            <td className="num">
              <b className="tnum">
                {F.fmtRate(r.commit_rate).replace("/day", "")}
              </b>
            </td>
            <td className="num tnum">{r.contributors}</td>
            <td style={{ height: 1 }}>
              <div style={{ height: 30 }}>
                <Sparkline series={sparks[r.id]} height={30} />
              </div>
            </td>
            <td className="num">
              <I.chevRight style={{ width: 15, height: 15, color: "var(--faint)" }} />
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

interface WorkspaceInsightsProps {
  repos: D.MockRepo[];
  onOpen: (repo: D.MockRepo) => void;
}

export default function WorkspaceInsights({
  repos,
  onOpen,
}: WorkspaceInsightsProps) {
  const [win, setWin] = useState("90d");
  const [excludeBots, setExcludeBots] = useState(false);
  const days = D.WINDOW_DAYS[win] || 90;

  const commitSeries = useMemo(() => {
    return D.aggregateSeries(repos, "commit_rate", days);
  }, [repos, days]);

  const prSeries = useMemo(() => {
    return D.aggregateSeries(repos, "pr_throughput", days);
  }, [repos, days]);

  const heat = useMemo(() => {
    return D.aggregateHeatmap(repos);
  }, [repos]);

  const langs = useMemo(() => {
    return D.languageBreakdown(repos);
  }, [repos]);

  const board = useMemo(() => {
    const rows = D.mergedLeaderboard(repos);
    return { kind: "leaderboard" as const, rows };
  }, [repos]);

  const agg = useMemo(
    () => ({
      commits:
        Math.round(repos.reduce((a, r) => a + r.commit_rate, 0) * 10) / 10,
      prs: repos.reduce((a, r) => a + r.open_prs, 0),
      issues: repos.reduce((a, r) => a + r.open_issues, 0),
      contributors: board.rows.length,
      releases: repos.reduce((a, r) => a + r.releases, 0),
    }),
    [repos, board],
  );

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
        onWin={setWin}
        onBots={setExcludeBots}
      />

      <div className="kpi-strip" style={{ marginTop: 8 }}>
        <Kpi
          icon={I.commit}
          label="Commits / day"
          value={agg.commits}
          delta={11}
        />
        <Kpi
          icon={I.users}
          label="Contributors"
          value={agg.contributors}
          delta={5}
        />
        <Kpi icon={I.pr} label="Open PRs" value={agg.prs} delta={-4} />
        <Kpi
          icon={I.issue}
          label="Open issues"
          value={agg.issues}
          delta={7}
        />
        <Kpi icon={I.tag} label="Releases" value={agg.releases} delta={2} />
      </div>

      <div
        style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}
      >
        <MetricCard
          title="Workspace contribution activity"
          sub="Daily commits summed across every tracked repository"
        >
          <div style={{ marginTop: 10 }}>
            <ContributionHeatmap weeks={heat} />
          </div>
        </MetricCard>

        <div className="metric-grid two">
          <MetricCard
            title="Commits per day"
            sub="Aggregated across the workspace"
          >
            <div style={{ marginTop: 10 }}>
              <BarSeries series={commitSeries} unit="commits" />
            </div>
          </MetricCard>
          <MetricCard
            title="PRs merged per day"
            sub="Aggregated throughput"
          >
            <div style={{ marginTop: 10 }}>
              <AreaSeries series={prSeries} unit="PRs" />
            </div>
          </MetricCard>
        </div>

        <div className="metric-grid two">
          <MetricCard
            title="Activity by language"
            sub="Commit rate grouped by primary language"
          >
            <LangBars rows={langs} />
          </MetricCard>
          <MetricCard title="Top contributors" sub="Ranked across all repositories">
            <div style={{ marginTop: 8 }}>
              <Leaderboard result={board} compact />
            </div>
          </MetricCard>
        </div>

        <MetricCard
          title="Most active repositories"
          sub="Ranked by commit rate — click to open"
        >
          <div style={{ marginTop: 6 }}>
            <ActiveReposTable repos={repos} onOpen={onOpen} />
          </div>
        </MetricCard>
      </div>
    </div>
  );
}
