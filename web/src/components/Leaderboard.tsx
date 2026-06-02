import type { LeaderboardResult } from "../api";
import { fmtNumber } from "../format";

interface Props {
  result: LeaderboardResult;
}

export default function Leaderboard({ result }: Props) {
  const rows = result.rows ?? [];
  if (rows.length === 0) return <p className="empty">No contributors in this window.</p>;
  return (
    <table className="data">
      <thead>
        <tr>
          <th>#</th>
          <th>Contributor</th>
          <th className="num">Commits</th>
          <th className="num">+</th>
          <th className="num">−</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((r, i) => (
          <tr key={r.login}>
            <td>{i + 1}</td>
            <td>{r.login}</td>
            <td className="num">{fmtNumber(r.commits)}</td>
            <td className="num">{fmtNumber(r.additions)}</td>
            <td className="num">{fmtNumber(r.deletions)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
