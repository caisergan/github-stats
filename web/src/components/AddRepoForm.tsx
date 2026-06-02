import { useState, type FormEvent } from "react";

interface Props {
  onAdd: (fullName: string) => Promise<unknown>;
}

const FULL_NAME = /^[\w.-]+\/[\w.-]+$/;

export default function AddRepoForm({ onAdd }: Props) {
  const [value, setValue] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    const trimmed = value.trim();
    if (!FULL_NAME.test(trimmed)) {
      setError("Enter a repository as owner/name.");
      return;
    }
    setError(null);
    setBusy(true);
    try {
      await onAdd(trimmed);
      setValue("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to add repository.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit}>
      <div className="add-repo">
        <input
          aria-label="Repository (owner/name)"
          placeholder="owner/name"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          disabled={busy}
        />
        <button type="submit" className="primary" disabled={busy}>
          {busy ? "Adding…" : "Track repo"}
        </button>
      </div>
      {error && <p className="form-error">{error}</p>}
    </form>
  );
}
