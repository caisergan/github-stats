import { useEffect, useState } from "react";
import { Routes, Route, Link } from "react-router-dom";
import { fetchMe, type Me } from "./api";
import Overview from "./pages/Overview";
import RepoDetail from "./pages/RepoDetail";

function SignIn() {
  return (
    <div className="app-shell">
      <header className="user-bar"><span className="brand">GitHub Stats</span></header>
      <div className="notice">
        <p>Track public and private repository analytics without GitHub premium.</p>
        <p><a className="primary-link" href="/auth/github">Sign in with GitHub</a></p>
      </div>
    </div>
  );
}

function NotFound() {
  return (
    <div className="app-shell">
      <div className="notice">
        <p>Page not found.</p>
        <p><Link to="/">← Back to overview</Link></p>
      </div>
    </div>
  );
}

export default function App() {
  const [me, setMe] = useState<Me | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetchMe()
      .then(setMe)
      .catch(() => setMe(null))
      .finally(() => setLoading(false));
  }, []);

  if (loading) return <div className="app-shell"><p className="state">Loading…</p></div>;
  if (!me) return <SignIn />;

  return (
    <Routes>
      <Route path="/" element={<Overview me={me} />} />
      <Route path="/:owner/:repo" element={<RepoDetail />} />
      <Route path="*" element={<NotFound />} />
    </Routes>
  );
}
