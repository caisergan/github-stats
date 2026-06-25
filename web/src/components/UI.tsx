import React, { useState, useRef, useEffect, ReactNode } from "react";
import { I } from "./Icons";

export interface OptionItem {
  value: string;
  label: string;
}

interface SegmentedProps {
  value: string;
  options: (string | OptionItem)[];
  onChange: (val: string) => void;
}

export function Segmented({ value, options, onChange }: SegmentedProps) {
  return (
    <div className="segmented" role="tablist">
      {options.map((o) => {
        const val = typeof o === "string" ? o : o.value;
        const label = typeof o === "string" ? o : o.label;
        return (
          <button
            key={val}
            className={val === value ? "active" : ""}
            onClick={() => onChange(val)}
            role="tab"
            aria-selected={val === value}
          >
            {label}
          </button>
        );
      })}
    </div>
  );
}

interface SwitchProps {
  checked: boolean;
  onChange: (val: boolean) => void;
  label?: string;
}

export function Switch({ checked, onChange, label }: SwitchProps) {
  const toggle = () => onChange(!checked);
  return (
    <span
      className="switch"
      onClick={toggle}
      role="switch"
      aria-checked={checked}
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.key === " " || e.key === "Enter") {
          e.preventDefault();
          toggle();
        }
      }}
    >
      <span className={"track" + (checked ? " on" : "")}>
        <span className="knob" />
      </span>
      {label && <span>{label}</span>}
    </span>
  );
}

interface SelectProps {
  value: string;
  options: (string | OptionItem)[];
  onChange: (val: string) => void;
}

export function Select({ value, options, onChange }: SelectProps) {
  return (
    <span className="select">
      <select value={value} onChange={(e) => onChange(e.target.value)}>
        {options.map((o) => {
          const val = typeof o === "string" ? o : o.value;
          const label = typeof o === "string" ? o : o.label;
          return (
            <option key={val} value={val}>
              {label}
            </option>
          );
        })}
      </select>
      <I.chevDown className="chev" style={{ width: 14, height: 14 }} />
    </span>
  );
}

interface BadgeProps {
  tone?: string;
  children: ReactNode;
  dot?: boolean;
  pulse?: boolean;
}

export function Badge({ tone, children, dot, pulse }: BadgeProps) {
  return (
    <span className={"badge" + (tone ? " " + tone : "")}>
      {dot && <span className={"dot" + (pulse ? " pulse" : "")} />}
      {children}
    </span>
  );
}

const SYNC_MAP: Record<string, { tone: string; label: string; pulse: boolean }> = {
  complete: { tone: "green", label: "Synced", pulse: false },
  running: { tone: "blue", label: "Syncing", pulse: true },
  pending: { tone: "amber", label: "Queued", pulse: true },
  error: { tone: "red", label: "Error", pulse: false },
};

export function SyncStatusBadge({ status }: { status: string }) {
  const s = SYNC_MAP[status] || { tone: "", label: status || "Unknown", pulse: false };
  return (
    <Badge tone={s.tone} dot pulse={s.pulse}>
      {s.label}
    </Badge>
  );
}

interface MenuProps {
  trigger: ReactNode;
  children: ReactNode;
  align?: "right" | "left";
}

export function Menu({ trigger, children, align = "right" }: MenuProps) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLSpanElement>(null);

  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, [open]);

  return (
    <span className="menu-wrap" ref={ref}>
      <span
        onClick={(e) => {
          e.stopPropagation();
          setOpen((o) => !o);
        }}
      >
        {trigger}
      </span>
      {open && (
        <div
          className="menu fade-in"
          style={align === "right" ? { right: 0 } : { left: 0 }}
          onClick={() => setOpen(false)}
        >
          {children}
        </div>
      )}
    </span>
  );
}

interface LangDotProps {
  name: string;
  color: string;
}

export function LangDot({ name, color }: LangDotProps) {
  return (
    <span className="lang">
      <span className="d" style={{ background: color }} />
      {name}
    </span>
  );
}
