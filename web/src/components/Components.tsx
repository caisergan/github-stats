import React, { useState } from "react";
import { I } from "./Icons";
import { Segmented, Switch } from "./UI";
import { RepoAccessError } from "../api";

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

interface AddRepoFormProps {
  onAdd: (fullName: string) => Promise<unknown>;
}

export function AddRepoForm({ onAdd }: AddRepoFormProps) {
  const [val, setVal] = useState("");
  const [err, setErr] = useState("");
  const [needsAccess, setNeedsAccess] = useState(false);
  const [busy, setBusy] = useState(false);

  const clearError = () => {
    setErr("");
    setNeedsAccess(false);
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const v = val.trim();
    if (!/^[\w.-]+\/[\w.-]+$/.test(v)) {
      setErr("Use the owner/name format, e.g. facebook/react");
      return;
    }
    clearError();
    setBusy(true);
    try {
      await onAdd(v);
      setVal("");
    } catch (e) {
      setErr(e instanceof Error ? e.message : "Failed to track repository.");
      setNeedsAccess(e instanceof RepoAccessError);
    } finally {
      setBusy(false);
    }
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
            disabled={busy}
            onChange={(e) => {
              setVal(e.target.value);
              clearError();
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
          {needsAccess && (
            <div style={{ marginTop: 6, color: "var(--muted)" }}>
              Grant access:{" "}
              <a href="/auth/github">Reconnect GitHub</a>
              {" · "}
              <a href="/settings">Add a token in Settings</a>
            </div>
          )}
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
