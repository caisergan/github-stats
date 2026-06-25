import { useCallback, useEffect, useState } from "react";
import { I } from "../components/Icons";
import { SyncStatusBadge } from "../components/UI";
import { WindowControls, Kpi, MetricCard } from "../components/Components";
import {
  ContributionHeatmap,
  BarSeries,
  AreaSeries,
  ScalarStat,
  BucketsBar,
  Leaderboard,
} from "../components/Charts";
import LatestList from "../components/LatestList";
import LoadAllCommits from "../components/LoadAllCommits";
import RefreshButton from "../components/RefreshButton";
import UntrackButton from "../components/UntrackButton";
import { useAsync } from "../hooks/useAsync";
import { fetchMetrics, fetchOverview, fetchLatest, fetchCommits } from "../api";
import type {
  Repo,
  MetricsMap,
  Result,
  Overview as OverviewT,
  SeriesPoint,
  BucketRow,
  LeaderRow,
  LatestItem,
  LatestCommit,
  WindowSpec,
} from "../api";
import { seriesToHeatmap } from "../aggregate";
import * as F from "../format";

// Trailing-period phrase for the contribution heatmap, derived from the selected
// window. ("all" caps at a year because the calendar grid only spans 53 weeks.)
const WINDOW_PHRASE: Record<WindowSpec, string> = {
  "30d": "in the last 30 days",
  "90d": "in the last 90 days",
  "6m": "in the last 6 months",
  "1y": "in the last year",
  all: "in the last year",
};

const METRIC_KEYS = [
  "commit_rate",
  "pr_throughput",
  "code_churn",
  "comment_volume",
  "time_to_merge",
  "review_latency",
  "issue_lifetime",
  "open_issue_age",
  "contributor_leaderboard",
];

// --- narrowing helpers (the MetricsMap is a tagged union keyed by `kind`) ----
function tsSeries(r: Result | undefined): SeriesPoint[] {
  return r && r.kind === "time_series" ? r.series : [];
}
function scalar(r: Result | undefined): { value: number; unit?: string; count?: number } {
  if (r && r.kind === "scalar") return { value: r.value ?? 0, unit: r.unit, count: r.count };
  return { value: 0 };
}
function buckets(r: Result | undefined): BucketRow[] {
  return r && r.kind === "buckets" ? r.buckets : [];
}
function leaders(r: Result | undefined): LeaderRow[] {
  return r && r.kind === "leaderboard" ? r.rows : [];
}

const TABS = [
  { id: "insights", label: "Insights", icon: I.activity },
  { id: "commits", label: "Commits", icon: I.commit },
  { id: "issues", label: "Issues", icon: I.issue },
  { id: "prs", label: "Pull Requests", icon: I.pr },
  { id: "contributors", label: "Contributors", icon: I.users },
  { id: "releases", label: "Releases", icon: I.tag },
];

interface RepoDetailProps {
  repo: Repo;
  onBack: () => void;
  onUntrack: (id: number) => Promise<void>;
}

export default function RepoDetail({ repo, onBack, onUntrack }: RepoDetailProps) {
  const [tab, setTab] = useState("insights");
  const [win, setWin] = useState<WindowSpec>("90d");
  const [excludeBots, setExcludeBots] = useState(false);

  const { owner, name } = F.splitRepo(repo.full_name);

  const ov = useAsync<OverviewT>(
    () => fetchOverview(repo.id, { window: win, excludeBots }),
    [repo.id, win, excludeBots],
  );
  const metrics = useAsync<MetricsMap>(
    () => fetchMetrics(repo.id, { window: win, excludeBots, keys: METRIC_KEYS }),
    [repo.id, win, excludeBots],
  );
  const heat = useAsync<number[][]>(async () => {
    const m = await fetchMetrics(repo.id, { window: win, excludeBots, keys: ["commit_rate"] });
    return seriesToHeatmap(tsSeries(m.commit_rate));
  }, [repo.id, excludeBots, win]);
  const prs = useAsync<LatestItem[]>(() => fetchLatest(repo.id, "prs", 20), [repo.id]);
  const issues = useAsync<LatestItem[]>(() => fetchLatest(repo.id, "issues", 20), [repo.id]);

  // Commits use offset pagination (not useAsync) so "Load more" can append pages
  // while the badge/header show GitHub's true total.
  const COMMIT_PAGE = 30;
  const [commitItems, setCommitItems] = useState<LatestCommit[]>([]);
  const [commitStored, setCommitStored] = useState(0);
  const [commitTotal, setCommitTotal] = useState(0);
  const [commitLoading, setCommitLoading] = useState(false);

  const loadCommitPage = useCallback(
    async (offset: number) => {
      setCommitLoading(true);
      try {
        const page = await fetchCommits(repo.id, COMMIT_PAGE, offset);
        setCommitStored(page.stored);
        setCommitTotal(page.total);
        setCommitItems((prev) => (offset === 0 ? page.items : [...prev, ...page.items]));
      } finally {
        setCommitLoading(false);
      }
    },
    [repo.id],
  );

  useEffect(() => {
    loadCommitPage(0);
  }, [loadCommitPage]);

  const reloadAll = () => {
    ov.reload();
    metrics.reload();
    heat.reload();
    loadCommitPage(0);
    prs.reload();
    issues.reload();
  };

  const overview = ov.data;
  const m = metrics.data;

  return (
    <div className="page fade-in">
      <div className="breadcrumb">
        <a onClick={onBack}>
          <I.chevLeft style={{ width: 14, height: 14 }} />
        </a>
        <a onClick={onBack}>Repositories</a>
        <span className="sep" style={{ color: "var(--faint)" }}>–</span>
        <span style={{ color: "var(--fg-2)" }}>
          {owner}/{name}
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
              <span className="owner">{owner}</span>
              <span className="slash">/</span>
              {name}
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
                {F.fmtNumber(repo.stargazers ?? 0)}
              </span>
              <span className="m">
                <I.fork style={{ width: 14, height: 14 }} />
                {F.fmtNumber(repo.forks ?? 0)}
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
          <RefreshButton repoID={repo.id} onComplete={reloadAll} />
          <UntrackButton
            repoID={repo.id}
            repoName={repo.full_name}
            onUntrack={onUntrack}
            onDone={onBack}
          />
        </div>
      </div>

      <WindowControls
        win={win}
        excludeBots={excludeBots}
        onWin={(w) => setWin(w as WindowSpec)}
        onBots={setExcludeBots}
      />

      {!overview || !m ? (
        <div className="empty" style={{ padding: 48, textAlign: "center" }}>
          Loading…
        </div>
      ) : (
        <>
          {/* Details KPI strip (always visible) */}
          <div className="kpi-strip" style={{ marginTop: 8 }}>
            <Kpi
              icon={I.commit}
              label="Commit rate"
              value={F.fmtRate(overview.commit_rate).replace("/day", "")}
              unit="/day"
              delta={9}
            />
            <Kpi icon={I.pr} label="Open PRs" value={overview.open_prs} delta={-3} />
            <Kpi icon={I.issue} label="Open issues" value={overview.open_issues} delta={5} />
            <Kpi icon={I.users} label="Contributors" value={overview.contributors} delta={2} />
            <Kpi icon={I.tag} label="Releases" value={overview.releases} delta={0} />
          </div>

          <div className="tabs">
            {TABS.map((t) => {
              const count: Record<string, number> = {
                commits: commitTotal,
                issues: overview.open_issues,
                prs: overview.open_prs,
                contributors: overview.contributors,
                releases: overview.releases,
              };
              return (
                <button
                  key={t.id}
                  className={tab === t.id ? "active" : ""}
                  onClick={() => setTab(t.id)}
                >
                  <t.icon style={{ width: 15, height: 15 }} />
                  {t.label}
                  {count[t.id] != null && <span className="count">{count[t.id]}</span>}
                </button>
              );
            })}
          </div>

          {tab === "insights" && (
            <InsightsTab m={m} heat={heat.data ?? []} heatLabel={WINDOW_PHRASE[win]} />
          )}
          {tab === "commits" && (
            <CommitsTab
              m={m}
              repoID={repo.id}
              repoFullName={repo.full_name}
              items={commitItems}
              stored={commitStored}
              total={commitTotal}
              loading={commitLoading}
              onLoadMore={() => loadCommitPage(commitItems.length)}
              onReload={() => loadCommitPage(0)}
            />
          )}
          {tab === "issues" && (
            <IssuesTab m={m} issues={issues.data ?? []} repoFullName={repo.full_name} />
          )}
          {tab === "prs" && (
            <PrsTab m={m} prs={prs.data ?? []} repoFullName={repo.full_name} />
          )}
          {tab === "contributors" && <ContributorsTab m={m} />}
          {tab === "releases" && <ReleasesTab ov={overview} />}
        </>
      )}
    </div>
  );
}

/* ---- tabs ---- */
function InsightsTab({
  m,
  heat,
  heatLabel,
}: {
  m: MetricsMap;
  heat: number[][];
  heatLabel: string;
}) {
  return (
    <div
      className="fade-in"
      style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}
    >
      <MetricCard title="Contribution activity" sub={`Daily commits ${heatLabel}`} span>
        <div style={{ marginTop: 10 }}>
          <ContributionHeatmap weeks={heat} label={heatLabel} />
        </div>
      </MetricCard>
      <div className="metric-grid two">
        <MetricCard title="Commits per day" sub="Commit throughput over the window">
          <div style={{ marginTop: 10 }}>
            <BarSeries series={tsSeries(m.commit_rate)} unit="commits" />
          </div>
        </MetricCard>
        <MetricCard title="Code churn" sub="Lines added + removed per day">
          <div style={{ marginTop: 10 }}>
            <AreaSeries series={tsSeries(m.code_churn)} unit="lines" />
          </div>
        </MetricCard>
      </div>
      <div className="metric-grid">
        <MetricCard title="Time to merge" sub="Median, opened → merged">
          <ScalarStat result={scalar(m.time_to_merge)} />
        </MetricCard>
        <MetricCard title="Review latency" sub="Median, opened → first review">
          <ScalarStat result={scalar(m.review_latency)} />
        </MetricCard>
        <MetricCard title="Issue lifetime" sub="Median, opened → closed">
          <ScalarStat result={scalar(m.issue_lifetime)} />
        </MetricCard>
        <MetricCard title="Open issue age" sub="Distribution of open issues">
          <BucketsBar result={{ buckets: buckets(m.open_issue_age) }} />
        </MetricCard>
        <MetricCard title="PR throughput" sub="PRs merged per day">
          <div style={{ marginTop: 4 }}>
            <BarSeries series={tsSeries(m.pr_throughput)} unit="PRs" height={150} />
          </div>
        </MetricCard>
        <MetricCard title="Comment volume" sub="Comments per day">
          <div style={{ marginTop: 4 }}>
            <AreaSeries series={tsSeries(m.comment_volume)} unit="comments" height={150} />
          </div>
        </MetricCard>
      </div>
    </div>
  );
}

function CommitsTab({
  m,
  repoID,
  repoFullName,
  items,
  stored,
  total,
  loading,
  onLoadMore,
  onReload,
}: {
  m: MetricsMap;
  repoID: number;
  repoFullName: string;
  items: LatestCommit[];
  stored: number;
  total: number;
  loading: boolean;
  onLoadMore: () => void;
  onReload: () => void;
}) {
  const canLoadMore = items.length < stored;
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
          <BarSeries series={tsSeries(m.commit_rate)} unit="commits" height={220} />
        </div>
      </MetricCard>
      <MetricCard title="Latest commits" sub={`${stored} of ${total} most recent`}>
        <LatestList kind="commits" items={items} repoFullName={repoFullName} />
        <div className="commits-foot">
          <div className="lm">
            {canLoadMore && (
              <button className="btn ghost sm" onClick={onLoadMore} disabled={loading}>
                {loading ? "Loading…" : "Load more"}
              </button>
            )}
            <span className="muted-note">
              Showing {items.length} of {stored} loaded
              {total > stored ? ` · ${total - stored} more on GitHub` : ""}
            </span>
          </div>
          <LoadAllCommits repoID={repoID} onComplete={onReload} />
        </div>
      </MetricCard>
    </div>
  );
}

function IssuesTab({
  m,
  issues,
  repoFullName,
}: {
  m: MetricsMap;
  issues: LatestItem[];
  repoFullName: string;
}) {
  const bks = buckets(m.open_issue_age);
  const openTotal = bks.reduce((a, b) => a + b.count, 0);
  const stale = bks
    .filter((b) => /90d|180d|older/.test(b.label))
    .reduce((a, b) => a + b.count, 0);
  return (
    <div
      className="fade-in"
      style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}
    >
      <div className="stat-row">
        <MetricCard title="Issue lifetime" sub="Median, opened → closed">
          <ScalarStat result={scalar(m.issue_lifetime)} />
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
        <BucketsBar result={{ buckets: bks }} />
      </MetricCard>
      <MetricCard title="Latest issues" sub={`${issues.length} most recent`}>
        <LatestList kind="issues" items={issues} repoFullName={repoFullName} />
      </MetricCard>
    </div>
  );
}

function PrsTab({
  m,
  prs,
  repoFullName,
}: {
  m: MetricsMap;
  prs: LatestItem[];
  repoFullName: string;
}) {
  const merged = Math.round(tsSeries(m.pr_throughput).reduce((a, p) => a + p.value, 0));
  const ttm = scalar(m.time_to_merge);
  return (
    <div
      className="fade-in"
      style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}
    >
      <MetricCard title="PR throughput" sub="Pull requests merged per day" span>
        <div style={{ marginTop: 10 }}>
          <BarSeries series={tsSeries(m.pr_throughput)} unit="PRs" height={200} />
        </div>
      </MetricCard>
      <div className="stat-row">
        <MetricCard title="Time to merge" sub="Median, opened → merged">
          <ScalarStat result={scalar(m.time_to_merge)} />
        </MetricCard>
        <MetricCard title="Review latency" sub="Median, opened → first review">
          <ScalarStat result={scalar(m.review_latency)} />
        </MetricCard>
        <MetricCard title="Merged in window" sub="Total PRs merged">
          <ScalarStat result={{ value: merged, unit: "PRs", count: ttm.count }} />
        </MetricCard>
      </div>
      <MetricCard title="Latest pull requests" sub={`${prs.length} most recent`}>
        <LatestList kind="prs" items={prs} repoFullName={repoFullName} />
      </MetricCard>
    </div>
  );
}

function ContributorsTab({ m }: { m: MetricsMap }) {
  const rows = leaders(m.contributor_leaderboard);
  return (
    <div
      className="fade-in"
      style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}
    >
      <MetricCard
        title="Top contributors"
        sub={`${rows.length} people, ranked by commits in the window`}
        span
      >
        <div style={{ marginTop: 8 }}>
          <Leaderboard
            result={{ rows: rows.map((r) => ({ ...r, img: F.avatarURL(r.login) })) }}
          />
        </div>
      </MetricCard>
    </div>
  );
}

function ReleasesTab({ ov }: { ov: OverviewT }) {
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
      </div>
      <MetricCard title="Release history">
        <p className="metric-note">{ov.releases} releases total.</p>
        <p className="empty">Individual release history isn’t available yet.</p>
      </MetricCard>
    </div>
  );
}
