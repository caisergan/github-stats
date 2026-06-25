import { useEffect, useRef, useState } from "react";
import { I } from "./Icons";
import {
  loadAllCommits,
  openSyncStream,
  fetchSyncStatus,
  type SyncEvent,
  type SyncStreamHandle,
} from "../api";

interface Props {
  repoID: number;
  onComplete: () => void;
}

type Tone = "" | "warn" | "error" | "done";

/**
 * Triggers a full-history backfill ("load all commits") and follows its progress
 * over the sync SSE stream. The backfill is quota-aware server-side: if GitHub's
 * API budget is exhausted the job is rescheduled and auto-resumes, surfaced here
 * as a "GitHub rate-limited — resuming in ~Ns" notice. State is restored after a
 * page reload by probing the repo's sync-job status on mount.
 */
export default function LoadAllCommits({ repoID, onComplete }: Props) {
  const [running, setRunning] = useState(false);
  const [status, setStatus] = useState("");
  const [tone, setTone] = useState<Tone>("");
  const handleRef = useRef<SyncStreamHandle | null>(null);

  // Close any open stream on unmount.
  useEffect(() => () => handleRef.current?.close(), []);

  // Restore in-progress state after a reload: if a job is still active, follow it.
  useEffect(() => {
    let cancelled = false;
    fetchSyncStatus(repoID)
      .then((s) => {
        if (cancelled || !s.active) return;
        setRunning(true);
        setStatus(s.status === "running" ? "Loading all history…" : "Queued…");
        follow();
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
    // follow is stable for a given repoID; re-running only on repoID is intended.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [repoID]);

  function follow() {
    handleRef.current?.close();
    handleRef.current = openSyncStream(repoID, {
      onEvent: (ev: SyncEvent) => {
        if (ev.phase === "throttled") {
          setTone("warn");
          setStatus(ev.message);
        } else if (ev.phase === "commits") {
          setTone("");
          setStatus(`Loading all history — ${ev.message}`);
        } else if (ev.phase === "error") {
          setTone("error");
          setStatus(ev.message);
        } else if (ev.message) {
          setTone("");
          setStatus(ev.message);
        }
      },
      onDone: (ev: SyncEvent) => {
        setRunning(false);
        handleRef.current = null;
        if (ev.phase === "error") {
          setTone("error");
          setStatus(ev.message || "failed");
        } else {
          setTone("done");
          setStatus("All history loaded");
          onComplete();
        }
      },
      onError: () => {
        setRunning(false);
        handleRef.current = null;
        setTone("error");
        setStatus("stream interrupted");
      },
    });
  }

  async function start() {
    if (running) return;
    setRunning(true);
    setTone("");
    setStatus("Starting…");
    try {
      await loadAllCommits(repoID);
    } catch (e) {
      setRunning(false);
      setTone("error");
      setStatus(e instanceof Error ? e.message : "failed to start");
      return;
    }
    follow();
  }

  return (
    <div className="row" style={{ gap: 10, alignItems: "center", flexWrap: "wrap" }}>
      <button className="btn sm" onClick={start} disabled={running} title="Ingest the full commit history from GitHub">
        <I.refresh
          style={
            running
              ? { animation: "spin 1s linear infinite", width: 14, height: 14 }
              : { width: 14, height: 14 }
          }
        />
        {running ? "Loading all history…" : "Load all history"}
      </button>
      {status && <span className={"load-all-status " + tone}>{status}</span>}
    </div>
  );
}
