import { describe, it, expect } from "vitest";
import { detectManifestKind } from "./importparse";

describe("detectManifestKind", () => {
  it("detects package.json by filename", () => {
    expect(detectManifestKind("package.json", "{}")).toBe("package_json");
  });
  it("detects requirements.txt by filename", () => {
    expect(detectManifestKind("requirements.txt", "flask")).toBe("requirements_txt");
  });
  it("detects an exported collection by content", () => {
    expect(detectManifestKind("backend.collection.json", '{"name":"x","repos":[]}')).toBe("collection");
  });
  it("returns null for unknown", () => {
    expect(detectManifestKind("notes.txt", "hello")).toBeNull();
  });
});
