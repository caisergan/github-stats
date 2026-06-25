import { useState } from "react";
import { I } from "./Icons";

interface Props {
  repoID: number;
  repoName: string;
  /** Untracks + hard-deletes the repo's data (server-side). Resolves on success. */
  onUntrack: (id: number) => Promise<void>;
  /** Called after a successful untrack — typically navigates back to the dashboard. */
  onDone: () => void;
}

/**
 * UntrackButton stops tracking a repo and hard-deletes all of its stored data.
 * Because that is destructive and irreversible, the first click opens an inline
 * confirmation explaining exactly what happens; only the confirm button acts.
 */
export default function UntrackButton({ repoID, repoName, onUntrack, onDone }: Props) {
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const run = async () => {
    setBusy(true);
    setError(null);
    try {
      await onUntrack(repoID);
      onDone();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Untrack failed.");
      setBusy(false);
    }
  };

  return (
    <span style={{ position: "relative", display: "inline-flex" }}>
      <button
        className="btn danger"
        onClick={() => {
          setError(null);
          setConfirming((c) => !c);
        }}
        disabled={busy}
      >
        <I.trash style={{ width: 15, height: 15 }} />
        Untrack repo
      </button>

      {confirming && (
        <div className="confirm-pop card pad" role="dialog" aria-label="Confirm untrack">
          <p className="ct">Untrack {repoName}?</p>
          <p className="cs">
            This stops tracking the repository and <strong>permanently deletes</strong>{" "}
            all of its synced data — commits, pull requests, issues, and stats — from
            this app's database. This can't be undone. Your repository on GitHub is not
            touched.
          </p>
          {error && <p className="form-error">{error}</p>}
          <div className="row" style={{ gap: 8, justifyContent: "flex-end", marginTop: 12 }}>
            <button className="btn sm" onClick={() => setConfirming(false)} disabled={busy}>
              Cancel
            </button>
            <button className="btn sm danger-solid" onClick={() => void run()} disabled={busy}>
              {busy ? "Deleting…" : "Yes, untrack & delete"}
            </button>
          </div>
        </div>
      )}
    </span>
  );
}
