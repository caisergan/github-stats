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

export default function SyncStatusBadge({ status }: Props) {
  const key = status || "pending";
  const cls = ["complete", "running", "pending", "error"].includes(key) ? key : "pending";
  return (
    <span className={`badge ${cls}`}>
      <span className="dot" />
      {LABELS[cls] ?? key}
    </span>
  );
}
