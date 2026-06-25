import { Link } from "react-router-dom";
import type { Repo, Overview, SeriesPoint } from "../api";
import { I } from "./Icons";
import { SyncStatusBadge, LangDot } from "./UI";
import { Sparkline } from "./Charts";
import LanguageBar from "./LanguageBar";
import { fmtNullableTs, fmtNumber, splitRepo } from "../format";

interface Props {
  repo: Repo;
  overview: Overview | null;
  series?: SeriesPoint[];
}

export default function RepoCard({ repo, overview, series }: Props) {
  const { owner, name } = splitRepo(repo.full_name);
  const stars = overview?.stargazers ?? repo.stargazers ?? 0;
  const forks = overview?.forks ?? repo.forks ?? 0;
  const lang = overview?.language || repo.language || "";
  const langColor = overview?.language_color || repo.language_color || "var(--muted)";
  const languages = overview?.languages ?? repo.languages ?? [];

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
        <SyncStatusBadge status={repo.sync_status} />
      </div>

      <div className="rc-desc">
        {overview?.description ||
          (repo.is_private ? "Private repository" : "No description provided.")}
      </div>

      <div className="rc-spark">
        <Sparkline series={series ?? []} />
      </div>

      <div className="rc-stats">
        <span className="rc-stat">
          <I.star style={{ width: 14, height: 14 }} />
          <b>{fmtNumber(stars)}</b>
        </span>
        <span className="rc-stat">
          <I.fork style={{ width: 14, height: 14 }} />
          <b>{fmtNumber(forks)}</b>
        </span>
        <span className="rc-stat">
          <I.pr style={{ width: 14, height: 14 }} />
          <b>{overview ? fmtNumber(overview.open_prs) : "—"}</b>
        </span>
        <span className="rc-stat">
          <I.issue style={{ width: 14, height: 14 }} />
          <b>{overview ? fmtNumber(overview.open_issues) : "—"}</b>
        </span>
      </div>

      {languages.length > 0 && <LanguageBar languages={languages} />}

      <div className="rc-foot">
        {languages.length === 0 && lang ? (
          <LangDot name={lang} color={langColor} />
        ) : (
          <span />
        )}
        <span className="synced">synced {fmtNullableTs(repo.last_synced_at)}</span>
      </div>
    </Link>
  );
}
