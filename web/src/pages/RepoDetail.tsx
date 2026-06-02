import React, { useState, useMemo } from "react";
import { I } from "../components/Icons";
import { SyncStatusBadge } from "../components/UI";
import {
  WindowControls,
  RefreshButton,
  Kpi,
  MetricCard,
  LatestList,
} from "../components/Components";
import {
  ContributionHeatmap,
  BarSeries,
  AreaSeries,
  ScalarStat,
  BucketsBar,
  Leaderboard,
} from "../components/Charts";
import * as D from "../data";
import * as F from "../format";

const TABS = [
  { id: "insights", label: "Insights", icon: I.activity },
  { id: "commits", label: "Commits", icon: I.commit },
  { id: "issues", label: "Issues", icon: I.issue },
  { id: "prs", label: "Pull Requests", icon: I.pr },
  { id: "contributors", label: "Contributors", icon: I.users },
  { id: "releases", label: "Releases", icon: I.tag },
];

interface RepoDetailProps {
  repo: D.MockRepo;
  onBack: () => void;
}

export default function RepoDetail({ repo, onBack }: RepoDetailProps) {
  const [tab, setTab] = useState("insights");
  const [win, setWin] = useState("90d");
  const [excludeBots, setExcludeBots] = useState(false);
  const [nonce, setNonce] = useState(0); // bump on refresh to re-derive

  const days = D.WINDOW_DAYS[win] || 90;
  const ov = useMemo(() => {
    return D.overview(repo, win, excludeBots);
  }, [repo, win, excludeBots, nonce]);

  const m = useMemo(() => {
    return D.makeMetrics(repo.seed + (excludeBots ? 7 : 0), days);
  }, [repo, days, excludeBots, nonce]);

  const commits = useMemo(() => {
    let c = D.latestCommits(repo.seed, 14);
    if (excludeBots) c = c.filter((x) => !x.is_bot);
    return c;
  }, [repo, excludeBots, nonce]);

  const prs = useMemo(() => {
    let p = D.latestPRs(repo.seed + 1, 12);
    if (excludeBots) p = p.filter((x) => !x.is_bot);
    return p;
  }, [repo, excludeBots, nonce]);

  const issues = useMemo(() => {
    return D.latestIssues(repo.seed + 2, 12);
  }, [repo, nonce]);

  const tabCounts: Record<string, number> = {
    commits: commits.length,
    issues: ov.open_issues,
    prs: ov.open_prs,
  };

  return (
    <div className="page fade-in">
      <div className="breadcrumb">
        <a onClick={onBack}>
          <I.chevLeft style={{ width: 14, height: 14 }} />
        </a>
        <a onClick={onBack}>Repositories</a>
        <I.chevRight style={{ width: 12, height: 12, color: "var(--faint)" }} />
        <span style={{ color: "var(--fg-2)" }}>
          {repo.owner}/{repo.name}
        </span>
      </div>

      <div className="detail-head">
        <div className="title">
          <div
            className="logo"
            style={{
              width: 38,
              height: 38,
              borderRadius: 9,
              background: "var(--accent)",
              color: "var(--accent-fg)",
              display: "grid",
              placeItems: "center",
            }}
          >
            <I.repo style={{ width: 19, height: 19 }} />
          </div>
          <div>
            <h1>
              <span className="owner">{repo.owner}</span>
              <span className="slash">/</span>
              {repo.name}
            </h1>
            <div className="meta">
              <span className="m">
                {repo.is_private ? (
                  <>
                    <I.lock style={{ width: 14, height: 14 }} />
                    Private
                  </>
                ) : (
                  <>
                    <I.globe style={{ width: 14, height: 14 }} />
                    Public
                  </>
                )}
              </span>
              <span className="m">
                <I.branch style={{ width: 14, height: 14 }} />
                {repo.default_branch}
              </span>
              <span className="m">
                <I.star style={{ width: 14, height: 14 }} />
                {F.fmtNumber(repo.stargazers)}
              </span>
              <span className="m">
                <I.fork style={{ width: 14, height: 14 }} />
                {F.fmtNumber(repo.forks)}
              </span>
              <span className="m">
                <I.clock style={{ width: 14, height: 14 }} />
                synced {F.fmtNullableTs(repo.last_synced_at)}
              </span>
            </div>
          </div>
        </div>
        <div className="actions">
          <SyncStatusBadge status={repo.sync_status} />
          <RefreshButton
            onComplete={() => {
              repo.last_synced_at = D.isoDaysAgo(0);
              repo.sync_status = "complete";
              setNonce((n) => n + 1);
            }}
          />
        </div>
      </div>

      <WindowControls
        win={win}
        excludeBots={excludeBots}
        onWin={setWin}
        onBots={setExcludeBots}
      />

      {/* Details KPI strip (always visible) */}
      <div className="kpi-strip" style={{ marginTop: 8 }}>
        <Kpi
          icon={I.commit}
          label="Commit rate"
          value={F.fmtRate(ov.commit_rate).replace("/day", "")}
          unit="/day"
          delta={9}
        />
        <Kpi icon={I.pr} label="Open PRs" value={ov.open_prs} delta={-3} />
        <Kpi icon={I.issue} label="Open issues" value={ov.open_issues} delta={5} />
        <Kpi icon={I.users} label="Contributors" value={ov.contributors} delta={2} />
        <Kpi icon={I.tag} label="Releases" value={ov.releases} delta={0} />
      </div>

      <div className="tabs">
        {TABS.map((t) => (
          <button
            key={t.id}
            className={tab === t.id ? "active" : ""}
            onClick={() => setTab(t.id)}
          >
            <t.icon style={{ width: 15, height: 15 }} />
            {t.label}
            {tabCounts[t.id] != null && (
              <span className="count">{tabCounts[t.id]}</span>
            )}
          </button>
        ))}
      </div>

      {tab === "insights" && <InsightsTab m={m} />}
      {tab === "commits" && <CommitsTab m={m} commits={commits} />}
      {tab === "issues" && <IssuesTab m={m} issues={issues} />}
      {tab === "prs" && <PrsTab m={m} prs={prs} />}
      {tab === "contributors" && <ContributorsTab m={m} />}
      {tab === "releases" && <ReleasesTab repo={repo} ov={ov} />}
    </div>
  );
}

/* ---- tabs ---- */
function InsightsTab({ m }: { m: D.MockMetricsMap }) {
  return (
    <div
      className="fade-in"
      style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}
    >
      <MetricCard
        title="Contribution activity"
        sub="Daily commits across the last year"
        span
      >
        <div style={{ marginTop: 10 }}>
          <ContributionHeatmap weeks={m.heatmap} />
        </div>
      </MetricCard>
      <div className="metric-grid two">
        <MetricCard title="Commits per day" sub="Commit throughput over the window">
          <div style={{ marginTop: 10 }}>
            <BarSeries series={m.commit_rate.series} unit="commits" />
          </div>
        </MetricCard>
        <MetricCard title="Code churn" sub="Lines added + removed per day">
          <div style={{ marginTop: 10 }}>
            <AreaSeries series={m.code_churn.series} unit="lines" />
          </div>
        </MetricCard>
      </div>
      <div className="metric-grid">
        <MetricCard title="Time to merge" sub="Median, opened → merged">
          <ScalarStat result={m.time_to_merge} />
        </MetricCard>
        <MetricCard title="Review latency" sub="Median, opened → first review">
          <ScalarStat result={m.review_latency} />
        </MetricCard>
        <MetricCard title="Issue lifetime" sub="Median, opened → closed">
          <ScalarStat result={m.issue_lifetime} />
        </MetricCard>
        <MetricCard title="Open issue age" sub="Distribution of open issues">
          <BucketsBar result={m.open_issue_age} />
        </MetricCard>
        <MetricCard title="PR throughput" sub="PRs merged per day">
          <div style={{ marginTop: 4 }}>
            <BarSeries
              series={m.pr_throughput.series}
              unit="PRs"
              height={150}
            />
          </div>
        </MetricCard>
        <MetricCard title="Comment volume" sub="Comments per day">
          <div style={{ marginTop: 4 }}>
            <AreaSeries
              series={m.comment_volume.series}
              unit="comments"
              height={150}
            />
          </div>
        </MetricCard>
      </div>
    </div>
  );
}

function CommitsTab({
  m,
  commits,
}: {
  m: D.MockMetricsMap;
  commits: D.MockCommit[];
}) {
  return (
    <div
      className="fade-in"
      style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}
    >
      <MetricCard
        title="Commits per day"
        sub="Hover any column for the exact count"
        span
      >
        <div style={{ marginTop: 10 }}>
          <BarSeries series={m.commit_rate.series} unit="commits" height={220} />
        </div>
      </MetricCard>
      <MetricCard title="Latest commits" sub={`${commits.length} most recent`}>
        <LatestList kind="commits" items={commits} limit={10} />
      </MetricCard>
    </div>
  );
}

function IssuesTab({
  m,
  issues,
}: {
  m: D.MockMetricsMap;
  issues: D.MockIssue[];
}) {
  const openTotal = m.open_issue_age.buckets.reduce((a, b) => a + b.count, 0);
  const stale = m.open_issue_age.buckets
    .filter(
      (b) =>
        /mo|wk|4w|3mo/.test(b.label) ||
        b.label === "> 3mo" ||
        b.label === "1–3mo",
    )
    .reduce((a, b) => a + b.count, 0);
  return (
    <div
      className="fade-in"
      style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}
    >
      <div className="stat-row">
        <MetricCard title="Issue lifetime" sub="Median, opened → closed">
          <ScalarStat result={m.issue_lifetime} />
        </MetricCard>
        <MetricCard title="Open issues" sub="Currently unresolved">
          <ScalarStat result={{ value: openTotal, unit: "issues" }} />
        </MetricCard>
        <MetricCard title="Aging > 1mo" sub="Open longer than a month">
          <ScalarStat result={{ value: stale, unit: "issues" }} />
        </MetricCard>
      </div>
      <MetricCard
        title="Open issue age"
        sub="How long open issues have been waiting"
        span
      >
        <BucketsBar result={m.open_issue_age} />
      </MetricCard>
      <MetricCard title="Latest issues" sub={`${issues.length} most recent`}>
        <LatestList kind="issues" items={issues} limit={10} />
      </MetricCard>
    </div>
  );
}

function PrsTab({ m, prs }: { m: D.MockMetricsMap; prs: D.MockPR[] }) {
  const merged = Math.round(
    m.pr_throughput.series.reduce((a, p) => a + p.value, 0),
  );
  return (
    <div
      className="fade-in"
      style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}
    >
      <MetricCard title="PR throughput" sub="Pull requests merged per day" span>
        <div style={{ marginTop: 10 }}>
          <BarSeries series={m.pr_throughput.series} unit="PRs" height={200} />
        </div>
      </MetricCard>
      <div className="stat-row">
        <MetricCard title="Time to merge" sub="Median, opened → merged">
          <ScalarStat result={m.time_to_merge} />
        </MetricCard>
        <MetricCard title="Review latency" sub="Median, opened → first review">
          <ScalarStat result={m.review_latency} />
        </MetricCard>
        <MetricCard title="Merged in window" sub="Total PRs merged">
          <ScalarStat
            result={{
              value: merged,
              unit: "PRs",
              count: m.time_to_merge.count,
            }}
          />
        </MetricCard>
      </div>
      <MetricCard title="Latest pull requests" sub={`${prs.length} most recent`}>
        <LatestList kind="prs" items={prs} limit={10} />
      </MetricCard>
    </div>
  );
}

function ContributorsTab({ m }: { m: D.MockMetricsMap }) {
  return (
    <div
      className="fade-in"
      style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}
    >
      <MetricCard
        title="Top contributors"
        sub={`${m.contributor_leaderboard.rows.length} people, ranked by commits in the window`}
        span
      >
        <div style={{ marginTop: 8 }}>
          <Leaderboard result={m.contributor_leaderboard} />
        </div>
      </MetricCard>
    </div>
  );
}

interface ReleaseHistoryItem {
  tag: string;
  date: string;
  latest: boolean;
  commits: number;
}

function ReleasesTab({
  repo,
  ov,
}: {
  repo: D.MockRepo;
  ov: D.MockOverview;
}) {
  const rels = useMemo(() => {
    const out: ReleaseHistoryItem[] = [];
    for (let i = 0; i < 6; i++) {
      const major = 2;
      const minor = ov.releases - i;
      out.push({
        tag: `v${major}.${Math.max(0, minor)}.0`,
        date: D.isoDaysAgo(i * 18 + 3),
        latest: i === 0,
        commits: 40 - i * 4,
      });
    }
    return out;
  }, [repo, ov]);

  return (
    <div
      className="fade-in"
      style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}
    >
      <div className="metric-grid">
        <MetricCard title="Total releases" sub="All tagged versions">
          <div className="scalar">
            <span className="v tnum">{ov.releases}</span>
          </div>
        </MetricCard>
        <MetricCard title="Cadence" sub="Average between releases">
          <div className="scalar">
            <span className="v tnum">18</span>
            <span className="u">days</span>
          </div>
        </MetricCard>
        <MetricCard title="Latest" sub="Most recent tag">
          <div className="scalar">
            <span className="v" style={{ fontSize: 24 }}>
              {rels[0]?.tag}
            </span>
          </div>
        </MetricCard>
      </div>
      <MetricCard title="Release history">
        <div className="latest" style={{ marginTop: 4 }}>
          {rels.map((r) => (
            <div className="item" key={r.tag}>
              <span className="ic">
                <I.tag style={{ width: 14, height: 14 }} />
              </span>
              <div className="body">
                <div className="ttl">
                  {r.tag}{" "}
                  {r.latest && (
                    <span className="badge green" style={{ marginLeft: 6 }}>
                      Latest
                    </span>
                  )}
                </div>
                <div className="sub">
                  <span>{r.commits} commits</span>
                  <span>·</span>
                  <span>{F.fmtRelative(r.date)}</span>
                </div>
              </div>
            </div>
          ))}
        </div>
      </MetricCard>
    </div>
  );
}
