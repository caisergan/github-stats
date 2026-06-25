import { useState } from "react";
import { I } from "../components/Icons";
import { Sparkline } from "../components/Charts";
import RepoCard from "../components/RepoCard";
import { useAsync } from "../hooks/useAsync";
import { fetchMetrics, fetchOverview } from "../api";
import type {
  Repo,
  Collection,
  MetricsMap,
  SeriesPoint,
  Overview as OverviewT,
} from "../api";
import { sumSeries } from "../aggregate";
import * as F from "../format";

function seriesOf(m: MetricsMap, key: string): SeriesPoint[] {
  const r = m[key];
  return r && r.kind === "time_series" ? r.series : [];
}

interface CollectionCardProps {
  col: Collection;
  repos: Repo[];
  onOpen: (col: Collection) => void;
}

function CollectionCard({ col, repos, onOpen }: CollectionCardProps) {
  const members = repos.filter((r) => col.repo_ids.includes(r.id));
  const memberIds = members.map((r) => r.id);
  const key = memberIds.join(",");

  const state = useAsync(async () => {
    if (memberIds.length === 0) {
      return { spark: [] as SeriesPoint[], commits: 0, contributors: 0, openPrs: 0 };
    }
    const metricsList = await Promise.all(
      memberIds.map((id) =>
        fetchMetrics(id, { window: "90d", excludeBots: false, keys: ["commit_rate"] }),
      ),
    );
    const overviews = await Promise.all(
      memberIds.map((id) => fetchOverview(id, { window: "90d", excludeBots: false })),
    );
    return {
      spark: sumSeries(metricsList.map((m) => seriesOf(m, "commit_rate"))),
      commits: Math.round(overviews.reduce((a, o) => a + o.commit_rate, 0) * 10) / 10,
      contributors: overviews.reduce((a, o) => a + o.contributors, 0),
      openPrs: overviews.reduce((a, o) => a + o.open_prs, 0),
    };
  }, [key]);

  const d = state.data ?? { spark: [], commits: 0, contributors: 0, openPrs: 0 };

  return (
    <div className="card hover repo-card fade-in" onClick={() => onOpen(col)}>
      <div className="rc-top">
        <div className="row" style={{ gap: 11 }}>
          <span
            className="logo"
            style={{
              width: 34,
              height: 34,
              borderRadius: 9,
              background: "var(--surface-2)",
              color: "var(--fg-2)",
              display: "grid",
              placeItems: "center",
              border: "1px solid var(--border)",
            }}
          >
            <I.folder style={{ width: 17, height: 17 }} />
          </span>
          <div>
            <div className="rc-name" style={{ fontSize: 15 }}>
              {col.name}
            </div>
            <div className="muted" style={{ fontSize: 12.5 }}>
              {members.length} repositories
            </div>
          </div>
        </div>
        <I.chevRight style={{ width: 16, height: 16, color: "var(--faint)" }} />
      </div>
      <div className="rc-spark">
        <Sparkline series={d.spark} />
      </div>
      <div className="rc-stats">
        <span className="rc-stat">
          <I.commit style={{ width: 14, height: 14 }} />
          <b>{d.commits}</b>
          <span className="muted">/d</span>
        </span>
        <span className="rc-stat">
          <I.users style={{ width: 14, height: 14 }} />
          <b>{d.contributors}</b>
        </span>
        <span className="rc-stat">
          <I.pr style={{ width: 14, height: 14 }} />
          <b>{d.openPrs}</b>
        </span>
      </div>
      <div className="rc-foot" style={{ borderTop: "1px solid var(--border)" }}>
        <div className="row" style={{ gap: 4 }}>
          <span className="muted" style={{ fontSize: 12 }}>
            {members
              .slice(0, 3)
              .map((r) => F.splitRepo(r.full_name).name)
              .join(" · ") || "No repositories yet"}
          </span>
        </div>
      </div>
    </div>
  );
}

interface NewCollectionModalProps {
  onClose: () => void;
  onCreate: (name: string) => void;
}

function NewCollectionModal({ onClose, onCreate }: NewCollectionModalProps) {
  const [name, setName] = useState("");

  const create = () => {
    if (!name.trim()) return;
    onCreate(name.trim());
    onClose();
  };

  return (
    <div
      style={{
        position: "fixed",
        inset: 0,
        zIndex: 80,
        display: "grid",
        placeItems: "center",
        background: "rgba(0,0,0,.45)",
      }}
      onClick={onClose}
    >
      <div
        className="card pad fade-in"
        style={{
          width: "min(520px, 92vw)",
          maxHeight: "84vh",
          overflow: "auto",
          background: "var(--surface)",
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <h2
          style={{
            margin: "0 0 4px",
            fontSize: 18,
            fontWeight: 650,
            letterSpacing: "-0.01em",
          }}
        >
          New collection
        </h2>
        <p className="muted" style={{ margin: "0 0 18px", fontSize: 13.5 }}>
          Group repositories to track them together. Add repositories after creating it.
        </p>
        <label className="eyebrow" style={{ display: "block", marginBottom: 7 }}>
          Name
        </label>
        <input
          className="input"
          placeholder="e.g. Platform team"
          value={name}
          onChange={(e) => setName(e.target.value)}
          autoFocus
        />
        <div
          className="row"
          style={{ justifyContent: "flex-end", gap: 9, marginTop: 20 }}
        >
          <button className="btn" onClick={onClose}>
            Cancel
          </button>
          <button className="btn primary" onClick={create} disabled={!name.trim()}>
            <I.plus style={{ width: 15, height: 15 }} />
            Create collection
          </button>
        </div>
      </div>
    </div>
  );
}

interface CollectionsProps {
  repos: Repo[];
  collections: Collection[];
  onOpenRepo: (repo: Repo) => void;
  onCreate: (name: string) => void;
}

export default function Collections({
  repos,
  collections,
  onOpenRepo,
  onCreate,
}: CollectionsProps) {
  const [selected, setSelected] = useState<Collection | null>(null);
  const [modal, setModal] = useState(false);

  // Fetch overviews for the selected collection's members (drives RepoCard).
  const memberIds = selected
    ? repos.filter((r) => selected.repo_ids.includes(r.id)).map((r) => r.id)
    : [];
  const memberKey = memberIds.join(",");
  const ovState = useAsync<Record<number, OverviewT>>(async () => {
    if (memberIds.length === 0) return {};
    const list = await Promise.all(
      memberIds.map((id) => fetchOverview(id, { window: "90d", excludeBots: false })),
    );
    return Object.fromEntries(list.map((o) => [o.id, o]));
  }, [memberKey]);
  const overviews = ovState.data ?? {};

  if (selected) {
    const members = repos.filter((r) => selected.repo_ids.includes(r.id));

    return (
      <div className="page fade-in">
        <div className="breadcrumb">
          <a onClick={() => setSelected(null)}>
            <I.chevLeft style={{ width: 14, height: 14 }} />
          </a>
          <a onClick={() => setSelected(null)}>Collections</a>
          <I.chevRight style={{ width: 12, height: 12, color: "var(--faint)" }} />
          <span style={{ color: "var(--fg-2)" }}>{selected.name}</span>
        </div>
        <div className="detail-head">
          <div className="title">
            <span
              className="logo"
              style={{
                width: 38,
                height: 38,
                borderRadius: 9,
                background: "var(--surface-2)",
                color: "var(--fg-2)",
                display: "grid",
                placeItems: "center",
                border: "1px solid var(--border)",
              }}
            >
              <I.folder style={{ width: 19, height: 19 }} />
            </span>
            <div>
              <h1>{selected.name}</h1>
              <div className="meta">
                <span className="m">{members.length} repositories</span>
              </div>
            </div>
          </div>
        </div>
        {members.length === 0 ? (
          <div className="card pad" style={{ textAlign: "center", padding: 56 }}>
            <div style={{ color: "var(--muted)" }}>
              No repositories in this collection yet.
            </div>
          </div>
        ) : (
          <div className="repo-grid" style={{ marginTop: 8 }}>
            {members.map((r) => (
              <RepoCard key={r.id} repo={r} overview={overviews[r.id] ?? null} />
            ))}
          </div>
        )}
      </div>
    );
  }

  return (
    <div className="page fade-in">
      <div className="page-head">
        <div>
          <h1>Collections</h1>
          <div className="sub">
            Group repositories into named sets to track them together
          </div>
        </div>
        <button className="btn primary" onClick={() => setModal(true)}>
          <I.plus style={{ width: 15, height: 15 }} />
          New collection
        </button>
      </div>
      {collections.length === 0 ? (
        <div className="card pad" style={{ textAlign: "center", padding: 56 }}>
          <div style={{ color: "var(--muted)" }}>
            No collections yet — create one to group repositories.
          </div>
        </div>
      ) : (
        <div className="repo-grid">
          {collections.map((c) => (
            <CollectionCard key={c.id} col={c} repos={repos} onOpen={setSelected} />
          ))}
        </div>
      )}
      {modal && (
        <NewCollectionModal onClose={() => setModal(false)} onCreate={onCreate} />
      )}
    </div>
  );
}
