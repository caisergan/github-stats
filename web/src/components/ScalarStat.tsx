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
    <div className="scalar-wrap">
      <div className="scalar">
        <span className="value">{display}</span>
        {unitLabel && <span className="unit">{unitLabel}</span>}
      </div>
      {typeof result.count === "number" && (
        <div className="count">n = {fmtNumber(result.count)}</div>
      )}
    </div>
  );
}
