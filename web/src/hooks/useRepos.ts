import { useCallback, useEffect, useState } from "react";
import { listRepos, addRepo, deleteRepo, type Repo } from "../api";

export interface UseRepos {
  repos: Repo[];
  loading: boolean;
  error: Error | null;
  reload: () => void;
  resolve: (owner: string, repo: string) => Repo | null;
  add: (fullName: string) => Promise<Repo>;
  remove: (id: number) => Promise<void>;
}

export function useRepos(): UseRepos {
  const [repos, setRepos] = useState<Repo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [nonce, setNonce] = useState(0);

  const reload = useCallback(() => setNonce((n) => n + 1), []);

  useEffect(() => {
    let active = true;
    setLoading(true);
    setError(null);
    listRepos()
      .then((rs) => {
        if (active) {
          setRepos(rs);
          setLoading(false);
        }
      })
      .catch((e: unknown) => {
        if (active) {
          setError(e instanceof Error ? e : new Error(String(e)));
          setLoading(false);
        }
      });
    return () => {
      active = false;
    };
  }, [nonce]);

  const resolve = useCallback(
    (owner: string, repo: string): Repo | null => {
      const target = `${owner}/${repo}`.toLowerCase();
      return repos.find((r) => r.full_name.toLowerCase() === target) ?? null;
    },
    [repos],
  );

  const add = useCallback(async (fullName: string): Promise<Repo> => {
    const created = await addRepo(fullName);
    setRepos((prev) => {
      const without = prev.filter((r) => r.id !== created.id);
      return [created, ...without];
    });
    return created;
  }, []);

  const remove = useCallback(async (id: number): Promise<void> => {
    await deleteRepo(id);
    setRepos((prev) => prev.filter((r) => r.id !== id));
  }, []);

  return { repos, loading, error, reload, resolve, add, remove };
}
