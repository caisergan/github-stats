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
  tone: "" | "error" | "warn" | "done";
}

export default function RefreshButton({ repoID, onComplete }: Props) {
  const [running, setRunning] = useState(false);
  const [lines, setLines] = useState<LogLine[]>([]);
  const [open, setOpen] = useState(false);
  const handleRef = useRef<SyncStreamHandle | null>(null);
  const seq = useRef(0);
  const logBoxRef = useRef<HTMLDivElement | null>(null);
  const wrapRef = useRef<HTMLDivElement | null>(null);

  // Close any open stream on unmount.
  useEffect(() => () => handleRef.current?.close(), []);

  // Dismiss the progress dropdown on an outside click.
  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, [open]);

  // Auto-scroll the log to the bottom as lines arrive — scoped to the
  // dropdown's own scroll container so the page itself never moves.
  useEffect(() => {
    const box = logBoxRef.current;
    if (open && box) {
      box.scrollTop = box.scrollHeight;
    }
  }, [lines, open]);

  function push(text: string, tone: LogLine["tone"], phase: string) {
    seq.current += 1;
    const id = seq.current;
    setLines((prev) => [...prev.slice(-49), { id, phase, text, tone }]);
  }

  async function start() {
    if (running) return;
    setRunning(true);
    setLines([]);
    setOpen(true); // reveal progress as it streams
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
        const tone: LogLine["tone"] =
          ev.phase === "error" ? "error" : ev.phase === "throttled" ? "warn" : "";
        push(ev.message || ev.phase, tone, ev.phase);
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
    <div
      className="menu-wrap"
      ref={wrapRef}
      style={{ position: "relative", display: "inline-flex", gap: 6, alignItems: "center" }}
    >
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
        <button
          className="btn ghost icon"
          onClick={() => setOpen((o) => !o)}
          title="Sync progress"
          aria-label="Sync progress"
        >
          <I.chevDown
            style={{
              width: 15,
              height: 15,
              transition: "transform .15s ease",
              transform: open ? "rotate(180deg)" : "none",
            }}
          />
        </button>
      )}

      {open && lines.length > 0 && (
        <div
          ref={logBoxRef}
          className="progress-log fade-in"
          style={{
            position: "absolute",
            right: 0,
            top: "calc(100% + 6px)",
            width: 360,
            marginTop: 0,
            maxHeight: 240,
            overflowY: "auto",
            zIndex: 60,
            boxShadow: "var(--shadow-lg)",
          }}
        >
          {lines.map((l) => (
            <div key={l.id} className={"ln " + l.tone}>
              <span className="ph">{l.phase}</span>
              <span>{l.text}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
