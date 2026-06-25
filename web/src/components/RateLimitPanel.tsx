import { useEffect, useState } from "react";
import { fetchRateLimit, type RateLimit, type RateBucket } from "../api";
import { fmtNumber, fmtRelative } from "../format";

// GitHub's documented hourly caps for authenticated use: 5,000 REST requests and
// 5,000 GraphQL points. The budget endpoint reports `remaining` only, so we
// measure against these official caps to show how much has been used.
const LIMIT = 5000;

function tone(remaining: number): "green" | "amber" | "red" {
  const frac = remaining / LIMIT;
  if (frac <= 0.1) return "red";
  if (frac <= 0.25) return "amber";
  return "green";
}

function Meter({ label, unit, bucket }: { label: string; unit: string; bucket: RateBucket }) {
  const remaining = Math.max(0, Math.min(LIMIT, bucket.remaining));
  const used = LIMIT - remaining;
  const pct = (remaining / LIMIT) * 100;
  return (
    <div className="rl-row">
      <div className="rl-head">
        <span className="rl-name">{label}</span>
        <span className="rl-fig">
          {fmtNumber(remaining)}
          <span className="muted">
            {" / "}
            {fmtNumber(LIMIT)} {unit} left
          </span>
        </span>
      </div>
      <div className="rl-bar">
        <span className={"fill " + tone(remaining)} style={{ width: `${pct}%` }} />
      </div>
      <div className="rl-meta">
        <span>{fmtNumber(used)} used this hour</span>
        <span>{bucket.reset ? `resets ${fmtRelative(bucket.reset)}` : "not measured yet"}</span>
      </div>
    </div>
  );
}

/**
 * RateLimitPanel shows the workspace's current GitHub API budget (REST requests
 * and GraphQL points remaining this hour) from the existing GET /api/rate-limit
 * endpoint, measured against GitHub's official 5,000/hr caps.
 */
export function RateLimitPanel() {
  const [data, setData] = useState<RateLimit | null>(null);
  const [error, setError] = useState(false);
  const [loading, setLoading] = useState(true);

  const load = () => {
    setLoading(true);
    fetchRateLimit()
      .then((d) => {
        setData(d);
        setError(false);
      })
      .catch(() => setError(true))
      .finally(() => setLoading(false));
  };
  useEffect(load, []);

  return (
    <div className="settings-section">
      <h2>GitHub API rate limits</h2>
      <p className="note">
        GitHub allows 5,000 REST requests and 5,000 GraphQL points per hour for
        authenticated use. These reflect this workspace's shared budget, updated as
        repositories sync.
      </p>
      {error ? (
        <p className="form-error">Couldn't load rate limits.</p>
      ) : !data ? (
        <p className="note">{loading ? "Loading…" : "No data."}</p>
      ) : (
        <div className="card pad rl-card">
          <Meter label="REST" unit="requests" bucket={data.rest} />
          <Meter label="GraphQL" unit="points" bucket={data.graphql} />
          <div className="rl-foot">
            <button className="btn sm ghost" onClick={load} disabled={loading}>
              {loading ? "Refreshing…" : "Refresh"}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
