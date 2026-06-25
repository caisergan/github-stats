import type { RepoLang } from "../api";

interface Segment {
  name: string;
  color: string;
  pct: number;
}

/**
 * Reduce a full language breakdown to the two most-used languages plus an
 * aggregated "Other" slice, as percentages of total bytes.
 */
export function topTwoPlusOther(languages: RepoLang[]): Segment[] {
  const total = languages.reduce((a, l) => a + (l.size || 0), 0);
  if (total <= 0) return [];
  const sorted = [...languages].sort((a, b) => b.size - a.size);
  const segs: Segment[] = sorted.slice(0, 2).map((l) => ({
    name: l.name,
    color: l.color || "var(--muted)",
    pct: (l.size / total) * 100,
  }));
  const restSize = sorted.slice(2).reduce((a, l) => a + l.size, 0);
  if (restSize > 0) {
    segs.push({ name: "Other", color: "var(--faint)", pct: (restSize / total) * 100 });
  }
  return segs;
}

const fmtPct = (p: number) => (p >= 10 ? p.toFixed(0) : p.toFixed(1));

export default function LanguageBar({ languages }: { languages: RepoLang[] }) {
  const segs = topTwoPlusOther(languages);
  if (segs.length === 0) return null;

  return (
    <div className="lang-bar-wrap">
      <div className="lang-bar">
        {segs.map((s) => (
          <span
            key={s.name}
            style={{ width: `${s.pct}%`, background: s.color }}
            title={`${s.name} ${fmtPct(s.pct)}%`}
          />
        ))}
      </div>
      <div className="lang-legend">
        {segs.map((s) => (
          <span className="lang-leg" key={s.name}>
            <span className="d" style={{ background: s.color }} />
            <b>{s.name}</b>
            <span className="pct">{fmtPct(s.pct)}%</span>
          </span>
        ))}
      </div>
    </div>
  );
}
