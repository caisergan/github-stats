import { useState } from "react";

interface Props {
  onCreate: (name: string) => Promise<void>;
}

// CollectionManager is the "add a new collection" control on the Overview.
export function CollectionManager({ onCreate }: Props) {
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (name.trim() === "") return;
    setBusy(true);
    try {
      await onCreate(name.trim());
      setName("");
    } finally {
      setBusy(false);
    }
  };

  return (
    <form className="add-repo" onSubmit={submit}>
      <input
        className="input"
        placeholder="New collection name…"
        value={name}
        onChange={(e) => setName(e.target.value)}
      />
      <button className="btn primary" type="submit" disabled={busy || name.trim() === ""}>
        Create
      </button>
    </form>
  );
}
