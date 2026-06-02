import type { ScalarResult } from "../api";
import { fmtHours, fmtNumber } from "../format";

interface Props {
  result: ScalarResult;
}

export default function ScalarStat({ result }: Props) {
  const hasValue = typeof result.value === "number";
  const isHours = result.unit === "hours";
  const display = !hasValue
    ? "—"
    : isHours
      ? fmtHours(result.value as number)
      : fmtNumber(result.value as number);
  const unitLabel = isHours ? "" : (result.unit ?? "");

  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-baseline gap-1.5">
        <span className="text-3xl font-black text-text tracking-tight hover:text-accent transition-colors duration-200">
          {display}
        </span>
        {unitLabel && (
          <span className="text-xs font-semibold uppercase text-muted tracking-wider">
            {unitLabel}
          </span>
        )}
      </div>
      {typeof result.count === "number" && (
        <span className="self-start text-[10px] font-bold text-muted bg-surface-hover px-1.5 py-0.5 rounded border border-border/40">
          n = {fmtNumber(result.count)}
        </span>
      )}
    </div>
  );
}
