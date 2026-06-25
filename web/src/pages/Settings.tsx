import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { getPatStatus, type PatStatus, type ImportResult } from "../api";
import { PatForm } from "../components/PatForm";
import { RateLimitPanel } from "../components/RateLimitPanel";
import { FileDropImport } from "../components/FileDropImport";

export default function Settings() {
  const [status, setStatus] = useState<PatStatus | null>(null);
  const [imported, setImported] = useState<ImportResult | null>(null);

  const reload = () => {
    void getPatStatus().then(setStatus).catch(() => setStatus({ has_pat: false }));
  };
  useEffect(reload, []);

  return (
    <div className="page fade-in">
      <div className="page-head">
        <div>
          <h1>Settings</h1>
          <div className="sub">
            <Link to="/">← Back to dashboard</Link>
          </div>
        </div>
      </div>

      {status && <PatForm status={status} onChange={reload} />}

      <RateLimitPanel />

      <div className="settings-section">
        <h2>Import repositories</h2>
        <p className="note">
          Import candidate repositories from a <code>package.json</code>,
          <code> requirements.txt</code>, or a previously exported collection. Resolved
          <code> owner/repo</code> entries can be added from the list below; ambiguous package
          names are shown for you to confirm.
        </p>
        <FileDropImport onResult={setImported} />
        {imported && (
          <div>
            <h3>Resolved ({imported.resolved.length})</h3>
            <ul>
              {imported.resolved.map((r) => (
                <li key={r}>
                  {r} — <a href={`/${r}`}>open</a>
                </li>
              ))}
            </ul>
            {imported.unresolved.length > 0 && (
              <>
                <h3>Could not resolve ({imported.unresolved.length})</h3>
                <ul>
                  {imported.unresolved.map((u) => (
                    <li key={u}>{u}</li>
                  ))}
                </ul>
              </>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
