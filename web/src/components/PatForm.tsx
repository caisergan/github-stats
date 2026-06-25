import { useState } from "react";
import { savePat, deletePat, type PatStatus } from "../api";

interface Props {
  status: PatStatus;
  onChange: () => void;
}

export function PatForm({ status, onChange }: Props) {
  const [token, setToken] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const save = async () => {
    setBusy(true);
    setError(null);
    try {
      await savePat(token);
      setToken("");
      onChange();
    } catch (e) {
      setError(e instanceof Error ? e.message : "save failed");
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    setBusy(true);
    try {
      await deletePat();
      onChange();
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="settings-section">
      <h2>Personal Access Token (optional)</h2>
      <p className="note">
        A fine-grained PAT is an <strong>alternate credential</strong> — useful for headless
        setups or organization repositories your OAuth app is not approved for. It does
        <strong> not</strong> raise GitHub's shared 5,000 requests/hour limit (that bucket is
        shared across your OAuth login and all PATs). For higher limits, a GitHub App is the
        only path (see the README).
      </p>
      {status.has_pat ? (
        <div>
          <p>
            A PAT is configured{status.login ? <> for <strong>{status.login}</strong></> : null}.
          </p>
          <button className="btn" onClick={remove} disabled={busy}>Remove PAT</button>
        </div>
      ) : (
        <div>
          <label>
            Token
            <input
              className="input"
              type="password"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              placeholder="github_pat_..."
              autoComplete="off"
            />
          </label>
          <button className="btn primary" onClick={save} disabled={busy || token === ""}>
            Save
          </button>
          {error && <p className="form-error">{error}</p>}
        </div>
      )}
    </div>
  );
}
