import React, { useState, useRef, useEffect, useId, ReactNode } from "react";
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

/**
 * A custom dropdown that replaces the native <select>. The native control can't
 * style its open option list (it falls back to the OS popup), so this renders a
 * design-system popover instead. Drop-in: same {value, options, onChange} props.
 * Closes on outside click / Escape, supports arrow-key navigation, and marks the
 * current value with a check — matching the app's Menu popover conventions.
 */
export function Select({ value, options, onChange }: SelectProps) {
  const opts = options.map((o) =>
    typeof o === "string" ? { value: o, label: o } : o,
  );
  const selected = opts.find((o) => o.value === value);
  const selectedIdx = opts.findIndex((o) => o.value === value);

  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(selectedIdx);
  const ref = useRef<HTMLSpanElement>(null);
  const baseId = useId();
  const optId = (i: number) => `${baseId}-opt-${i}`;

  // Close on outside click (same pattern as Menu).
  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, [open]);

  // Highlight the current value each time the menu opens.
  useEffect(() => {
    if (open) setActive(selectedIdx >= 0 ? selectedIdx : 0);
  }, [open, selectedIdx]);

  const choose = (val: string) => {
    onChange(val);
    setOpen(false);
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Escape") {
      setOpen(false);
    } else if (!open && ["ArrowDown", "ArrowUp", "Enter", " "].includes(e.key)) {
      e.preventDefault();
      setOpen(true);
    } else if (open && e.key === "ArrowDown") {
      e.preventDefault();
      setActive((a) => Math.min(opts.length - 1, a + 1));
    } else if (open && e.key === "ArrowUp") {
      e.preventDefault();
      setActive((a) => Math.max(0, a - 1));
    } else if (open && (e.key === "Enter" || e.key === " ")) {
      e.preventDefault();
      if (active >= 0 && active < opts.length) choose(opts[active].value);
    }
  };

  return (
    <span className="dropdown" ref={ref}>
      <button
        type="button"
        className={"dd-trigger" + (open ? " open" : "")}
        onClick={() => setOpen((o) => !o)}
        onKeyDown={onKeyDown}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-activedescendant={open && active >= 0 ? optId(active) : undefined}
      >
        <span className="dd-value">{selected ? selected.label : ""}</span>
        <I.chevDown className="dd-chev" style={{ width: 14, height: 14 }} />
      </button>
      {open && (
        <div className="dd-menu fade-in" role="listbox">
          {opts.map((o, i) => (
            <button
              key={o.value}
              id={optId(i)}
              type="button"
              role="option"
              aria-selected={o.value === value}
              className={"dd-opt" + (i === active ? " active" : "")}
              onClick={() => choose(o.value)}
              onMouseEnter={() => setActive(i)}
            >
              {o.value === value && (
                <I.check className="dd-check" style={{ width: 14, height: 14 }} />
              )}
              {o.label}
            </button>
          ))}
        </div>
      )}
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
