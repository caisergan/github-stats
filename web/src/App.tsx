import { useEffect, useState } from "react";
import { fetchMe, type Me } from "./api";

export default function App() {
  const [me, setMe] = useState<Me | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetchMe()
      .then(setMe)
      .finally(() => setLoading(false));
  }, []);

  if (loading) return <p>Loading…</p>;

  if (!me) {
    return (
      <main style={{ fontFamily: "system-ui", padding: 40 }}>
        <h1>GitHub Stats</h1>
        <a href="/auth/github">Sign in with GitHub</a>
      </main>
    );
  }

  return (
    <main style={{ fontFamily: "system-ui", padding: 40 }}>
      <h1>GitHub Stats</h1>
      <p>
        Signed in as <strong>{me.login}</strong>
        {me.avatar_url && (
          <img src={me.avatar_url} alt="" width={24} height={24}
               style={{ verticalAlign: "middle", marginLeft: 8, borderRadius: "50%" }} />
        )}
      </p>
      <a href="/auth/logout">Sign out</a>
    </main>
  );
}
