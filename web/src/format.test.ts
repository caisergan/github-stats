import { describe, it, expect } from "vitest";
import {
  fmtDate,
  fmtRelative,
  fmtHours,
  fmtRate,
  fmtNumber,
  fmtNullableTs,
} from "./format";

const NOW = new Date("2026-05-31T12:00:00Z").getTime();

describe("format", () => {
  it("fmtDate renders a short YYYY-MM-DD-ish label", () => {
    expect(fmtDate("2026-05-09")).toBe("May 9, 2026");
  });

  it("fmtRelative renders coarse buckets", () => {
    expect(fmtRelative("2026-05-31T11:00:00Z", NOW)).toBe("1h ago");
    expect(fmtRelative("2026-05-31T11:59:30Z", NOW)).toBe("just now");
    expect(fmtRelative("2026-05-29T12:00:00Z", NOW)).toBe("2d ago");
  });

  it("fmtHours switches to days past 48h", () => {
    expect(fmtHours(12.5)).toBe("12.5h");
    expect(fmtHours(0)).toBe("0h");
    expect(fmtHours(72)).toBe("3.0d");
  });

  it("fmtRate formats per-day rates", () => {
    expect(fmtRate(2.444)).toBe("2.4/day");
    expect(fmtRate(0)).toBe("0/day");
  });

  it("fmtNumber adds thousands separators", () => {
    expect(fmtNumber(1234567)).toBe("1,234,567");
    expect(fmtNumber(42)).toBe("42");
  });

  it("fmtNullableTs handles null as 'never'", () => {
    expect(fmtNullableTs(null, NOW)).toBe("never");
    expect(fmtNullableTs("2026-05-31T11:00:00Z", NOW)).toBe("1h ago");
  });
});
