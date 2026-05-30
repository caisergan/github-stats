export interface Me {
  id: number;
  github_id: number;
  login: string;
  avatar_url: string;
}

export async function fetchMe(): Promise<Me | null> {
  const res = await fetch("/api/me", { credentials: "same-origin" });
  if (res.status === 401) return null;
  if (!res.ok) throw new Error(`/api/me failed: ${res.status}`);
  return (await res.json()) as Me;
}
