import { Link } from "react-router-dom";
import type { Repo, Overview } from "../api";
import { I } from "./Icons";
import { SyncStatusBadge } from "./UI";
import { fmtNullableTs, fmtNumber, splitRepo } from "../format";

interface Props {
  repo: Repo;
  overview: Overview | null;
}

export default function RepoCard({ repo, overview }: Props) {
  const { owner, name } = splitRepo(repo.full_name);
  const stat = (n: number | undefined) => (overview ? fmtNumber(n ?? 0) : "—");

  return (
    <Link
      to={`/${repo.full_name}`}
      className="card hover repo-card fade-in"
      style={{ textDecoration: "none", color: "inherit" }}
    >
      <div className="rc-top">
        <div className="rc-name">
          {repo.is_private ? (
            <I.lock style={{ width: 15, height: 15, color: "var(--muted)" }} />
          ) : (
            <I.repo style={{ width: 15, height: 15, color: "var(--muted)" }} />
          )}
          <span>
            <span className="owner">{owner}/</span>
            {name}
          </span>
        </div>
        <span className={"badge" + (repo.is_private ? " amber" : "")}>
          {repo.is_private ? "Private" : "Public"}
        </span>
      </div>

      <div className="rc-desc">
        {overview?.description ||
          (repo.is_private ? "Private repository" : "No description provided.")}
      </div>

      <div className="rc-stats">
        <span className="rc-stat">
          <I.issue style={{ width: 14, height: 14 }} />
          <b>{stat(overview?.open_issues)}</b> issues
        </span>
        <span className="rc-stat">
          <I.pr style={{ width: 14, height: 14 }} />
          <b>{stat(overview?.open_prs)}</b> PRs
        </span>
        <span className="rc-stat">
          <I.users style={{ width: 14, height: 14 }} />
          <b>{stat(overview?.contributors)}</b> authors
        </span>
      </div>

      <div className="rc-foot">
        <SyncStatusBadge status={repo.sync_status} />
        <span className="synced">synced {fmtNullableTs(repo.last_synced_at)}</span>
      </div>
    </Link>
  );
}
