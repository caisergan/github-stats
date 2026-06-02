import { Link } from "react-router-dom";
import { GitPullRequest, AlertCircle, Users, BookOpen, Lock } from "lucide-react";
import type { Repo, Overview } from "../api";
import SyncStatusBadge from "./SyncStatusBadge";
import { fmtNullableTs, fmtNumber } from "../format";

interface Props {
  repo: Repo;
  overview: Overview | null;
}

export default function RepoCard({ repo, overview }: Props) {
  const to = `/${repo.full_name}`;
  return (
    <Link to={to} className="custom-glass glow-card block p-5 rounded-xl hover:text-text hover:no-underline">
      {/* Title + Visibility */}
      <div className="flex items-start justify-between gap-3 mb-2">
        <div className="flex items-center gap-2">
          <BookOpen size={16} className="text-accent" />
          <h3 className="text-sm font-bold truncate max-w-[180px]" title={repo.full_name}>
            {repo.full_name}
          </h3>
        </div>
        {repo.is_private ? (
          <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-amber/10 border border-amber/20 text-[10px] font-semibold text-amber-400">
            <Lock size={8} />
            <span>Private</span>
          </span>
        ) : (
          <span className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded bg-muted/10 border border-muted/20 text-[10px] font-semibold text-muted">
            <span>Public</span>
          </span>
        )}
      </div>

      {/* Description */}
      <p className="text-xs text-muted mb-4 h-8 line-clamp-2 overflow-hidden leading-relaxed">
        {overview?.description || (repo.is_private ? "Private repository" : "No description provided.")}
      </p>

      {/* Quick Stats Grid */}
      <div className="grid grid-cols-3 gap-2 mb-4">
        <div className="bg-surface-hover/30 border border-border/50 rounded-lg p-2 text-center transition-all duration-200 hover:bg-surface-hover/60">
          <div className="flex justify-center text-red/80 mb-0.5">
            <AlertCircle size={12} />
          </div>
          <span className="block text-sm font-bold text-text">{overview ? fmtNumber(overview.open_issues) : "—"}</span>
          <span className="text-[10px] text-muted">issues</span>
        </div>
        <div className="bg-surface-hover/30 border border-border/50 rounded-lg p-2 text-center transition-all duration-200 hover:bg-surface-hover/60">
          <div className="flex justify-center text-accent/80 mb-0.5">
            <GitPullRequest size={12} />
          </div>
          <span className="block text-sm font-bold text-text">{overview ? fmtNumber(overview.open_prs) : "—"}</span>
          <span className="text-[10px] text-muted">PRs</span>
        </div>
        <div className="bg-surface-hover/30 border border-border/50 rounded-lg p-2 text-center transition-all duration-200 hover:bg-surface-hover/60">
          <div className="flex justify-center text-green/80 mb-0.5">
            <Users size={12} />
          </div>
          <span className="block text-sm font-bold text-text">{overview ? fmtNumber(overview.contributors) : "—"}</span>
          <span className="text-[10px] text-muted">authors</span>
        </div>
      </div>

      {/* Card Footer */}
      <div className="flex items-center justify-between border-t border-border/40 pt-3 text-[11px] text-muted">
        <SyncStatusBadge status={repo.sync_status} />
        <span>synced {fmtNullableTs(repo.last_synced_at)}</span>
      </div>
    </Link>
  );
}
