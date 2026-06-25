import type { RateLimit } from "../api";
import { fmtRelative } from "../format";

interface Props {
  rateLimit: RateLimit | null;
  threshold?: number;
}

// RateLimitBanner warns when either GitHub bucket is running low (a sync may be
// paused until the reset time). Honest: this reflects the shared per-user budget.
export function RateLimitBanner({ rateLimit, threshold = 50 }: Props) {
  if (!rateLimit) return null;
  const low: string[] = [];
  let reset = "";
  if (rateLimit.rest.remaining <= threshold) {
    low.push("REST");
    reset = rateLimit.rest.reset || reset;
  }
  if (rateLimit.graphql.remaining <= threshold) {
    low.push("GraphQL");
    reset = rateLimit.graphql.reset || reset;
  }
  if (low.length === 0) return null;
  return (
    <div className="rate-banner" role="alert">
      <span>⚠</span>
      <span>
        GitHub rate limit low ({low.join(" + ")}). Syncing is throttled
        {reset ? <> and resets {fmtRelative(reset)}</> : null}.
      </span>
    </div>
  );
}
