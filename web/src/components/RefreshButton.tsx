import { useEffect, useRef, useState } from "react";
import { refreshRepo, openSyncStream, type SyncEvent, type SyncStreamHandle } from "../api";

interface Props {
  repoID: number;
  onComplete: () => void;
}

interface LogLine {
  id: number;
  text: string;
  tone: "" | "error" | "done";
}

export default function RefreshButton({ repoID, onComplete }: Props) {
  const [running, setRunning] = useState(false);
  const [lines, setLines] = useState<LogLine[]>([]);
  const handleRef = useRef<SyncStreamHandle | null>(null);
  const seq = useRef(0);

  // Close any open stream on unmount.
  useEffect(() => () => handleRef.current?.close(), []);

  function push(text: string, tone: LogLine["tone"]) {
    seq.current += 1;
    const id = seq.current;
    setLines((prev) => [...prev.slice(-49), { id, text, tone }]);
  }

  async function start() {
    if (running) return;
    setRunning(true);
    setLines([]);
    try {
      await refreshRepo(repoID);
    } catch (e) {
      push(e instanceof Error ? e.message : "refresh failed", "error");
      setRunning(false);
      return;
    }
    handleRef.current = openSyncStream(repoID, {
      onEvent: (ev: SyncEvent) => {
        push(ev.message || ev.phase, ev.phase === "error" ? "error" : "");
      },
      onDone: (ev: SyncEvent) => {
        push(ev.message || "done", ev.phase === "error" ? "error" : "done");
        setRunning(false);
        handleRef.current = null;
        onComplete();
      },
      onError: () => {
        push("stream interrupted", "error");
        setRunning(false);
        handleRef.current = null;
      },
    });
  }

  return (
    <div className="refresh-box">
      <button className="primary" onClick={start} disabled={running}>
        {running ? "Refreshing…" : "Refresh now"}
      </button>
      {lines.length > 0 && (
        <div className="progress-log">
          {lines.map((l) => (
            <div key={l.id} className={`line ${l.tone}`}>{l.text}</div>
          ))}
        </div>
      )}
    </div>
  );
}
