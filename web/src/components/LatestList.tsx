import { GitCommit, GitPullRequest, AlertCircle } from "lucide-react";
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
  if (items.length === 0) {
    return <p className="text-xs text-muted italic py-3">Nothing yet.</p>;
  }

  return (
    <div className="space-y-1">
      {items.map((item) => {
        if (isCommit(item)) {
          return (
            <div className="flex items-center gap-3 p-2 hover:bg-surface-hover/20 rounded-lg border border-transparent hover:border-border/30 transition-all duration-200" key={item.sha}>
              <div className="w-8 h-8 rounded-lg bg-accent/10 border border-accent/20 flex items-center justify-center text-accent shrink-0">
                <GitCommit size={15} />
              </div>
              <div className="flex-1 min-w-0">
                <p className="text-xs font-bold text-text truncate">{item.msg_first_line}</p>
                <p className="text-[10px] text-muted truncate">
                  by <span className="font-semibold text-text">{item.author_login}</span> · {fmtRelative(item.committed_at)}
                </p>
              </div>
            </div>
          );
        }
        if (isPR(item, kind)) {
          const pr = item as LatestPR;
          const isMerged = pr.merged_at !== null;
          const isClosed = pr.closed_at !== null;
          const statusColor = isMerged
            ? "text-purple-400 bg-purple-500/10 border-purple-500/20"
            : isClosed
              ? "text-red bg-red/10 border-red/20"
              : "text-green bg-green/10 border-green/20";

          return (
            <div className="flex items-center gap-3 p-2 hover:bg-surface-hover/20 rounded-lg border border-transparent hover:border-border/30 transition-all duration-200" key={`pr-${pr.number}`}>
              <div className={`w-8 h-8 rounded-lg border flex items-center justify-center shrink-0 ${statusColor}`}>
                <GitPullRequest size={15} />
              </div>
              <div className="flex-1 min-w-0">
                <p className="text-xs font-bold text-text truncate">
                  <span className="text-muted mr-1.5">#{pr.number}</span>
                  {pr.title}
                </p>
                <p className="text-[10px] text-muted truncate">
                  by <span className="font-semibold text-text">{pr.author_login}</span> · {fmtRelative(pr.created_at)}
                </p>
              </div>
            </div>
          );
        }
        const iss = item as LatestIssue;
        const isClosed = iss.closed_at !== null;
        const statusColor = isClosed
          ? "text-purple-400 bg-purple-500/10 border-purple-500/20"
          : "text-red bg-red/10 border-red/20";

        return (
          <div className="flex items-center gap-3 p-2 hover:bg-surface-hover/20 rounded-lg border border-transparent hover:border-border/30 transition-all duration-200" key={`iss-${iss.number}`}>
            <div className={`w-8 h-8 rounded-lg border flex items-center justify-center shrink-0 ${statusColor}`}>
              <AlertCircle size={15} />
            </div>
            <div className="flex-1 min-w-0">
              <p className="text-xs font-bold text-text truncate">
                <span className="text-muted mr-1.5">#{iss.number}</span>
                {iss.title}
              </p>
              <p className="text-[10px] text-muted truncate">
                by <span className="font-semibold text-text">{iss.author_login}</span> · {fmtRelative(iss.created_at)}
              </p>
            </div>
          </div>
        );
      })}
    </div>
  );
}
