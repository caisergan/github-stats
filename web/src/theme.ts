// Canonical theme persistence layer (M6). Stores the user's light/dark choice
// in localStorage and applies it as the `data-theme` attribute on <html>
// (document.documentElement) — the same element the existing styles.css keys
// its `[data-theme="dark"]` overrides off of. Default is "dark".
export type Theme = "light" | "dark";
export const THEME_KEY = "gs_theme";

export function getTheme(): Theme {
  const v = localStorage.getItem(THEME_KEY);
  return v === "light" ? "light" : "dark";
}

export function setTheme(t: Theme): void {
  localStorage.setItem(THEME_KEY, t);
  applyTheme(t);
}

export function applyTheme(t: Theme): void {
  document.documentElement.setAttribute("data-theme", t);
}
