import { Link } from "react-router-dom";
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
    <Link to={to} className="repo-card">
      <h3>{repo.full_name}</h3>
      <p className="desc">{overview?.description || (repo.is_private ? "Private repository" : "")}</p>
      <div className="stats">
        <div className="stat">
          <span className="n">{overview ? fmtNumber(overview.open_issues) : "—"}</span>
          <span className="l">issues</span>
        </div>
        <div className="stat">
          <span className="n">{overview ? fmtNumber(overview.open_prs) : "—"}</span>
          <span className="l">PRs</span>
        </div>
        <div className="stat">
          <span className="n">{overview ? fmtNumber(overview.contributors) : "—"}</span>
          <span className="l">authors</span>
        </div>
      </div>
      <div className="card-foot">
        <SyncStatusBadge status={repo.sync_status} />
        <span>synced {fmtNullableTs(repo.last_synced_at)}</span>
      </div>
    </Link>
  );
}
