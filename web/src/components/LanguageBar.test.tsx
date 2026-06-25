import { describe, it, expect } from "vitest";
import { topTwoPlusOther } from "./LanguageBar";

describe("topTwoPlusOther", () => {
  it("keeps the top 2 by size and aggregates the rest as Other", () => {
    const segs = topTwoPlusOther([
      { name: "Go", color: "#00ADD8", size: 80 },
      { name: "CSS", color: "#563d7c", size: 5 },
      { name: "TypeScript", color: "#2b7489", size: 15 },
    ]);
    expect(segs.map((s) => s.name)).toEqual(["Go", "TypeScript", "Other"]);
    expect(Math.round(segs[0].pct)).toBe(80);
    expect(Math.round(segs[1].pct)).toBe(15);
    expect(Math.round(segs[2].pct)).toBe(5);
  });

  it("omits Other when there are at most 2 languages", () => {
    const segs = topTwoPlusOther([
      { name: "Go", color: "#00ADD8", size: 70 },
      { name: "HTML", color: "#e34c26", size: 30 },
    ]);
    expect(segs.map((s) => s.name)).toEqual(["Go", "HTML"]);
  });

  it("returns [] for empty or zero-byte input", () => {
    expect(topTwoPlusOther([])).toEqual([]);
    expect(topTwoPlusOther([{ name: "X", color: "", size: 0 }])).toEqual([]);
  });
});
