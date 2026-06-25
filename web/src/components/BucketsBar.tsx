import type { BucketsResult } from "../api";
import { fmtNumber } from "../format";

interface Props {
  result: BucketsResult;
}

export default function BucketsBar({ result }: Props) {
  const buckets = result.buckets ?? [];
  if (buckets.length === 0) {
    return <p className="text-xs text-muted italic py-3">No data for this window.</p>;
  }
  const max = Math.max(1, ...buckets.map((b) => b.count));

  return (
    <div className="space-y-3 py-1">
      {buckets.map((b) => (
        <div className="flex items-center gap-3" key={b.label}>
          {/* Label */}
          <span className="w-16 text-xs font-medium text-muted truncate text-left" title={b.label}>
            {b.label}
          </span>
          
          {/* Progress Capsule Bar */}
          <div className="flex-1 h-2.5 bg-surface-hover/80 border border-border/40 rounded-full overflow-hidden">
            <div
              className="h-full bg-accent rounded-full shadow-[0_0_8px_rgba(47,129,247,0.25)] transition-all duration-500"
              style={{ width: `${(b.count / max) * 100}%` }}
            />
          </div>

          {/* Value Count */}
          <span className="w-8 text-right text-xs font-bold text-text tabular-nums">
            {fmtNumber(b.count)}
          </span>
        </div>
      ))}
    </div>
  );
}
