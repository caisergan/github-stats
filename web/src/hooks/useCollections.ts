import { useCallback, useEffect, useState } from "react";
import {
  listCollections,
  createCollection,
  renameCollection,
  deleteCollection,
  addRepoToCollection,
  removeRepoFromCollection,
  type Collection,
} from "../api";

export function useCollections() {
  const [collections, setCollections] = useState<Collection[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      setCollections(await listCollections());
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e : new Error(String(e)));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

  return {
    collections,
    loading,
    error,
    reload,
    create: async (name: string) => {
      await createCollection(name);
      await reload();
    },
    rename: async (id: number, name: string) => {
      await renameCollection(id, name);
      await reload();
    },
    remove: async (id: number) => {
      await deleteCollection(id);
      await reload();
    },
    addRepo: async (cid: number, rid: number) => {
      await addRepoToCollection(cid, rid);
      await reload();
    },
    removeRepo: async (cid: number, rid: number) => {
      await removeRepoFromCollection(cid, rid);
      await reload();
    },
  };
}
