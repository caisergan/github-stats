import React, {
  useState,
  useRef,
  useLayoutEffect,
  useCallback,
  useEffect,
} from "react";
import ReactDOM from "react-dom";
import { I } from "./Icons";
import * as F from "../format";

export function useWidth(): [React.RefObject<HTMLDivElement>, number] {
  const ref = useRef<HTMLDivElement>(null);
  const [w, setW] = useState(0);
  useLayoutEffect(() => {
    if (!ref.current) return;
    if (typeof ResizeObserver === "undefined") {
      setW(400); // safe mock fallback for jsdom
      return;
    }
    const ro = new ResizeObserver((entries) => {
      for (const e of entries) setW(Math.floor(e.contentRect.width));
    });
    ro.observe(ref.current);
    setW(Math.floor(ref.current.getBoundingClientRect().width));
    return () => ro.disconnect();
  }, []);
  return [ref, w];
}

export function useTip() {
  const [tip, setTip] = useState<{ x: number; y: number; html: string } | null>(
    null,
  );
  const show = useCallback((e: React.MouseEvent, html: string) => {
    setTip({ x: e.clientX, y: e.clientY, html });
  }, []);
  const hide = useCallback(() => setTip(null), []);
  const node = tip
    ? ReactDOM.createPortal(
        <div
          className="tip"
          style={{ left: tip.x, top: tip.y }}
          dangerouslySetInnerHTML={{ __html: tip.html }}
        />,
        document.body,
      )
    : null;
  return { show, hide, node };
}

interface BucketItem {
  label: string;
  end?: string;
  value: number;
  n?: number;
}

function bucketize(
  seriesList: { date: string; value: number }[],
  target: number,
): BucketItem[] {
  if (seriesList.length <= target) {
    return seriesList.map((p) => ({ label: p.date, value: p.value }));
  }
  const size = Math.ceil(seriesList.length / target);
  const out: BucketItem[] = [];
  for (let i = 0; i < seriesList.length; i += size) {
    const slice = seriesList.slice(i, i + size);
    const sum = slice.reduce((a, p) => a + p.value, 0);
    out.push({
      label: slice[0].date,
      end: slice[slice.length - 1].date,
      value: Math.round(sum * 10) / 10,
      n: slice.length,
    });
  }
  return out;
}

const fmtDateLabel = (iso: string) => {
  return F.fmtDateShort(iso);
};

interface BarSeriesProps {
  series: { date: string; value: number }[];
  unit?: string;
  height?: number;
}

export function BarSeries({ series, unit, height = 188 }: BarSeriesProps) {
  const [ref, w] = useWidth();
  const tip = useTip();
  if (!series || series.length === 0) {
    return <div className="empty">No data in this window.</div>;
  }

  const target = w < 460 ? 26 : 44;
  const cols = bucketize(series, target);
  const padL = 34,
    padB = 22,
    padT = 8,
    padR = 6;
  const H = height,
    innerH = H - padB - padT;
  const innerW = Math.max(0, w - padL - padR);
  const max = Math.max(1, ...cols.map((c) => c.value));
  const niceMax = niceCeil(max);
  const bw = cols.length ? Math.max(2, (innerW / cols.length) * 0.62) : 0;
  const gap = cols.length ? innerW / cols.length : 0;
  const ticks = 4;

  return (
    <div className="chart-wrap" ref={ref}>
      {w > 0 && (
        <svg width={w} height={H} style={{ display: "block" }}>
          {Array.from({ length: ticks + 1 }).map((_, i) => {
            const v = (niceMax / ticks) * i;
            const y = padT + innerH - (v / niceMax) * innerH;
            return (
              <g key={i}>
                <line
                  x1={padL}
                  y1={y}
                  x2={w - padR}
                  y2={y}
                  stroke="var(--grid-1)"
                  strokeWidth="1"
                />
                <text
                  x={padL - 7}
                  y={y + 3.5}
                  textAnchor="end"
                  fontSize="10"
                  fill="var(--faint)"
                >
                  {niceFormatCompact(v)}
                </text>
              </g>
            );
          })}
          {cols.map((c, i) => {
            const x = padL + i * gap + (gap - bw) / 2;
            const h = (c.value / niceMax) * innerH;
            const y = padT + innerH - h;
            const label =
              c.end && c.end !== c.label
                ? `${fmtDateLabel(c.label)} – ${fmtDateLabel(c.end)}`
                : fmtDateLabel(c.label);
            return (
              <rect
                key={i}
                x={x}
                y={y}
                width={bw}
                height={Math.max(1, h)}
                rx={Math.min(3, bw / 2)}
                fill="var(--chart)"
                opacity="0.9"
                onMouseMove={(e) =>
                  tip.show(
                    e,
                    `<b>${c.value.toLocaleString("en-US")}</b> ${unit || ""}<br/>${label}`,
                  )
                }
                onMouseLeave={tip.hide}
                style={{ cursor: "pointer" }}
              />
            );
          })}
          {cols.map((c, i) => {
            const step = Math.ceil(cols.length / 6);
            if (i % step !== 0) return null;
            const x = padL + i * gap + gap / 2;
            return (
              <text
                key={i}
                x={x}
                y={H - 6}
                textAnchor="middle"
                fontSize="10"
                fill="var(--faint)"
              >
                {fmtDateLabel(c.label)}
              </text>
            );
          })}
        </svg>
      )}
      {tip.node}
    </div>
  );
}

interface AreaSeriesProps {
  series: { date: string; value: number }[];
  unit?: string;
  height?: number;
}

export function AreaSeries({ series, unit, height = 188 }: AreaSeriesProps) {
  const [ref, w] = useWidth();
  const tip = useTip();
  const [hoverI, setHoverI] = useState<number | null>(null);

  if (!series || series.length === 0) {
    return <div className="empty">No data in this window.</div>;
  }

  const bucketed = bucketize(series, w < 460 ? 40 : 90);
  const cols = bucketed.map((c) => c.value);
  const dates = bucketed.map((c) => c.label);
  const padL = 34,
    padB = 22,
    padT = 8,
    padR = 6;
  const H = height,
    innerH = H - padB - padT,
    innerW = Math.max(1, w - padL - padR);
  const max = Math.max(1, ...cols),
    niceMax = niceCeil(max);
  const n = cols.length;
  const X = (i: number) => padL + (n <= 1 ? innerW / 2 : (i / (n - 1)) * innerW);
  const Y = (v: number) => padT + innerH - (v / niceMax) * innerH;

  const line = cols
    .map((v, i) => `${i === 0 ? "M" : "L"}${X(i).toFixed(1)} ${Y(v).toFixed(1)}`)
    .join(" ");
  const area = `${line} L${X(n - 1).toFixed(1)} ${padT + innerH} L${X(0).toFixed(1)} ${padT + innerH} Z`;
  const uid = "ag" + Math.abs(hashStr(unit || "x"));

  return (
    <div className="chart-wrap" ref={ref}>
      {w > 0 && (
        <svg
          width={w}
          height={H}
          style={{ display: "block" }}
          onMouseMove={(e) => {
            const rect = e.currentTarget.getBoundingClientRect();
            const rel = e.clientX - rect.left - padL;
            let i = Math.round((rel / innerW) * (n - 1));
            i = Math.max(0, Math.min(n - 1, i));
            setHoverI(i);
            tip.show(
              e,
              `<b>${cols[i].toLocaleString("en-US")}</b> ${unit || ""}<br/>${fmtDateLabel(dates[i])}`,
            );
          }}
          onMouseLeave={() => {
            setHoverI(null);
            tip.hide();
          }}
        >
          <defs>
            <linearGradient id={uid} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="var(--chart)" stopOpacity="0.22" />
              <stop offset="100%" stopColor="var(--chart)" stopOpacity="0" />
            </linearGradient>
          </defs>
          {Array.from({ length: 5 }).map((_, i) => {
            const v = (niceMax / 4) * i;
            const y = Y(v);
            return (
              <g key={i}>
                <line
                  x1={padL}
                  y1={y}
                  x2={w - padR}
                  y2={y}
                  stroke="var(--grid-1)"
                  strokeWidth="1"
                />
                <text
                  x={padL - 7}
                  y={y + 3.5}
                  textAnchor="end"
                  fontSize="10"
                  fill="var(--faint)"
                >
                  {niceFormatCompact(v)}
                </text>
              </g>
            );
          })}
          <path d={area} fill={`url(#${uid})`} />
          <path
            d={line}
            fill="none"
            stroke="var(--chart)"
            strokeWidth="2"
            strokeLinejoin="round"
            strokeLinecap="round"
          />
          {hoverI != null && (
            <g>
              <line
                x1={X(hoverI)}
                y1={padT}
                x2={X(hoverI)}
                y2={padT + innerH}
                stroke="var(--border-strong)"
                strokeWidth="1"
                strokeDasharray="3 3"
              />
              <circle
                cx={X(hoverI)}
                cy={Y(cols[hoverI])}
                r="3.5"
                fill="var(--chart)"
                stroke="var(--surface)"
                strokeWidth="2"
              />
            </g>
          )}
          {dates.map((d, i) => {
            const step = Math.ceil(n / 6);
            if (i % step !== 0) return null;
            return (
              <text
                key={i}
                x={X(i)}
                y={H - 6}
                textAnchor="middle"
                fontSize="10"
                fill="var(--faint)"
              >
                {fmtDateLabel(d)}
              </text>
            );
          })}
        </svg>
      )}
      {tip.node}
    </div>
  );
}

interface SparklineProps {
  series: { date: string; value: number }[];
  height?: number;
}

export function Sparkline({ series, height = 44 }: SparklineProps) {
  const [ref, w] = useWidth();
  if (!series || !series.length) {
    return <div ref={ref} />;
  }

  const cols = bucketize(series, 30).map((c) => c.value);
  const max = Math.max(1, ...cols);
  const n = cols.length;
  const X = (i: number) => (n <= 1 ? w / 2 : (i / (n - 1)) * w);
  const Y = (v: number) => height - 3 - (v / max) * (height - 6);
  const line = cols
    .map((v, i) => `${i === 0 ? "M" : "L"}${X(i).toFixed(1)} ${Y(v).toFixed(1)}`)
    .join(" ");
  const area = `${line} L${X(n - 1)} ${height} L${X(0)} ${height} Z`;
  const uid = "sp" + Math.abs(hashStr(String(series[0]?.value)) + n);

  return (
    <div className="chart-wrap" ref={ref}>
      {w > 0 && (
        <svg width={w} height={height} style={{ display: "block" }}>
          <defs>
            <linearGradient id={uid} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="var(--chart)" stopOpacity="0.18" />
              <stop offset="100%" stopColor="var(--chart)" stopOpacity="0" />
            </linearGradient>
          </defs>
          <path d={area} fill={`url(#${uid})`} />
          <path
            d={line}
            fill="none"
            stroke="var(--chart)"
            strokeWidth="1.6"
            strokeLinejoin="round"
            strokeLinecap="round"
          />
        </svg>
      )}
    </div>
  );
}

interface BucketsBarProps {
  result: { buckets: { label: string; count: number }[] };
}

export function BucketsBar({ result }: BucketsBarProps) {
  const max = Math.max(1, ...result.buckets.map((b) => b.count));
  return (
    <div className="buckets">
      {result.buckets.map((b, i) => (
        <div className="row" key={i}>
          <span className="bl">{b.label}</span>
          <span className="bar">
            <span style={{ width: `${(b.count / max) * 100}%` }} />
          </span>
          <span className="bn tnum">{b.count}</span>
        </div>
      ))}
    </div>
  );
}

interface LeaderboardProps {
  result: { rows: { login: string; img: string; commits: number; additions: number; deletions: number }[] };
  compact?: boolean;
}

export function Leaderboard({ result, compact }: LeaderboardProps) {
  const rows = result.rows.slice(0, compact ? 5 : 10);
  const maxC = Math.max(1, ...rows.map((r) => r.commits));
  return (
    <table className="tbl">
      <thead>
        <tr>
          <th style={{ width: 30 }}></th>
          <th>Contributor</th>
          <th className="num">Commits</th>
          {!compact && <th style={{ width: 110 }}>Share</th>}
          <th className="num">+/−</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((r, i) => (
          <tr key={r.login}>
            <td>
              <span className={"rank" + (i === 0 ? " r1" : "")}>{i + 1}</span>
            </td>
            <td>
              <span className="who">
                {r.img ? (
                  <img
                    className="avatar"
                    src={r.img}
                    width="22"
                    height="22"
                    alt=""
                  />
                ) : (
                  <span
                    className="avatar"
                    style={{
                      width: 22,
                      height: 22,
                      display: "grid",
                      placeItems: "center",
                      fontSize: 10,
                      fontWeight: "bold",
                      background: "var(--surface-2)",
                    }}
                  >
                    {r.login.slice(0, 2).toUpperCase()}
                  </span>
                )}
                <span style={{ fontWeight: 500 }}>{r.login}</span>
              </span>
            </td>
            <td className="num">
              <b className="tnum">{r.commits.toLocaleString("en-US")}</b>
            </td>
            {!compact && (
              <td>
                <span className="contrib-track">
                  <span
                    className="contrib-bar"
                    style={{ width: `${(r.commits / maxC) * 100}%` }}
                  />
                </span>
              </td>
            )}
            <td className="num tnum">
              <span style={{ color: "var(--green)" }}>
                +{niceFormatCompact(r.additions)}
              </span>{" "}
              <span style={{ color: "var(--red)" }}>
                −{niceFormatCompact(r.deletions)}
              </span>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

interface ScalarStatProps {
  result: { value: number; unit?: string; count?: number };
}

export function ScalarStat({ result }: ScalarStatProps) {
  const isHours = result.unit === "hours";
  const display = isHours ? F.fmtHours(result.value) : result.value.toLocaleString("en-US");
  const parts = /^([\d.]+)(\D+)?$/.exec(display) || [display, display, ""];
  return (
    <div>
      <div className="scalar">
        <span className="v tnum">{parts[1]}</span>
        <span className="u">{parts[2] || result.unit || ""}</span>
      </div>
      {result.count != null && (
        <div className="scalar-sub">
          <I.samples style={{ width: 13, height: 13 }} />
          across {result.count.toLocaleString("en-US")} samples
        </div>
      )}
    </div>
  );
}

interface ContributionHeatmapProps {
  weeks: number[][];
}

const MONTH_ABBR = [
  "Jan",
  "Feb",
  "Mar",
  "Apr",
  "May",
  "Jun",
  "Jul",
  "Aug",
  "Sep",
  "Oct",
  "Nov",
  "Dec",
];

export function ContributionHeatmap({ weeks }: ContributionHeatmapProps) {
  const tip = useTip();
  const total = weeks.reduce(
    (a, wk) => a + wk.reduce((b, d) => b + d, 0),
    0,
  );
  const maxDay = Math.max(1, ...weeks.flat());
  const level = (c: number) => (c === 0 ? 0 : Math.min(4, Math.ceil((c / maxDay) * 4)));
  const color = (lv: number) =>
    lv === 0
      ? "var(--surface-2)"
      : `color-mix(in srgb, var(--chart) ${18 + lv * 20}%, var(--surface-2))`;

  // responsive geometry — fill the card width with square cells
  const [wrapRef, cw] = useWidth();
  const TOP = 16;
  const step = cw > 0 ? cw / weeks.length : 16; // column pitch fills available width
  const CELL = step * 0.82; // square cell, ~18% gap
  const RX = Math.max(2, CELL * 0.22);
  const W = cw || weeks.length * 16;
  const H = TOP + 7 * step - (step - CELL);

  // month labels across the top (at the first week of each new month)
  const monthLabels: { wi: number; label: string }[] = [];
  let lastMonth = -1;
  weeks.forEach((_, wi) => {
    const d = new Date("2026-05-31T00:00:00Z");
    d.setUTCDate(d.getUTCDate() - (weeks.length - 1 - wi) * 7);
    const mo = d.getUTCMonth();
    if (mo !== lastMonth) {
      monthLabels.push({ wi, label: MONTH_ABBR[mo] });
      lastMonth = mo;
    }
  });

  return (
    <div>
      <div className="between" style={{ marginBottom: 10 }}>
        <span className="muted" style={{ fontSize: 12 }}>
          <b style={{ color: "var(--fg)", fontWeight: 650 }}>
            {total.toLocaleString("en-US")}
          </b>{" "}
          contributions in the last year
        </span>
      </div>
      <div ref={wrapRef} style={{ paddingBottom: 4 }}>
        {cw > 0 && (
          <svg width={W} height={H} style={{ display: "block" }}>
            {monthLabels.map((m, idx) =>
              idx === monthLabels.length - 1 ||
              monthLabels[idx + 1].wi - m.wi >= 3 ? (
                <text
                  key={idx}
                  x={m.wi * step}
                  y={10}
                  fontSize="10.5"
                  fill="var(--muted)"
                >
                  {m.label}
                </text>
              ) : null,
            )}
            {weeks.map((wk, wi) =>
              wk.map((c, di) => {
                const lv = level(c);
                const d = new Date("2026-05-31T00:00:00Z");
                d.setUTCDate(
                  d.getUTCDate() - ((weeks.length - 1 - wi) * 7 + (6 - di)),
                );
                return (
                  <rect
                    key={wi + "-" + di}
                    x={wi * step}
                    y={TOP + di * step}
                    width={CELL}
                    height={CELL}
                    rx={RX}
                    fill={color(lv)}
                    stroke="var(--border)"
                    strokeWidth="0.5"
                    onMouseMove={(e) =>
                      tip.show(
                        e,
                        `<b>${c} commits</b><br/>${F.fmtDate(
                          d.toISOString().slice(0, 10),
                        )}`,
                      )
                    }
                    onMouseLeave={tip.hide}
                    style={{ cursor: "pointer" }}
                  />
                );
              }),
            )}
          </svg>
        )}
      </div>
      <div className="hm-scale">
        Less
        {[0, 1, 2, 3, 4].map((lv) => (
          <span key={lv} className="cell" style={{ background: color(lv) }} />
        ))}
        More
      </div>
      {tip.node}
    </div>
  );
}

interface MetricViewProps {
  result: any;
  compact?: boolean;
}

export function MetricView({ result, compact }: MetricViewProps) {
  if (!result) return <div className="empty">No data.</div>;
  switch (result.kind) {
    case "time_series":
      return result._area ? (
        <AreaSeries series={result.series} unit={result.unit} />
      ) : (
        <BarSeries series={result.series} unit={result.unit} />
      );
    case "scalar":
      return <ScalarStat result={result} />;
    case "buckets":
      return <BucketsBar result={result} />;
    case "leaderboard":
      return <Leaderboard result={result} compact={compact} />;
    default:
      return <div className="empty">Unsupported metric.</div>;
  }
}

/* Helpers */
function niceCeil(v: number): number {
  if (v <= 0) return 1;
  const mag = Math.pow(10, Math.floor(Math.log10(v)));
  const n = v / mag;
  const step = n <= 1 ? 1 : n <= 2 ? 2 : n <= 5 ? 5 : 10;
  return step * mag;
}

function niceFormatCompact(v: number): string {
  if (v >= 1000000) {
    return (v / 1000000).toFixed(v % 1000000 === 0 ? 0 : 1) + "M";
  }
  if (v >= 1000) {
    return (v / 1000).toFixed(v % 1000 === 0 ? 0 : 1) + "k";
  }
  return Math.round(v * 10) / 10 + "";
}

function hashStr(s: string): number {
  let h = 0;
  for (let i = 0; i < (s || "").length; i++) {
    h = (h * 31 + s.charCodeAt(i)) | 0;
  }
  return h;
}
