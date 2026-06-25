import React, { useState, useRef, useEffect } from "react";
import { I } from "./Icons";
import { Sparkline } from "./Charts";
import { Segmented, Switch, SyncStatusBadge, LangDot } from "./UI";
import * as F from "../format";
import * as D from "../data";

interface KpiProps {
  icon?: React.ComponentType<any>;
  label: string;
  value: string | number;
  unit?: string;
  delta?: number;
}

export function Kpi({ icon: IconComp, label, value, unit, delta }: KpiProps) {
  return (
    <div className="card kpi">
      <div className="label">
        {IconComp && (
          <span className="ic">
            <IconComp style={{ width: 15, height: 15 }} />
          </span>
        )}
        <span className="eyebrow">{label}</span>
      </div>
      <div className="val tnum">
        {value}
        {unit && <small>{unit}</small>}
      </div>
      {delta != null && delta !== 0 && (
        <div
          className={"delta " + (delta > 0 ? "up" : delta < 0 ? "down" : "flat")}
        >
          {delta > 0 ? (
            <I.arrowUp style={{ width: 14, height: 14 }} />
          ) : delta < 0 ? (
            <I.arrowDown style={{ width: 14, height: 14 }} />
          ) : null}
          {`${Math.abs(delta)}% vs prev`}
        </div>
      )}
    </div>
  );
}

interface MetricCardProps {
  title: string;
  sub?: string;
  children: React.ReactNode;
  span?: boolean;
  action?: React.ReactNode;
}

export function MetricCard({
  title,
  sub,
  children,
  span,
  action,
}: MetricCardProps) {
  return (
    <div className={"card pad" + (span ? " col-span-2" : "")}>
      <div className="metric-head">
        <div>
          <div className="t">{title}</div>
          {sub && <div className="s">{sub}</div>}
        </div>
        {action}
      </div>
      {children}
    </div>
  );
}

interface RepoCardProps {
  repo: D.MockRepo;
  metrics: D.MockMetricsMap;
  onOpen: (repo: D.MockRepo) => void;
}

export function RepoCard({ repo, metrics, onOpen }: RepoCardProps) {
  return (
    <div
      className="card hover repo-card fade-in"
      onClick={() => onOpen(repo)}
      style={{ cursor: "pointer" }}
    >
      <div className="rc-top">
        <div className="rc-name">
          {repo.is_private ? (
            <I.lock style={{ width: 15, height: 15, color: "var(--muted)" }} />
          ) : (
            <I.repo style={{ width: 15, height: 15, color: "var(--muted)" }} />
          )}
          <span>
            <span className="owner">{repo.owner}/</span>
            {repo.name}
          </span>
        </div>
        <SyncStatusBadge status={repo.sync_status} />
      </div>
      <div className="rc-desc">{repo.description}</div>
      <div className="rc-spark">
        <Sparkline series={metrics.commit_rate.series} />
      </div>
      <div className="rc-stats">
        <span className="rc-stat">
          <I.star style={{ width: 14, height: 14 }} />
          <b>{F.fmtNumber(repo.stargazers)}</b>
        </span>
        <span className="rc-stat">
          <I.fork style={{ width: 14, height: 14 }} />
          <b>{F.fmtNumber(repo.forks)}</b>
        </span>
        <span className="rc-stat">
          <I.pr style={{ width: 14, height: 14 }} />
          <b>{repo.open_prs}</b>
        </span>
        <span className="rc-stat">
          <I.issue style={{ width: 14, height: 14 }} />
          <b>{repo.open_issues}</b>
        </span>
      </div>
      <div className="rc-foot">
        <LangDot name={repo.lang} color={repo.langColor} />
        <span className="synced">
          {repo.sync_status === "pending"
            ? "not yet synced"
            : "synced " + F.fmtNullableTs(repo.last_synced_at)}
        </span>
      </div>
    </div>
  );
}

interface AddRepoFormProps {
  onAdd: (fullName: string) => void;
}

export function AddRepoForm({ onAdd }: AddRepoFormProps) {
  const [val, setVal] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    const v = val.trim();
    if (!/^[\w.-]+\/[\w.-]+$/.test(v)) {
      setErr("Use the owner/name format, e.g. facebook/react");
      return;
    }
    setErr("");
    setBusy(true);
    setTimeout(() => {
      onAdd(v);
      setVal("");
      setBusy(false);
    }, 450);
  };

  return (
    <form onSubmit={submit} style={{ width: "100%" }}>
      <div className="row" style={{ gap: 8 }}>
        <span className="field-icon" style={{ flex: 1, maxWidth: 340 }}>
          <I.github style={{ width: 15, height: 15 }} />
          <input
            className="input"
            placeholder="owner/name"
            value={val}
            onChange={(e) => {
              setVal(e.target.value);
              setErr("");
            }}
            aria-label="Add repository"
          />
        </span>
        <button className="btn primary" type="submit" disabled={busy}>
          {busy ? (
            "Adding…"
          ) : (
            <>
              <I.plus style={{ width: 15, height: 15 }} />
              Track repo
            </>
          )}
        </button>
      </div>
      {err && (
        <div style={{ color: "var(--red)", fontSize: 12.5, marginTop: 7 }}>
          {err}
        </div>
      )}
    </form>
  );
}

const WINDOWS = [
  { value: "30d", label: "30d" },
  { value: "90d", label: "90d" },
  { value: "6m", label: "6m" },
  { value: "1y", label: "1y" },
  { value: "all", label: "All" },
];

interface WindowControlsProps {
  win: string;
  excludeBots: boolean;
  onWin: (win: string) => void;
  onBots: (bots: boolean) => void;
}

export function WindowControls({
  win,
  excludeBots,
  onWin,
  onBots,
}: WindowControlsProps) {
  return (
    <div className="controls">
      <span className="row" style={{ gap: 9 }}>
        <span className="ctl-label">Window</span>
        <Segmented value={win} options={WINDOWS} onChange={onWin} />
      </span>
      <Switch checked={excludeBots} onChange={onBots} label="Exclude bots" />
      <span className="grow" />
    </div>
  );
}

interface RefreshButtonProps {
  onComplete?: () => void;
}

export function RefreshButton({ onComplete }: RefreshButtonProps) {
  const [running, setRunning] = useState(false);
  const [lines, setLines] = useState<D.MockSyncPhase[]>([]);
  const timer = useRef<NodeJS.Timeout | null>(null);
  const logRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    return () => {
      if (timer.current) clearInterval(timer.current);
    };
  }, []);

  useEffect(() => {
    if (logRef.current) {
      logRef.current.scrollTop = logRef.current.scrollHeight;
    }
  }, [lines]);

  const start = () => {
    if (running) return;
    setRunning(true);
    setLines([
      { phase: "init", message: "POST /api/repos/.../refresh → 202 Accepted" },
    ]);
    const phases = D.SYNC_PHASES;
    let i = 0;
    timer.current = setInterval(() => {
      const p = phases[i];
      setLines((prev) => [...prev, p]);
      i++;
      if (p.done) {
        if (timer.current) clearInterval(timer.current);
        setTimeout(() => {
          setRunning(false);
          onComplete && onComplete();
        }, 600);
      }
    }, 420);
  };

  return (
    <div style={{ width: running || lines.length ? "100%" : "auto" }}>
      <button className="btn primary" onClick={start} disabled={running}>
        <I.refresh
          style={
            running
              ? {
                  animation: "spin 1s linear infinite",
                  width: 15,
                  height: 15,
                }
              : { width: 15, height: 15 }
          }
        />
        {running ? "Refreshing…" : "Refresh now"}
      </button>
      {lines.length > 0 && (
        <div className="progress-log" ref={logRef}>
          {lines.map((l, idx) => {
            const tone = l.done ? "done" : l.phase === "error" ? "error" : "";
            return (
              <div key={idx} className={"ln " + tone}>
                <span className="ph">{l.phase}</span>
                <span>{l.message}</span>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

interface LatestListProps {
  kind: "commits" | "prs" | "issues";
  items: any[];
  limit?: number;
}

export function LatestList({ kind, items, limit = 8 }: LatestListProps) {
  const rows = items.slice(0, limit);
  if (!rows.length) return <div className="empty">Nothing yet.</div>;
  return (
    <div className="latest">
      {rows.map((it, i) => {
        if (kind === "commits") {
          return (
            <div className="item" key={it.sha || i}>
              <span className="ic green">
                <I.commit style={{ width: 14, height: 14 }} />
              </span>
              <div className="body">
                <div className="ttl">{it.msg_first_line}</div>
                <div className="sub">
                  <span className="sha">{it.sha}</span>
                  <span>·</span>
                  <span>
                    {it.author_login}
                    {it.is_bot && " 🤖"}
                  </span>
                  <span>·</span>
                  <span>{F.fmtRelative(it.committed_at)}</span>
                </div>
              </div>
              <span className="churn">
                <span className="add">+{it.additions}</span>{" "}
                <span className="del">−{it.deletions}</span>
              </span>
            </div>
          );
        }
        if (kind === "prs") {
          const tone =
            it.state === "merged"
              ? "purple"
              : it.state === "open"
                ? "green"
                : "red";
          return (
            <div className="item" key={"pr" + (it.number || i)}>
              <span className={"ic " + tone}>
                <I.pr style={{ width: 14, height: 14 }} />
              </span>
              <div className="body">
                <div className="ttl">{it.title}</div>
                <div className="sub">
                  <span>#{it.number}</span>
                  <span>·</span>
                  <span style={{ textTransform: "capitalize" }}>{it.state}</span>
                  <span>·</span>
                  <span>{it.author_login}</span>
                  <span>·</span>
                  <span>{F.fmtRelative(it.created_at)}</span>
                </div>
              </div>
              <span
                className="row"
                style={{ gap: 4, color: "var(--muted)", fontSize: 12 }}
              >
                <I.comment style={{ width: 13, height: 13 }} />
                {it.comments_count}
              </span>
            </div>
          );
        }
        // issues
        const tone = it.state === "open" ? "green" : "purple";
        return (
          <div className="item" key={"is" + (it.number || i)}>
            <span className={"ic " + tone}>
              <I.issue style={{ width: 14, height: 14 }} />
            </span>
            <div className="body">
              <div className="ttl">{it.title}</div>
              <div className="sub">
                <span>#{it.number}</span>
                <span>·</span>
                <span style={{ textTransform: "capitalize" }}>{it.state}</span>
                <span>·</span>
                <span>{it.author_login}</span>
                <span>·</span>
                <span>opened {F.fmtRelative(it.created_at)}</span>
              </div>
            </div>
            <span
              className="row"
              style={{ gap: 4, color: "var(--muted)", fontSize: 12 }}
            >
              <I.comment style={{ width: 13, height: 13 }} />
              {it.comments_count}
            </span>
          </div>
        );
      })}
    </div>
  );
}
