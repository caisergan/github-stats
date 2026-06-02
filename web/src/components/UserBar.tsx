import { GitBranch, Home, LogOut, Github } from "lucide-react";
import type { Me } from "../api";
import { Link, useLocation } from "react-router-dom";

interface Props {
  me: Me;
}

export default function UserBar({ me }: Props) {
  const location = useLocation();
  const isHome = location.pathname === "/";

  return (
    <aside className="w-64 shrink-0 bg-surface border-r border-border flex flex-col min-h-screen">
      {/* Brand Logo Header */}
      <div className="h-16 px-6 border-b border-border flex items-center gap-3">
        <div className="w-8 h-8 rounded-lg bg-accent/10 border border-accent/30 flex items-center justify-center text-accent shadow-[0_0_10px_rgba(47,129,247,0.2)]">
          <GitBranch size={18} className="animate-pulse" />
        </div>
        <span className="text-sm font-bold tracking-wide text-text">GitHub Stats</span>
      </div>

      {/* Navigation Menu */}
      <nav className="flex-1 py-6 px-4 space-y-1">
        <Link
          to="/"
          className={`flex items-center gap-3 px-3 py-2 rounded-lg text-sm font-medium transition-all duration-200 ${
            isHome
              ? "bg-accent/10 text-accent border border-accent/20"
              : "text-muted hover:text-text hover:bg-surface-hover border border-transparent"
          }`}
        >
          <Home size={18} />
          <span>Dashboard</span>
        </Link>
        <a
          href="https://github.com"
          target="_blank"
          rel="noopener noreferrer"
          className="flex items-center gap-3 px-3 py-2 rounded-lg text-sm font-medium text-muted hover:text-text hover:bg-surface-hover border border-transparent transition-all duration-200"
        >
          <Github size={18} />
          <span>GitHub Web</span>
        </a>
      </nav>

      {/* Bottom User Profile Section */}
      <div className="p-4 border-t border-border bg-bg/50">
        <div className="flex items-center gap-3 p-2 rounded-xl border border-border bg-surface/50">
          {me.avatar_url ? (
            <img src={me.avatar_url} alt="" className="w-9 h-9 rounded-full border border-border" />
          ) : (
            <div className="w-9 h-9 rounded-full bg-accent/20 flex items-center justify-center text-accent font-bold">
              {me.login[0].toUpperCase()}
            </div>
          )}
          <div className="flex-1 min-w-0">
            <p className="text-xs font-semibold text-text truncate">{me.login}</p>
            <p className="text-[10px] text-muted truncate">Active Session</p>
          </div>
          <a
            href="/auth/logout"
            className="w-7 h-7 rounded-lg border border-border hover:border-red hover:text-red flex items-center justify-center text-muted transition-all duration-200"
            title="Sign out"
          >
            <LogOut size={14} />
          </a>
        </div>
      </div>
    </aside>
  );
}
