import type { SyncStatus } from "../api";

interface Props {
  status: SyncStatus;
}

const LABELS: Record<string, string> = {
  complete: "Synced",
  running: "Syncing",
  pending: "Queued",
  error: "Error",
};

const STYLES: Record<string, { bg: string; border: string; text: string; dot: string }> = {
  complete: {
    bg: "bg-green/10",
    border: "border-green/20",
    text: "text-green-400",
    dot: "bg-green-500 shadow-[0_0_8px_#2ea043]",
  },
  running: {
    bg: "bg-accent/10",
    border: "border-accent/20",
    text: "text-accent",
    dot: "bg-accent animate-status-pulse shadow-[0_0_8px_#2f81f7]",
  },
  pending: {
    bg: "bg-amber/10",
    border: "border-amber/20",
    text: "text-amber-400",
    dot: "bg-amber shadow-[0_0_8px_#d29922]",
  },
  error: {
    bg: "bg-red/10",
    border: "border-red/20",
    text: "text-red-400",
    dot: "bg-red shadow-[0_0_8px_#f85149]",
  },
};

export default function SyncStatusBadge({ status }: Props) {
  const key = status || "pending";
  const cls = ["complete", "running", "pending", "error"].includes(key) ? key : "pending";
  const style = STYLES[cls];

  return (
    <span className={`inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full border text-xs font-semibold ${style.bg} ${style.border} ${style.text}`}>
      <span className={`w-1.5 h-1.5 rounded-full ${style.dot}`} />
      {LABELS[cls] ?? key}
    </span>
  );
}
