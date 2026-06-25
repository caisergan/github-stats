// Pure formatting helpers. `now` is injectable for deterministic tests.

const MONTHS = [
  "Jan", "Feb", "Mar", "Apr", "May", "Jun",
  "Jul", "Aug", "Sep", "Oct", "Nov", "Dec",
];

const MONTHS_LONG = [
  "January", "February", "March", "April", "May", "June",
  "July", "August", "September", "October", "November", "December",
];

/** "2026-05-09" -> "May 9, 2026". Falls back to the raw string if unparseable. */
export function fmtDate(iso: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(iso);
  if (!m) return iso;
  const year = Number(m[1]);
  const month = Number(m[2]) - 1;
  const day = Number(m[3]);
  if (month < 0 || month > 11) return iso;
  return `${MONTHS_LONG[month]} ${day}, ${year}`;
}

/** Short month-day for chart axes: "2026-05-09" -> "May 9". */
export function fmtDateShort(iso: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(iso);
  if (!m) return iso;
  const month = Number(m[2]) - 1;
  const day = Number(m[3]);
  if (month < 0 || month > 11) return iso;
  return `${MONTHS[month]} ${day}`;
}

/** Coarse relative time: "just now", "5m ago", "3h ago", "2d ago", "4mo ago". */
export function fmtRelative(iso: string, now: number = Date.now()): string {
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return iso;
  const secs = Math.max(0, Math.floor((now - then) / 1000));
  if (secs < 60) return "just now";
  const mins = Math.floor(secs / 60);
  if (mins < 60) return `${mins}m ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  const days = Math.floor(hrs / 24);
  if (days < 30) return `${days}d ago`;
  const months = Math.floor(days / 30);
  if (months < 12) return `${months}mo ago`;
  return `${Math.floor(months / 12)}y ago`;
}

/** Hours float -> "12.5h"; switches to days at 48h: "3.0d". */
export function fmtHours(hours: number): string {
  if (!Number.isFinite(hours)) return "—";
  if (hours >= 48) return `${(hours / 24).toFixed(1)}d`;
  const rounded = Math.round(hours * 10) / 10;
  return Number.isInteger(rounded) ? `${rounded}h` : `${rounded.toFixed(1)}h`;
}

/** Per-day rate -> "2.4/day". */
export function fmtRate(perDay: number): string {
  if (!Number.isFinite(perDay)) return "—";
  const rounded = Math.round(perDay * 10) / 10;
  return Number.isInteger(rounded) ? `${rounded}/day` : `${rounded.toFixed(1)}/day`;
}

/** Integer with thousands separators. */
export function fmtNumber(n: number): string {
  return Math.round(n).toLocaleString("en-US");
}

/** string|null timestamp -> relative time, or "never" when null/empty. */
export function fmtNullableTs(iso: string | null, now: number = Date.now()): string {
  if (!iso) return "never";
  return fmtRelative(iso, now);
}

/** Deterministic GitHub avatar URL for a login (the API omits avatar URLs on
 *  leaderboard/latest rows, so we derive them client-side). */
export function avatarURL(login: string): string {
  return `https://avatars.githubusercontent.com/${login}?size=48`;
}

/** Split a "owner/name" full_name into its parts. Extra slashes stay in `name`. */
export function splitRepo(fullName: string): { owner: string; name: string } {
  const i = fullName.indexOf("/");
  if (i < 0) return { owner: fullName, name: "" };
  return { owner: fullName.slice(0, i), name: fullName.slice(i + 1) };
}
