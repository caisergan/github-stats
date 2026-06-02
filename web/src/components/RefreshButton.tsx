import { useEffect, useRef, useState } from "react";
import { RefreshCw, Terminal, AlertTriangle, CheckCircle2 } from "lucide-react";
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
  const logEndRef = useRef<HTMLDivElement | null>(null);

  // Close any open stream on unmount.
  useEffect(() => () => handleRef.current?.close(), []);

  // Auto-scroll to bottom of log when lines update
  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [lines]);

  function push(text: string, tone: LogLine["tone"]) {
    seq.current += 1;
    const id = seq.current;
    setLines((prev) => [...prev.slice(-49), { id, text, tone }]);
  }

  async function start() {
    if (running) return;
    setRunning(true);
    setLines([]);
    push("initializing repository synchronization...", "");
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
        push("stream interrupted prematurely", "error");
        setRunning(false);
        handleRef.current = null;
      },
    });
  }

  return (
    <div className="flex flex-col gap-3">
      <button
        onClick={start}
        disabled={running}
        className="flex items-center justify-center gap-2 px-4 py-2 bg-accent hover:bg-accent-hover text-white text-xs font-bold uppercase tracking-wider rounded-lg shadow-[0_0_15px_rgba(47,129,247,0.15)] hover:shadow-[0_0_20px_rgba(47,129,247,0.3)] transition-all duration-200 disabled:opacity-60 disabled:cursor-not-allowed"
      >
        <RefreshCw size={14} className={running ? "animate-spin" : ""} />
        <span>{running ? "Syncing…" : "Refresh Now"}</span>
      </button>

      {lines.length > 0 && (
        <div className="custom-glass rounded-xl overflow-hidden shadow-[0_4px_20px_rgba(0,0,0,0.4)] border border-border/60">
          {/* Terminal Console Header */}
          <div className="bg-surface/90 px-4 py-2 border-b border-border/40 flex items-center justify-between">
            <div className="flex items-center gap-2 text-xs font-bold text-muted uppercase tracking-wider">
              <Terminal size={12} className="text-accent" />
              <span>Sync Stream Logs</span>
            </div>
            <div className="flex gap-1.5">
              <div className="w-2.5 h-2.5 rounded-full bg-red/30" />
              <div className="w-2.5 h-2.5 rounded-full bg-amber/30" />
              <div className="w-2.5 h-2.5 rounded-full bg-green/30" />
            </div>
          </div>

          {/* Console Body */}
          <div className="p-4 bg-black/90 font-mono text-[11px] leading-relaxed max-h-40 overflow-y-auto space-y-1">
            {lines.map((l) => {
              if (l.tone === "error") {
                return (
                  <div key={l.id} className="flex items-start gap-1.5 text-red/90">
                    <AlertTriangle size={12} className="shrink-0 mt-0.5" />
                    <span>[ERR] {l.text}</span>
                  </div>
                );
              }
              if (l.tone === "done") {
                return (
                  <div key={l.id} className="flex items-start gap-1.5 text-green-400">
                    <CheckCircle2 size={12} className="shrink-0 mt-0.5 animate-bounce" />
                    <span>[OK] {l.text}</span>
                  </div>
                );
              }
              return (
                <div key={l.id} className="text-muted/90 flex items-start gap-1.5 pl-4">
                  <span>&gt;</span>
                  <span>{l.text}</span>
                </div>
              );
            })}
            <div ref={logEndRef} />
          </div>
        </div>
      )}
    </div>
  );
}
