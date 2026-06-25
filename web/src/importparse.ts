export type ManifestKind = "package_json" | "requirements_txt" | "collection";

// detectManifestKind classifies a dropped file by name (and content for collection files).
export function detectManifestKind(filename: string, content: string): ManifestKind | null {
  const name = filename.toLowerCase();
  if (name.endsWith(".collection.json")) return "collection";
  if (name === "package.json") {
    return "package_json";
  }
  if (name === "requirements.txt") return "requirements_txt";
  if (name.endsWith(".json")) {
    try {
      const obj = JSON.parse(content);
      if (Array.isArray(obj?.repos) && typeof obj?.name === "string") return "collection";
      if (obj?.dependencies || obj?.devDependencies) return "package_json";
    } catch {
      // fall through
    }
  }
  return null;
}
