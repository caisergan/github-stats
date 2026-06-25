import { useEffect, useState } from "react";
import { Sun, Moon } from "lucide-react";
import { getTheme, setTheme, type Theme } from "../theme";

interface Props {
  // Optional controlled mode: when provided, the parent owns the theme state
  // (e.g. App's tweaks). The toggle still persists + applies via theme.ts so
  // there is one coherent source of truth and no competing data-theme writes.
  value?: Theme;
  onChange?: (t: Theme) => void;
}

// ThemeToggle is the single source of truth for the persisted light/dark theme.
// Uncontrolled: reads/writes theme.ts (localStorage + `data-theme` on <html>).
// Controlled: reflects `value` and calls `onChange`, while still persisting.
export function ThemeToggle({ value, onChange }: Props = {}) {
  const [local, setLocal] = useState<Theme>(() => value ?? getTheme());
  const theme = value ?? local;

  // Re-apply on mount/whenever the active theme changes so the in-memory state
  // and the DOM attribute agree even if another control touched `data-theme`.
  useEffect(() => {
    setTheme(theme);
  }, [theme]);

  const toggle = () => {
    const next: Theme = theme === "dark" ? "light" : "dark";
    setTheme(next);
    setLocal(next);
    onChange?.(next);
  };

  return (
    <button
      type="button"
      className="btn ghost icon theme-toggle"
      onClick={toggle}
      title="Toggle theme"
      aria-label="Toggle theme"
    >
      {theme === "dark" ? (
        <Sun style={{ width: 17, height: 17 }} />
      ) : (
        <Moon style={{ width: 17, height: 17 }} />
      )}
    </button>
  );
}
