import React, { useState, useMemo } from "react";
import { I } from "../components/Icons";
import { Sparkline } from "../components/Charts";
import { RepoCard } from "../components/Components";
import * as D from "../data";

const COL_ICONS: Record<string, React.ComponentType<any>> = {
  server: I.server,
  layout: I.layout,
  globe: I.globe,
  folder: I.folder,
  layers: I.layers,
};

interface CollectionCardProps {
  col: D.MockCollection;
  repos: D.MockRepo[];
  onOpen: (col: D.MockCollection) => void;
}

function CollectionCard({ col, repos, onOpen }: CollectionCardProps) {
  const members = repos.filter((r) => col.repoIds.includes(r.id));
  const Ic = COL_ICONS[col.emoji] || I.folder;
  const commits = Math.round(
    members.reduce((a, r) => a + r.commit_rate, 0) * 10,
  ) / 10;
  const contributors = members.reduce((a, r) => a + r.contributors, 0);
  const openPrs = members.reduce((a, r) => a + r.open_prs, 0);
  const spark = useMemo(() => {
    return D.aggregateSeries(members, "commit_rate", 90);
  }, [col, members]);

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
            <Ic style={{ width: 17, height: 17 }} />
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
      <div className="rc-desc" style={{ WebkitLineClamp: 1 }}>
        {col.desc}
      </div>
      <div className="rc-spark">
        <Sparkline series={spark} />
      </div>
      <div className="rc-stats">
        <span className="rc-stat">
          <I.commit style={{ width: 14, height: 14 }} />
          <b>{commits}</b>
          <span className="muted">/d</span>
        </span>
        <span className="rc-stat">
          <I.users style={{ width: 14, height: 14 }} />
          <b>{contributors}</b>
        </span>
        <span className="rc-stat">
          <I.pr style={{ width: 14, height: 14 }} />
          <b>{openPrs}</b>
        </span>
      </div>
      <div className="rc-foot" style={{ borderTop: "1px solid var(--border)" }}>
        <div className="row" style={{ gap: 4 }}>
          {members.slice(0, 4).map((r) => (
            <span key={r.id} className="lang" style={{ gap: 4 }}>
              <span className="d" style={{ background: r.langColor }} />
            </span>
          ))}
          <span className="muted" style={{ fontSize: 12 }}>
            {[...new Set(members.map((r) => r.lang))]
              .slice(0, 3)
              .join(" · ")}
          </span>
        </div>
      </div>
    </div>
  );
}

interface NewCollectionModalProps {
  repos: D.MockRepo[];
  onClose: () => void;
  onCreate: (col: D.MockCollection) => void;
}

function NewCollectionModal({
  repos,
  onClose,
  onCreate,
}: NewCollectionModalProps) {
  const [name, setName] = useState("");
  const [picked, setPicked] = useState<number[]>([]);
  const toggle = (id: number) =>
    setPicked((p) => (p.includes(id) ? p.filter((x) => x !== id) : [...p, id]));

  const create = () => {
    if (!name.trim() || picked.length === 0) return;
    onCreate({
      id: "c" + Date.now(),
      name: name.trim(),
      desc: `${picked.length} repositories`,
      emoji: "folder",
      repoIds: picked,
    });
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
          Group repositories to track them together.
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
        <label
          className="eyebrow"
          style={{ display: "block", margin: "18px 0 9px" }}
        >
          Repositories ({picked.length} selected)
        </label>
        <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
          {repos.map((r) => {
            const on = picked.includes(r.id);
            return (
              <div
                key={r.id}
                onClick={() => toggle(r.id)}
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: 10,
                  padding: "9px 11px",
                  borderRadius: "var(--radius-sm)",
                  cursor: "pointer",
                  border: "1px solid " + (on ? "var(--accent)" : "var(--border)"),
                  background: on ? "var(--accent-weak)" : "var(--surface)",
                }}
              >
                <span
                  style={{
                    width: 17,
                    height: 17,
                    borderRadius: 5,
                    border:
                      "1.5px solid " +
                      (on ? "var(--accent)" : "var(--border-strong)"),
                    background: on ? "var(--accent)" : "transparent",
                    display: "grid",
                    placeItems: "center",
                    color: "var(--accent-fg)",
                    flex: "none",
                  }}
                >
                  {on && <I.check style={{ width: 12, height: 12 }} />}
                </span>
                <span className="lang">
                  <span className="d" style={{ background: r.langColor }} />
                </span>
                <span style={{ fontSize: 13.5, fontWeight: 500 }}>
                  <span className="muted">{r.owner}/</span>
                  {r.name}
                </span>
              </div>
            );
          })}
        </div>
        <div
          className="row"
          style={{ justifyContent: "flex-end", gap: 9, marginTop: 20 }}
        >
          <button className="btn" onClick={onClose}>
            Cancel
          </button>
          <button
            className="btn primary"
            onClick={create}
            disabled={!name.trim() || picked.length === 0}
          >
            <I.plus style={{ width: 15, height: 15 }} />
            Create collection
          </button>
        </div>
      </div>
    </div>
  );
}

interface CollectionsProps {
  repos: D.MockRepo[];
  collections: D.MockCollection[];
  onOpenRepo: (repo: D.MockRepo) => void;
  onCreate: (col: D.MockCollection) => void;
}

export default function Collections({
  repos,
  collections,
  onOpenRepo,
  onCreate,
}: CollectionsProps) {
  const [selected, setSelected] = useState<D.MockCollection | null>(null);
  const [modal, setModal] = useState(false);

  if (selected) {
    const members = repos.filter((r) => selected.repoIds.includes(r.id));
    const sparks: Record<number, D.MockMetricsMap> = {};
    members.forEach((r) => {
      sparks[r.id] = D.makeMetrics(r.seed, 90);
    });
    const Ic = COL_ICONS[selected.emoji] || I.folder;

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
              <Ic style={{ width: 19, height: 19 }} />
            </span>
            <div>
              <h1>{selected.name}</h1>
              <div className="meta">
                <span className="m">{members.length} repositories</span>
                <span className="m">{selected.desc}</span>
              </div>
            </div>
          </div>
        </div>
        <div className="repo-grid" style={{ marginTop: 8 }}>
          {members.map((r) => (
            <RepoCard
              key={r.id}
              repo={r}
              metrics={sparks[r.id]}
              onOpen={onOpenRepo}
            />
          ))}
        </div>
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
      <div className="repo-grid">
        {collections.map((c) => (
          <CollectionCard
            key={c.id}
            col={c}
            repos={repos}
            onOpen={setSelected}
          />
        ))}
      </div>
      {modal && (
        <NewCollectionModal
          repos={repos}
          onClose={() => setModal(false)}
          onCreate={onCreate}
        />
      )}
    </div>
  );
}
