import type { LatestItem, LatestCommit, LatestPR, LatestIssue, LatestKind } from "../api";
import { fmtRelative } from "../format";

interface Props {
  kind: LatestKind;
  items: LatestItem[];
}

function isCommit(i: LatestItem): i is LatestCommit {
  return (i as LatestCommit).sha !== undefined;
}
function isPR(i: LatestItem, kind: LatestKind): i is LatestPR {
  return kind === "prs";
}

export default function LatestList({ kind, items }: Props) {
  if (items.length === 0) return <p className="empty">Nothing yet.</p>;
  return (
    <div>
      {items.map((item) => {
        if (isCommit(item)) {
          return (
            <div className="latest-item" key={item.sha}>
              <span className="meta">{fmtRelative(item.committed_at)}</span>
              <span className="title">{item.msg_first_line}</span>
              <span className="meta">{item.author_login}</span>
            </div>
          );
        }
        if (isPR(item, kind)) {
          const pr = item as LatestPR;
          return (
            <div className="latest-item" key={`pr-${pr.number}`}>
              <span className="meta">#{pr.number}</span>
              <span className="title">{pr.title}</span>
              <span className="meta">{pr.state.toLowerCase()} · {fmtRelative(pr.created_at)}</span>
            </div>
          );
        }
        const iss = item as LatestIssue;
        return (
          <div className="latest-item" key={`iss-${iss.number}`}>
            <span className="meta">#{iss.number}</span>
            <span className="title">{iss.title}</span>
            <span className="meta">{iss.state.toLowerCase()} · {fmtRelative(iss.created_at)}</span>
          </div>
        );
      })}
    </div>
  );
}
