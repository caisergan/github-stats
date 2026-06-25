import { describe, it, expect, beforeEach } from "vitest";
import { getTheme, setTheme, applyTheme, THEME_KEY } from "./theme";

describe("theme", () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.removeAttribute("data-theme");
  });

  it("defaults to dark when nothing is stored", () => {
    expect(getTheme()).toBe("dark");
  });

  it("persists and reads back the chosen theme", () => {
    setTheme("light");
    expect(localStorage.getItem(THEME_KEY)).toBe("light");
    expect(getTheme()).toBe("light");
  });

  it("applyTheme sets the data-theme attribute", () => {
    applyTheme("light");
    expect(document.documentElement.getAttribute("data-theme")).toBe("light");
    applyTheme("dark");
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });
});
