import { useEffect, useRef, useState } from "react";
import { I } from "./Icons";
import { refreshRepo, openSyncStream, type SyncEvent, type SyncStreamHandle } from "../api";

interface Props {
  repoID: number;
  onComplete: () => void;
}

interface LogLine {
  id: number;
  phase: string;
  text: string;
  tone: "" | "error" | "done";
}

export default function RefreshButton({ repoID, onComplete }: Props) {
  const [running, setRunning] = useState(false);
  const [lines, setLines] = useState<LogLine[]>([]);
  const handleRef = useRef<SyncStreamHandle | null>(null);
  const seq = useRef(0);
  const logEndRef = useRef<HTMLDivElement | null>(null);

  // Close any open stream on unmount.
  useEffect(() => () => handleRef.current?.close(), []);

  // Auto-scroll the log to the bottom as lines arrive.
  useEffect(() => {
    if (typeof logEndRef.current?.scrollIntoView === "function") {
      logEndRef.current.scrollIntoView({ behavior: "smooth" });
    }
  }, [lines]);

  function push(text: string, tone: LogLine["tone"], phase: string) {
    seq.current += 1;
    const id = seq.current;
    setLines((prev) => [...prev.slice(-49), { id, phase, text, tone }]);
  }

  async function start() {
    if (running) return;
    setRunning(true);
    setLines([]);
    push("initializing repository synchronization…", "", "init");
    try {
      await refreshRepo(repoID);
    } catch (e) {
      push(e instanceof Error ? e.message : "refresh failed", "error", "error");
      setRunning(false);
      return;
    }
    handleRef.current = openSyncStream(repoID, {
      onEvent: (ev: SyncEvent) => {
        push(ev.message || ev.phase, ev.phase === "error" ? "error" : "", ev.phase);
      },
      onDone: (ev: SyncEvent) => {
        push(ev.message || "done", ev.phase === "error" ? "error" : "done", ev.phase || "done");
        setRunning(false);
        handleRef.current = null;
        onComplete();
      },
      onError: () => {
        push("stream interrupted prematurely", "error", "error");
        setRunning(false);
        handleRef.current = null;
      },
    });
  }

  return (
    <div style={{ width: running || lines.length ? "100%" : "auto" }}>
      <button className="btn primary" onClick={start} disabled={running}>
        <I.refresh
          style={
            running
              ? { animation: "spin 1s linear infinite", width: 15, height: 15 }
              : { width: 15, height: 15 }
          }
        />
        {running ? "Refreshing…" : "Refresh now"}
      </button>
      {lines.length > 0 && (
        <div className="progress-log">
          {lines.map((l) => (
            <div key={l.id} className={"ln " + l.tone}>
              <span className="ph">{l.phase}</span>
              <span>{l.text}</span>
            </div>
          ))}
          <div ref={logEndRef} />
        </div>
      )}
    </div>
  );
}
