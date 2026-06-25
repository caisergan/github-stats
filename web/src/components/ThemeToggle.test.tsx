import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ThemeToggle } from "./ThemeToggle";
import { THEME_KEY } from "../theme";

describe("ThemeToggle", () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.removeAttribute("data-theme");
  });

  it("reflects the persisted theme on mount", () => {
    localStorage.setItem(THEME_KEY, "light");
    render(<ThemeToggle />);
    // Applies the persisted theme to <html>.
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
  });

  it("toggles and persists the theme on click", () => {
    localStorage.setItem(THEME_KEY, "dark");
    render(<ThemeToggle />);
    const btn = screen.getByRole("button", { name: /theme/i });
    fireEvent.click(btn);
    expect(localStorage.getItem(THEME_KEY)).toBe("light");
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
    fireEvent.click(btn);
    expect(localStorage.getItem(THEME_KEY)).toBe("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });
});
