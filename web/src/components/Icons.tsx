import React, { ComponentPropsWithoutRef } from "react";

interface IconProps extends ComponentPropsWithoutRef<"svg"> {
  d?: string;
  fill?: string;
  vb?: string;
  sw?: number;
}

export const Icon = ({
  d,
  children,
  fill,
  vb = "0 0 24 24",
  sw = 2,
  ...rest
}: IconProps) => (
  <svg
    viewBox={vb}
    fill={fill || "none"}
    stroke={fill ? "none" : "currentColor"}
    strokeWidth={sw}
    strokeLinecap="round"
    strokeLinejoin="round"
    aria-hidden="true"
    {...rest}
  >
    {children || (d ? <path d={d} /> : null)}
  </svg>
);

export const I = {
  bars: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <path d="M3 21V10M9 21V4M15 21v-8M21 21V7" />
    </Icon>
  ),
  star: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <path d="M12 2.5l2.9 5.9 6.5.9-4.7 4.6 1.1 6.5L12 17.8 6.2 20.9l1.1-6.5L2.6 9.8l6.5-.9z" />
    </Icon>
  ),
  fork: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <circle cx="6" cy="5" r="2.4" />
      <circle cx="18" cy="5" r="2.4" />
      <circle cx="12" cy="19" r="2.4" />
      <path d="M6 7.4v3c0 1.5 1.2 2.6 2.6 2.6h6.8c1.4 0 2.6-1.1 2.6-2.6v-3M12 13v3.6" />
    </Icon>
  ),
  issue: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <circle cx="12" cy="12" r="9" />
      <circle cx="12" cy="12" r="2.2" />
    </Icon>
  ),
  pr: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <circle cx="6" cy="6" r="2.4" />
      <circle cx="6" cy="18" r="2.4" />
      <circle cx="18" cy="18" r="2.4" />
      <path d="M6 8.4v7.2M18 15.6V12a4 4 0 0 0-4-4h-3m0 0 2.5-2.5M11 8.4 13.5 11" />
    </Icon>
  ),
  commit: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <circle cx="12" cy="12" r="3.2" />
      <path d="M12 2v6.8M12 15.2V22" />
    </Icon>
  ),
  repo: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <path d="M5 4.5A1.5 1.5 0 0 1 6.5 3H19v15H6.5A1.5 1.5 0 0 0 5 19.5zM5 19.5A1.5 1.5 0 0 0 6.5 21H19v-3" />
    </Icon>
  ),
  lock: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <rect x="5" y="11" width="14" height="9" rx="2" />
      <path d="M8 11V8a4 4 0 0 1 8 0v3" />
    </Icon>
  ),
  globe: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <circle cx="12" cy="12" r="9" />
      <path d="M3 12h18M12 3c2.6 2.5 2.6 15.5 0 18M12 3c-2.6 2.5-2.6 15.5 0 18" />
    </Icon>
  ),
  search: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <circle cx="11" cy="11" r="7" />
      <path d="m20 20-3.2-3.2" />
    </Icon>
  ),
  chevDown: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <path d="m6 9 6 6 6-6" />
    </Icon>
  ),
  chevRight: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <path d="m9 6 6 6-6 6" />
    </Icon>
  ),
  chevLeft: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <path d="m15 6-6 6 6 6" />
    </Icon>
  ),
  refresh: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <path d="M21 12a9 9 0 1 1-2.6-6.4M21 4v4h-4" />
    </Icon>
  ),
  plus: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <path d="M12 5v14M5 12h14" />
    </Icon>
  ),
  sun: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <circle cx="12" cy="12" r="4" />
      <path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" />
    </Icon>
  ),
  moon: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z" />
    </Icon>
  ),
  users: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <circle cx="9" cy="8" r="3.2" />
      <path d="M3.5 20a5.5 5.5 0 0 1 11 0M16 5.2a3.2 3.2 0 0 1 0 6.1M17 20a5.5 5.5 0 0 0-3-4.9" />
    </Icon>
  ),
  tag: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <path d="M20 12.5 12.5 20a2 2 0 0 1-2.8 0l-6-6a2 2 0 0 1-.6-1.4V5a2 2 0 0 1 2-2h7.6a2 2 0 0 1 1.4.6l6 6a2 2 0 0 1 0 2.9z" />
      <circle cx="8" cy="8" r="1.4" fill="currentColor" stroke="none" />
    </Icon>
  ),
  clock: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7v5l3.5 2" />
    </Icon>
  ),
  activity: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <path d="M3 12h4l3 8 4-16 3 8h4" />
    </Icon>
  ),
  gauge: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <path d="M12 13l4-4M4.5 18a9 9 0 1 1 15 0" />
    </Icon>
  ),
  samples: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <path d="M4 20V13M9 20V8M14 20v-6M19 20V5" />
    </Icon>
  ),
  trendUp: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <path d="m3 17 6-6 4 4 8-8M21 7v5M21 7h-5" />
    </Icon>
  ),
  trendDown: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <path d="m3 7 6 6 4-4 8 8M21 17v-5M21 17h-5" />
    </Icon>
  ),
  arrowUp: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p} sw={2.4}>
      <path d="M12 19V5M6 11l6-6 6 6" />
    </Icon>
  ),
  arrowDown: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p} sw={2.4}>
      <path d="M12 5v14M6 13l6 6 6-6" />
    </Icon>
  ),
  dot3: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p} sw={2.4}>
      <circle cx="5" cy="12" r="1.3" fill="currentColor" stroke="none" />
      <circle cx="12" cy="12" r="1.3" fill="currentColor" stroke="none" />
      <circle cx="19" cy="12" r="1.3" fill="currentColor" stroke="none" />
    </Icon>
  ),
  external: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <path d="M14 4h6v6M20 4l-8 8M18 14v4a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4" />
    </Icon>
  ),
  trash: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <path d="M4 7h16M9 7V5a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2M6 7l1 13a1 1 0 0 0 1 1h8a1 1 0 0 0 1-1l1-13" />
    </Icon>
  ),
  signout: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <path d="M15 4h3a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-3M10 17l5-5-5-5M15 12H3" />
    </Icon>
  ),
  settings: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1.6 1.6 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.6 1.6 0 0 0-2.7 1.1V21a2 2 0 0 1-4 0v-.1A1.6 1.6 0 0 0 7 19.4a1.6 1.6 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.6 1.6 0 0 0-1.1-2.7H1a2 2 0 0 1 0-4h.1A1.6 1.6 0 0 0 2.6 7a1.6 1.6 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.6 1.6 0 0 0 1.8.3H7a1.6 1.6 0 0 0 1-1.5V1a2 2 0 0 1 4 0v.1A1.6 1.6 0 0 0 17 2.6a1.6 1.6 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.6 1.6 0 0 0-.3 1.8V7a1.6 1.6 0 0 0 1.5 1H23a2 2 0 0 1 0 4h-.1a1.6 1.6 0 0 0-1.5 1z" />
    </Icon>
  ),
  check: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p} sw={2.4}>
      <path d="m5 13 4 4 10-10" />
    </Icon>
  ),
  bell: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <path d="M18 8a6 6 0 1 0-12 0c0 7-3 9-3 9h18s-3-2-3-9M13.7 21a2 2 0 0 1-3.4 0" />
    </Icon>
  ),
  filter: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <path d="M3 5h18l-7 8v6l-4-2v-4z" />
    </Icon>
  ),
  comment: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <path d="M21 11.5a8 8 0 0 1-11.5 7.2L3 21l2.3-6.5A8 8 0 1 1 21 11.5z" />
    </Icon>
  ),
  branch: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <circle cx="6" cy="6" r="2.4" />
      <circle cx="6" cy="18" r="2.4" />
      <circle cx="18" cy="7" r="2.4" />
      <path d="M6 8.4v7.2M18 9.4c0 4-3 4.6-6 4.6" />
    </Icon>
  ),
  code: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <path d="m8 9-3 3 3 3M16 9l3 3-3 3M13 6l-2 12" />
    </Icon>
  ),
  download: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <path d="M12 3v12M7 10l5 5 5-5M5 21h14" />
    </Icon>
  ),
  github: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p} fill="currentColor">
      <path d="M12 1.8a10.2 10.2 0 0 0-3.2 19.9c.5.1.7-.2.7-.5v-1.8c-2.8.6-3.4-1.3-3.4-1.3-.5-1.2-1.1-1.5-1.1-1.5-.9-.6.1-.6.1-.6 1 .1 1.5 1 1.5 1 .9 1.6 2.4 1.1 3 .8.1-.6.3-1.1.6-1.4-2.2-.300000-4.6-1.1-4.6-4.9 0-1.1.4-2 1-2.7-.1-.3-.4-1.3.1-2.7 0 0 .8-.3 2.8 1a9.6 9.6 0 0 1 5 0c2-1.3 2.8-1 2.8-1 .5 1.4.2 2.4.1 2.7.6.7 1 1.6 1 2.7 0 3.8-2.3 4.6-4.6 4.9.4.3.7.9.7 1.9v2.8c0 .3.2.6.7.5A10.2 10.2 0 0 0 12 1.8z" />
    </Icon>
  ),
  server: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <rect x="3" y="4" width="18" height="7" rx="2" />
      <rect x="3" y="13" width="18" height="7" rx="2" />
      <path d="M7 7.5h.01M7 16.5h.01" />
    </Icon>
  ),
  layout: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <rect x="3" y="4" width="18" height="16" rx="2" />
      <path d="M3 9h18M9 9v11" />
    </Icon>
  ),
  folder: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <path d="M3 7a2 2 0 0 1 2-2h4l2 2.5h8a2 2 0 0 1 2 2V18a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
    </Icon>
  ),
  layers: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <path d="m12 3 9 5-9 5-9-5 9-5zM3 13l9 5 9-5M3 17l9 5 9-5" />
    </Icon>
  ),
  sparkles: (p: ComponentPropsWithoutRef<"svg">) => (
    <Icon {...p}>
      <path d="M12 4l1.6 4.4L18 10l-4.4 1.6L12 16l-1.6-4.4L6 10l4.4-1.6L12 4zM19 15l.8 2.2L22 18l-2.2.8L19 21l-.8-2.2L16 18l2.2-.8L19 15z" />
    </Icon>
  ),
};
