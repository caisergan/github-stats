import React, { useState, useEffect } from "react";
import { Routes, Route, Link, useNavigate, useLocation, useParams } from "react-router-dom";
import { I } from "./components/Icons";
import { Menu } from "./components/UI";
import {
  TweaksPanel,
  TweakSection,
  TweakRadio,
  TweakSlider,
  TweakColor,
} from "./components/TweaksPanel";
import Overview from "./pages/Overview";
import RepoDetail from "./pages/RepoDetail";
import Collections from "./pages/Collections";
import WorkspaceInsights from "./pages/WorkspaceInsights";
import Settings from "./pages/Settings";
import { ThemeToggle } from "./components/ThemeToggle";
import { getTheme, setTheme, type Theme } from "./theme";
import { logout, fetchMe } from "./api";
import type { Repo, Me } from "./api";
import { useAsync } from "./hooks/useAsync";
import { useRepos } from "./hooks/useRepos";
import { useCollections } from "./hooks/useCollections";

const ACCENTS = ["#18181b", "#2563eb", "#059669", "#7c3aed", "#dc2626"];
const FONTS = {
  Geist: '"Geist", ui-sans-serif, system-ui, sans-serif',
  Inter: '"Inter", ui-sans-serif, system-ui, sans-serif',
  System: 'ui-sans-serif, system-ui, -apple-system, "Segoe UI", sans-serif',
};

const TWEAK_DEFAULTS = {
  accent: "#18181b",
  theme: "light",
  font: "Geist",
  radius: 8,
  density: "balanced",
};

function accentFg(hex: string) {
  const c = hex.replace("#", "");
  const r = parseInt(c.slice(0, 2), 16);
  const g = parseInt(c.slice(2, 4), 16);
  const b = parseInt(c.slice(4, 6), 16);
  const lum = (0.299 * r + 0.587 * g + 0.114 * b) / 255;
  return lum > 0.6 ? "#0a0a0c" : "#ffffff";
}

interface UserMenuProps {
  me: Me;
}

function UserMenu({ me }: UserMenuProps) {
  // POST /auth/logout with an X-CSRF-Token header (api.logout handles CSRF),
  // replacing the M5 GET/anchor sign-out.
  const signOut = async () => {
    try {
      await logout(false);
    } finally {
      window.location.href = "/";
    }
  };

  return (
    <Menu
      trigger={
        <span className="user-chip">
          {me.avatar_url ? (
            <img src={me.avatar_url} alt="" />
          ) : (
            <span
              style={{
                width: 26,
                height: 26,
                borderRadius: "99px",
                background: "var(--border)",
                display: "grid",
                placeItems: "center",
                fontWeight: "bold",
              }}
            >
              {me.login.slice(0, 2).toUpperCase()}
            </span>
          )}
          <span className="nm">{me.login}</span>
          <I.chevDown style={{ width: 14, height: 14, color: "var(--muted)" }} />
        </span>
      }
    >
      <div className="mhead">
        Signed in as<b>{me.login}</b>
      </div>
      <div className="sep" />
      <Link to="/settings" className="mi">
        <I.settings style={{ width: 14, height: 14 }} />
        Settings
      </Link>
      <a
        href={`https://github.com/${me.login}`}
        target="_blank"
        rel="noopener noreferrer"
        className="mi"
      >
        <I.github style={{ width: 14, height: 14 }} />
        GitHub profile
      </a>
      <div className="sep" />
      <button type="button" className="mi" onClick={signOut}>
        <I.signout style={{ width: 14, height: 14 }} />
        Sign out
      </button>
    </Menu>
  );
}

function SignIn() {
  return (
    <div className="signin">
      <div className="card pad box">
        <div className="logo-lg">
          <I.bars style={{ width: 28, height: 28 }} />
        </div>
        <h1>GitHub Stats</h1>
        <p>Engineering Analytics, Reimagined</p>
        <p style={{ fontSize: 13, color: "var(--muted)", margin: "-12px 0 24px" }}>
          Gain rich insights into your repositories. Monitor review latency, issue lifetimes, and code churn without GitHub premium limits.
        </p>
        <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
          <a
            href="/auth/github"
            className="btn primary"
            style={{ height: 42, display: "flex", gap: 8, justifyContent: "center" }}
          >
            <I.github style={{ width: 18, height: 18 }} />
            Sign in with GitHub
          </a>
          <span style={{ fontSize: 11, color: "var(--muted)" }}>
            Secure local cookie-based session callbacks.
          </span>
        </div>
      </div>
    </div>
  );
}

interface RepoDetailWrapperProps {
  resolve: (owner: string, repo: string) => Repo | null;
  onBack: () => void;
}

function RepoDetailWrapper({ resolve, onBack }: RepoDetailWrapperProps) {
  const { owner = "", repo = "" } = useParams();
  const matched = resolve(owner, repo);

  if (!matched) {
    return (
      <div className="page fade-in" style={{ textAlign: "center", padding: 80 }}>
        <h2>Repository Not Found</h2>
        <p className="muted">The repository {owner}/{repo} is not tracked.</p>
        <button className="btn primary" onClick={onBack}>
          Back to Dashboard
        </button>
      </div>
    );
  }

  return <RepoDetail repo={matched} onBack={onBack} />;
}

export default function App() {
  // Initialize the in-memory theme from the persisted choice (theme.ts) so the
  // tweaks state, the header ThemeToggle, and localStorage all agree on mount.
  const [tweaks, setTweak] = useState({ ...TWEAK_DEFAULTS, theme: getTheme() });
  const meState = useAsync<Me | null>(fetchMe, []);
  const reposApi = useRepos();
  const collectionsApi = useCollections();
  const repos = reposApi.repos;
  const collections = collectionsApi.collections;
  const [showTweaks, setShowTweaks] = useState(false);

  const navigate = useNavigate();
  const location = useLocation();

  // Apply tweaks dynamically to document root element. Note: `data-theme` is
  // owned by theme.ts / <ThemeToggle> (the single persisted source of truth),
  // so it is intentionally NOT set here to avoid two controls fighting over it.
  useEffect(() => {
    const root = document.documentElement;
    root.setAttribute("data-density", tweaks.density);
    root.style.setProperty("--accent", tweaks.accent);
    root.style.setProperty("--accent-fg", accentFg(tweaks.accent));
    root.style.setProperty("--radius", tweaks.radius + "px");
    root.style.setProperty(
      "--font-sans",
      FONTS[tweaks.font as keyof typeof FONTS] || FONTS.Geist,
    );
  }, [tweaks]);

  const updateTweak = (key: keyof typeof TWEAK_DEFAULTS, val: any) => {
    setTweak((prev) => ({ ...prev, [key]: val }));
  };

  const handleOpenRepo = (repo: Repo) => {
    navigate(`/${repo.full_name}`);
    window.scrollTo(0, 0);
  };

  const handleAddRepo = async (fullName: string) => {
    const created = await reposApi.add(fullName);
    handleOpenRepo(created);
  };

  const handleAddCollection = async (name: string) => {
    await collectionsApi.create(name);
  };

  if (meState.loading) return <div className="app-loading">Loading…</div>;
  const me = meState.data;
  if (!me) return <SignIn />;

  const path = location.pathname;
  const isOverview = path === "/";
  const isCollections = path === "/collections";
  const isInsights = path === "/insights";

  return (
    <div className="app">
      <header className="topbar">
        <div className="brand" onClick={() => navigate("/")} style={{ cursor: "pointer" }}>
          <span className="logo">
            <I.bars style={{ width: 17, height: 17 }} />
          </span>
          GitHub Stats
        </div>
        <nav className="nav">
          <Link to="/" className={isOverview || (!isCollections && !isInsights) ? "active" : ""}>
            Repositories
          </Link>
          <Link to="/collections" className={isCollections ? "active" : ""}>
            Collections
          </Link>
          <Link to="/insights" className={isInsights ? "active" : ""}>
            Insights
          </Link>
        </nav>
        <span className="spacer" />
        <button
          className="btn ghost icon"
          onClick={() => setShowTweaks((o) => !o)}
          title="Open Tweaks"
        >
          <I.sparkles style={{ width: 17, height: 17 }} />
        </button>
        <button className="btn ghost icon" title="Notifications">
          <I.bell style={{ width: 17, height: 17 }} />
        </button>
        <ThemeToggle
          value={tweaks.theme as Theme}
          onChange={(t) => updateTweak("theme", t)}
        />
        <UserMenu me={me} />
      </header>

      <Routes>
        <Route
          path="/"
          element={
            <Overview repos={repos} onOpen={handleOpenRepo} onAdd={handleAddRepo} />
          }
        />
        <Route
          path="/collections"
          element={
            <Collections
              repos={repos}
              collections={collections}
              onOpenRepo={handleOpenRepo}
              onCreate={handleAddCollection}
            />
          }
        />
        <Route
          path="/insights"
          element={<WorkspaceInsights repos={repos} onOpen={handleOpenRepo} />}
        />
        <Route path="/settings" element={<Settings />} />
        <Route
          path="/:owner/:repo"
          element={
            <RepoDetailWrapper resolve={reposApi.resolve} onBack={() => navigate("/")} />
          }
        />
      </Routes>

      <TweaksPanel
        title="Tweaks"
        open={showTweaks}
        onClose={() => setShowTweaks(false)}
      >
        <TweakSection label="Brand" />
        <TweakColor
          label="Accent"
          value={tweaks.accent}
          options={ACCENTS}
          onChange={(v) => updateTweak("accent", v)}
        />
        <TweakRadio
          label="Theme"
          value={tweaks.theme}
          options={["light", "dark"]}
          onChange={(v) => {
            // Persist + apply via theme.ts so the radio, the header toggle, and
            // localStorage stay in lockstep (one source of truth).
            setTheme(v as Theme);
            updateTweak("theme", v);
          }}
        />
        <TweakSection label="Typography" />
        <TweakRadio
          label="Font"
          value={tweaks.font}
          options={["Geist", "Inter", "System"]}
          onChange={(v) => updateTweak("font", v)}
        />
        <TweakSection label="Layout" />
        <TweakRadio
          label="Density"
          value={tweaks.density}
          options={["compact", "balanced", "spacious"]}
          onChange={(v) => updateTweak("density", v)}
        />
        <TweakSlider
          label="Corner radius"
          value={tweaks.radius}
          min={0}
          max={16}
          step={1}
          unit="px"
          onChange={(v) => updateTweak("radius", v)}
        />
      </TweaksPanel>
    </div>
  );
}
