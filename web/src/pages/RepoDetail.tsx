import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import {
  fetchOverview,
  fetchMetrics,
  fetchLatest,
  type Result,
  type WindowSpec,
  type LatestItem,
} from "../api";
import { useRepos } from "../hooks/useRepos";
import { useAsync } from "../hooks/useAsync";
import MetricView from "../components/MetricView";
import WindowControls from "../components/WindowControls";
import RefreshButton from "../components/RefreshButton";
import LatestList from "../components/LatestList";
import SyncStatusBadge from "../components/SyncStatusBadge";
import { fmtNullableTs, fmtNumber, fmtRate } from "../format";

const INSIGHT_KEYS = [
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

function MetricCard({ title, result }: { title: string; result: Result | undefined }) {
  return (
    <div className="metric-card">
      <h3>{title}</h3>
      {result ? <MetricView result={result} /> : <p className="empty">No data.</p>}
    </div>
  );
}

export default function RepoDetail() {
  const { owner = "", repo = "" } = useParams();
  const { loading: reposLoading, resolve, add } = useRepos();
  const [windowSpec, setWindowSpec] = useState<WindowSpec>("30d");
  const [excludeBots, setExcludeBots] = useState(false);

  const tracked = resolve(owner, repo);
  const repoID = tracked?.id ?? 0;

  const overview = useAsync(
    () => fetchOverview(repoID, { window: windowSpec, excludeBots }),
    [repoID, windowSpec, excludeBots],
  );
  const metrics = useAsync(
    () => fetchMetrics(repoID, { keys: INSIGHT_KEYS, window: windowSpec, excludeBots }),
    [repoID, windowSpec, excludeBots],
  );
  const commits = useAsync<LatestItem[]>(() => fetchLatest(repoID, "commits", 20), [repoID]);
  const prs = useAsync<LatestItem[]>(() => fetchLatest(repoID, "prs", 20), [repoID]);
  const issues = useAsync<LatestItem[]>(() => fetchLatest(repoID, "issues", 20), [repoID]);

  function reloadAll() {
    overview.reload();
    metrics.reload();
    commits.reload();
    prs.reload();
    issues.reload();
  }

  if (reposLoading) {
    return <div className="app-shell"><p className="state">Loading…</p></div>;
  }

  if (!tracked) {
    return (
      <div className="app-shell">
        <p><Link to="/">← All repositories</Link></p>
        <div className="notice">
          <p><strong>{owner}/{repo}</strong> is not tracked yet.</p>
          <button
            className="primary"
            onClick={() => add(`${owner}/${repo}`)}
          >
            Track this repo
          </button>
        </div>
      </div>
    );
  }

  const ov = overview.data;
  const m = metrics.data ?? {};

  return (
    <div className="app-shell">
      <p><Link to="/">← All repositories</Link></p>

      <div className="detail-head">
        <div>
          <h1>{tracked.full_name}</h1>
          <div className="sub">
            {tracked.is_private ? "Private" : "Public"} · {tracked.default_branch} ·
            {" "}synced {fmtNullableTs(tracked.last_synced_at)}
          </div>
        </div>
        <div className="refresh-controls">
          <SyncStatusBadge status={tracked.sync_status} />
          <RefreshButton repoID={repoID} onComplete={reloadAll} />
        </div>
      </div>

      <WindowControls
        window={windowSpec}
        excludeBots={excludeBots}
        onWindow={setWindowSpec}
        onExcludeBots={setExcludeBots}
      />

      {metrics.error && (
        <p className="state error">Failed to load metrics: {metrics.error.message}</p>
      )}

      {/* Details */}
      <section className="section">
        <h2>Details</h2>
        <div className="metric-grid">
          <div className="metric-card"><h3>Open issues</h3><div className="scalar"><span className="value">{ov ? fmtNumber(ov.open_issues) : "—"}</span></div></div>
          <div className="metric-card"><h3>Open PRs</h3><div className="scalar"><span className="value">{ov ? fmtNumber(ov.open_prs) : "—"}</span></div></div>
          <div className="metric-card"><h3>Contributors</h3><div className="scalar"><span className="value">{ov ? fmtNumber(ov.contributors) : "—"}</span></div></div>
          <div className="metric-card"><h3>Commit rate</h3><div className="scalar"><span className="value">{ov ? fmtRate(ov.commit_rate) : "—"}</span></div></div>
        </div>
      </section>

      {/* Insights */}
      <section className="section">
        <h2>Insights</h2>
        <div className="metric-grid">
          <MetricCard title="Commit rate" result={m.commit_rate} />
          <MetricCard title="PR throughput" result={m.pr_throughput} />
          <MetricCard title="Code churn" result={m.code_churn} />
          <MetricCard title="Comment volume" result={m.comment_volume} />
        </div>
      </section>

      {/* Commits */}
      <section className="section">
        <h2>Commits</h2>
        <div className="metric-grid">
          <MetricCard title="Commits per day" result={m.commit_rate} />
          <div className="metric-card">
            <h3>Latest commits</h3>
            <LatestList kind="commits" items={commits.data ?? []} />
          </div>
        </div>
      </section>

      {/* Issues */}
      <section className="section">
        <h2>Issues</h2>
        <div className="metric-grid">
          <MetricCard title="Issue lifetime" result={m.issue_lifetime} />
          <MetricCard title="Open issue age" result={m.open_issue_age} />
          <div className="metric-card">
            <h3>Latest issues</h3>
            <LatestList kind="issues" items={issues.data ?? []} />
          </div>
        </div>
      </section>

      {/* PRs */}
      <section className="section">
        <h2>Pull requests</h2>
        <div className="metric-grid">
          <MetricCard title="PR throughput" result={m.pr_throughput} />
          <MetricCard title="Time to merge" result={m.time_to_merge} />
          <MetricCard title="Review latency" result={m.review_latency} />
          <div className="metric-card">
            <h3>Latest PRs</h3>
            <LatestList kind="prs" items={prs.data ?? []} />
          </div>
        </div>
      </section>

      {/* Contributors */}
      <section className="section">
        <h2>Contributors</h2>
        <div className="metric-grid">
          <MetricCard title="Leaderboard" result={m.contributor_leaderboard} />
        </div>
      </section>

      {/* Releases */}
      <section className="section">
        <h2>Releases</h2>
        <div className="metric-grid">
          <div className="metric-card">
            <h3>Total releases</h3>
            <div className="scalar"><span className="value">{ov ? fmtNumber(ov.releases) : "—"}</span></div>
          </div>
        </div>
      </section>
    </div>
  );
}
