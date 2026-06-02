import { useEffect, useState } from "react";
import type { Me, Repo, Overview as OverviewBundle } from "../api";
import { fetchOverview } from "../api";
import { useRepos } from "../hooks/useRepos";
import UserBar from "../components/UserBar";
import AddRepoForm from "../components/AddRepoForm";
import RepoCard from "../components/RepoCard";

interface Props {
  me: Me;
}

// Hydrate each card's headline numbers from the per-repo overview bundle.
function useOverviews(repos: Repo[]): Record<number, OverviewBundle> {
  const [map, setMap] = useState<Record<number, OverviewBundle>>({});
  useEffect(() => {
    let active = true;
    for (const r of repos) {
      fetchOverview(r.id, { window: "30d", excludeBots: false })
        .then((ov) => {
          if (active) setMap((prev) => ({ ...prev, [r.id]: ov }));
        })
        .catch(() => {
          /* card falls back to placeholders */
        });
    }
    return () => {
      active = false;
    };
  }, [repos]);
  return map;
}

export default function Overview({ me }: Props) {
  const { repos, loading, error, add } = useRepos();
  const overviews = useOverviews(repos);

  return (
    <div className="app-shell">
      <UserBar me={me} />
      <AddRepoForm onAdd={add} />

      {loading && <p className="state">Loading repositories…</p>}
      {error && <p className="state error">Failed to load repositories: {error.message}</p>}
      {!loading && !error && repos.length === 0 && (
        <div className="notice">No repositories tracked yet. Add one above to get started.</div>
      )}

      <div className="repo-grid">
        {repos.map((r) => (
          <RepoCard key={r.id} repo={r} overview={overviews[r.id] ?? null} />
        ))}
      </div>
    </div>
  );
}
