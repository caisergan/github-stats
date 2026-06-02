import type { BucketsResult } from "../api";
import { fmtNumber } from "../format";

interface Props {
  result: BucketsResult;
}

export default function BucketsBar({ result }: Props) {
  const buckets = result.buckets ?? [];
  if (buckets.length === 0) return <p className="empty">No data for this window.</p>;
  const max = Math.max(1, ...buckets.map((b) => b.count));
  return (
    <div className="buckets">
      {buckets.map((b) => (
        <div className="row" key={b.label}>
          <span className="label">{b.label}</span>
          <span className="bar">
            <span style={{ width: `${(b.count / max) * 100}%` }} />
          </span>
          <span className="n">{fmtNumber(b.count)}</span>
        </div>
      ))}
    </div>
  );
}
