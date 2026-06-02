import type { Result } from "../api";
import TimeSeriesChart from "./TimeSeriesChart";
import ScalarStat from "./ScalarStat";
import BucketsBar from "./BucketsBar";
import Leaderboard from "./Leaderboard";

interface Props {
  result: Result;
}

/** MetricView dispatches a Result to the one renderer matching its `kind`. */
export default function MetricView({ result }: Props) {
  switch (result.kind) {
    case "time_series":
      return <TimeSeriesChart series={result.series} label={result.label} />;
    case "scalar":
      return <ScalarStat result={result} />;
    case "buckets":
      return <BucketsBar result={result} />;
    case "leaderboard":
      return <Leaderboard result={result} />;
    default:
      return <p className="empty">Unsupported metric type.</p>;
  }
}
