import React, { useState, useMemo } from "react";
import { I } from "../components/Icons";
import { Select } from "../components/UI";
import { Kpi, RepoCard, AddRepoForm } from "../components/Components";
import * as D from "../data";
import * as F from "../format";

interface OverviewProps {
  repos: D.MockRepo[];
  onOpen: (repo: D.MockRepo) => void;
  onAdd: (fullName: string) => void;
}

export default function Overview({ repos, onOpen, onAdd }: OverviewProps) {
  const [q, setQ] = useState("");
  const [sort, setSort] = useState("activity");
  const [status, setStatus] = useState("all");

  // per-repo light metrics for sparklines
  const sparks = useMemo(() => {
    const m: Record<number, D.MockMetricsMap> = {};
    repos.forEach((r) => {
      m[r.id] = D.makeMetrics(r.seed, 90);
    });
    return m;
  }, [repos]);

  const filtered = useMemo(() => {
    let list = repos.filter(
      (r) =>
        r.full_name.toLowerCase().includes(q.toLowerCase()) ||
        (r.description || "").toLowerCase().includes(q.toLowerCase()),
    );
    if (status !== "all") {
      list = list.filter((r) => r.sync_status === status);
    }
    const by: Record<string, (a: D.MockRepo, b: D.MockRepo) => number> = {
      activity: (a, b) => b.commit_rate - a.commit_rate,
      stars: (a, b) => b.stargazers - a.stargazers,
      name: (a, b) => a.full_name.localeCompare(b.full_name),
      issues: (a, b) => b.open_issues - a.open_issues,
    };
    return [...list].sort(by[sort]);
  }, [repos, q, sort, status]);

  // aggregate KPIs
  const agg = useMemo(() => {
    const commits = repos.reduce((a, r) => a + r.commit_rate, 0);
    const contributors = repos.reduce((a, r) => a + r.contributors, 0);
    const openPrs = repos.reduce((a, r) => a + r.open_prs, 0);
    const openIssues = repos.reduce((a, r) => a + r.open_issues, 0);
    return {
      commits: Math.round(commits * 10) / 10,
      contributors,
      openPrs,
      openIssues,
    };
  }, [repos]);

  return (
    <div className="page fade-in">
      <div className="page-head">
        <div>
          <h1>Repositories</h1>
          <div className="sub">
            Tracking {repos.length} repositories across your workspace
          </div>
        </div>
        <AddRepoForm onAdd={onAdd} />
      </div>

      <div className="kpi-strip">
        <Kpi
          icon={I.repo}
          label="Tracked repos"
          value={repos.length}
          delta={0}
        />
        <Kpi
          icon={I.commit}
          label="Commits / day"
          value={agg.commits}
          delta={12}
        />
        <Kpi
          icon={I.users}
          label="Contributors"
          value={F.fmtNumber(agg.contributors)}
          delta={4}
        />
        <I.pr style={{ display: "none" }} />
        <Kpi icon={I.pr} label="Open PRs" value={agg.openPrs} delta={-8} />
        <Kpi
          icon={I.issue}
          label="Open issues"
          value={agg.openIssues}
          delta={6}
        />
      </div>

      <div className="toolbar">
        <span className="field-icon search">
          <I.search style={{ width: 15, height: 15 }} />
          <input
            className="input"
            placeholder="Filter repositories…"
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </span>
        <span className="row" style={{ gap: 8 }}>
          <span className="ctl-label">Status</span>
          <Select
            value={status}
            onChange={setStatus}
            options={[
              { value: "all", label: "All statuses" },
              { value: "complete", label: "Synced" },
              { value: "running", label: "Syncing" },
              { value: "pending", label: "Queued" },
            ]}
          />
        </span>
        <span className="row" style={{ gap: 8 }}>
          <span className="ctl-label">Sort</span>
          <Select
            value={sort}
            onChange={setSort}
            options={[
              { value: "activity", label: "Most active" },
              { value: "stars", label: "Most stars" },
              { value: "issues", label: "Most open issues" },
              { value: "name", label: "Name (A–Z)" },
            ]}
          />
        </span>
      </div>

      {filtered.length === 0 ? (
        <div className="card pad" style={{ textAlign: "center", padding: 56 }}>
          <div style={{ color: "var(--muted)" }}>
            No repositories match “{q}”.
          </div>
        </div>
      ) : (
        <div className="repo-grid">
          {filtered.map((r) => (
            <RepoCard
              key={r.id}
              repo={r}
              metrics={sparks[r.id]}
              onOpen={onOpen}
            />
          ))}
        </div>
      )}
    </div>
  );
}
