import { useEffect, useRef } from "react";
import uPlot from "uplot";
import "uplot/dist/uPlot.min.css";
import type { SeriesPoint } from "../api";

interface Props {
  series: SeriesPoint[];
  label?: string;
  height?: number;
}

// Convert ISO date points into uPlot's columnar [xs, ys] with unix-second xs.
function toData(series: SeriesPoint[]): uPlot.AlignedData {
  const xs: number[] = [];
  const ys: number[] = [];
  for (const p of series) {
    xs.push(Date.parse(p.date + "T00:00:00Z") / 1000);
    ys.push(p.value);
  }
  return [xs, ys];
}

function makeOpts(width: number, height: number, label: string): uPlot.Options {
  return {
    width,
    height,
    cursor: { y: false },
    legend: { show: false },
    scales: { x: { time: true } },
    axes: [
      { stroke: "#8b949e", grid: { stroke: "#21262d" }, ticks: { stroke: "#30363d" } },
      { stroke: "#8b949e", grid: { stroke: "#21262d" }, ticks: { stroke: "#30363d" }, size: 44 },
    ],
    series: [
      {},
      { label, stroke: "#2f81f7", width: 2, fill: "rgba(47,129,247,0.12)", points: { show: false } },
    ],
  };
}

/**
 * TimeSeriesChart is a thin lifecycle wrapper around uPlot. It creates one
 * instance on mount, resizes it to the container, updates data when `series`
 * changes, and destroys it on unmount. An empty series renders a placeholder
 * (and never constructs uPlot) so jsdom/test and empty-data paths are safe.
 */
export default function TimeSeriesChart({ series, label = "", height = 200 }: Props) {
  const ref = useRef<HTMLDivElement | null>(null);
  const plotRef = useRef<uPlot | null>(null);

  // Create on mount; destroy on unmount. Re-create if it was previously empty.
  useEffect(() => {
    if (series.length === 0) return;
    const el = ref.current;
    if (!el) return;
    const width = el.clientWidth || 600;
    const plot = new uPlot(makeOpts(width, height, label), toData(series), el);
    plotRef.current = plot;

    const onResize = () => plot.setSize({ width: el.clientWidth || width, height });
    window.addEventListener("resize", onResize);
    return () => {
      window.removeEventListener("resize", onResize);
      plot.destroy();
      plotRef.current = null;
    };
    // Recreate only when the series identity transitions to/from empty or label/height change.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [series.length === 0, label, height]);

  // Push data updates without tearing down the instance.
  useEffect(() => {
    if (plotRef.current && series.length > 0) {
      plotRef.current.setData(toData(series));
    }
  }, [series]);

  if (series.length === 0) {
    return <p className="empty">No data for this window.</p>;
  }
  return <div className="chart" ref={ref} />;
}
