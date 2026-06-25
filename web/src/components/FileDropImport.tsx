import { useRef, useState } from "react";
import { importManifest, type ImportResult } from "../api";
import { detectManifestKind } from "../importparse";

interface Props {
  onResult: (r: ImportResult) => void;
}

// readText reads a File as UTF-8 text. Prefers the modern Blob.text() but falls
// back to FileReader for environments that lack it (e.g. jsdom under Vitest).
function readText(file: File): Promise<string> {
  if (typeof file.text === "function") return file.text();
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result ?? ""));
    reader.onerror = () => reject(reader.error ?? new Error("read failed"));
    reader.readAsText(file);
  });
}

export function FileDropImport({ onResult }: Props) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleFile = async (file: File) => {
    setError(null);
    const content = await readText(file);
    const kind = detectManifestKind(file.name, content);
    if (!kind) {
      setError("Unrecognized file. Use package.json, requirements.txt, or an exported collection.");
      return;
    }
    try {
      const result = await importManifest(kind, content);
      onResult(result);
    } catch (e) {
      setError(e instanceof Error ? e.message : "import failed");
    }
  };

  return (
    <div
      className={"file-drop" + (dragging ? " dragging" : "")}
      onDragOver={(e) => {
        e.preventDefault();
        setDragging(true);
      }}
      onDragLeave={() => setDragging(false)}
      onDrop={(e) => {
        e.preventDefault();
        setDragging(false);
        if (e.dataTransfer.files[0]) void handleFile(e.dataTransfer.files[0]);
      }}
      onClick={() => inputRef.current?.click()}
    >
      <p>Drop a package.json, requirements.txt, or exported collection here, or click to choose.</p>
      <input
        ref={inputRef}
        data-testid="file-input"
        type="file"
        accept=".json,.txt"
        style={{ display: "none" }}
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) void handleFile(f);
        }}
      />
      {error && <p className="form-error">{error}</p>}
    </div>
  );
}
