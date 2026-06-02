import { useState, type FormEvent } from "react";
import { Plus, Loader2, Search } from "lucide-react";

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
    <form onSubmit={submit} className="w-full">
      <div className="flex gap-2">
        <div className="relative flex-1">
          <div className="absolute inset-y-0 left-0 pl-3.5 flex items-center pointer-events-none text-muted">
            <Search size={16} />
          </div>
          <input
            aria-label="Repository (owner/name)"
            placeholder="owner/name (e.g. facebook/react)"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            disabled={busy}
            className="w-full pl-10 pr-4 py-2 bg-surface hover:bg-surface-hover/50 border border-border rounded-lg text-sm text-text placeholder-muted input-ring"
          />
        </div>
        <button
          type="submit"
          disabled={busy}
          className="flex items-center gap-1.5 px-4 py-2 bg-accent hover:bg-accent-hover text-white text-sm font-semibold rounded-lg shadow-[0_0_15px_rgba(47,129,247,0.15)] hover:shadow-[0_0_20px_rgba(47,129,247,0.3)] transition-all duration-200 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {busy ? (
            <>
              <Loader2 size={16} className="animate-spin" />
              <span>Tracking…</span>
            </>
          ) : (
            <>
              <Plus size={16} />
              <span>Track repo</span>
            </>
          )}
        </button>
      </div>
      {error && <p className="mt-1.5 text-xs text-red font-medium">{error}</p>}
    </form>
  );
}
