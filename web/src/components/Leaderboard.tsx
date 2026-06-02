import type { LeaderboardResult } from "../api";
import { fmtNumber } from "../format";

interface Props {
  result: LeaderboardResult;
}

export default function Leaderboard({ result }: Props) {
  const rows = result.rows ?? [];
  if (rows.length === 0) {
    return <p className="text-xs text-muted italic py-3">No contributors in this window.</p>;
  }

  return (
    <div className="overflow-x-auto w-full">
      <table className="w-full border-collapse text-left text-xs leading-normal">
        <thead>
          <tr className="border-b border-border/80 text-[10px] uppercase font-bold tracking-wider text-muted">
            <th className="pb-2.5 font-bold w-10">#</th>
            <th className="pb-2.5 font-bold">Contributor</th>
            <th className="pb-2.5 font-bold text-right w-20">Commits</th>
            <th className="pb-2.5 font-bold text-right text-green w-16">+ Add</th>
            <th className="pb-2.5 font-bold text-right text-red w-16">- Del</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-border/40">
          {rows.map((r, i) => (
            <tr key={r.login} className="hover:bg-surface-hover/20 transition-colors duration-150">
              <td className="py-2.5 text-muted font-semibold">{i + 1}</td>
              <td className="py-2.5 font-bold text-text">{r.login}</td>
              <td className="py-2.5 text-right font-semibold tabular-nums text-text">{fmtNumber(r.commits)}</td>
              <td className="py-2.5 text-right font-medium tabular-nums text-green-400">+{fmtNumber(r.additions)}</td>
              <td className="py-2.5 text-right font-medium tabular-nums text-red">-{fmtNumber(r.deletions)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
