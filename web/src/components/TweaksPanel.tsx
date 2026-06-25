import React, { useState, useRef, useEffect, useCallback, ReactNode } from "react";

export interface TweakValues {
  accent: string;
  theme: string;
  font: string;
  radius: number;
  density: string;
}

interface TweakOptionItem {
  value: string;
  label: string;
}

export function useTweaks(
  defaults: TweakValues,
): [TweakValues, (key: keyof TweakValues, val: any) => void] {
  const [values, setValues] = useState<TweakValues>(defaults);

  const setTweak = useCallback((key: keyof TweakValues, val: any) => {
    setValues((prev) => ({ ...prev, [key]: val }));
    // Dispatch local event for stage peers to react
    window.dispatchEvent(
      new CustomEvent("tweakchange", { detail: { [key]: val } }),
    );
  }, []);

  return [values, setTweak];
}

interface TweaksPanelProps {
  title?: string;
  children: ReactNode;
  open: boolean;
  onClose: () => void;
}

export function TweaksPanel({
  title = "Tweaks",
  children,
  open,
  onClose,
}: TweaksPanelProps) {
  const dragRef = useRef<HTMLDivElement>(null);
  const offsetRef = useRef({ x: 16, y: 16 });
  const PAD = 16;

  const clampToViewport = useCallback(() => {
    const panel = dragRef.current;
    if (!panel) return;
    const w = panel.offsetWidth;
    const h = panel.offsetHeight;
    const maxRight = Math.max(PAD, window.innerWidth - w - PAD);
    const maxBottom = Math.max(PAD, window.innerHeight - h - PAD);
    offsetRef.current = {
      x: Math.min(maxRight, Math.max(PAD, offsetRef.current.x)),
      y: Math.min(maxBottom, Math.max(PAD, offsetRef.current.y)),
    };
    panel.style.right = offsetRef.current.x + "px";
    panel.style.bottom = offsetRef.current.y + "px";
  }, []);

  useEffect(() => {
    if (!open) return;
    clampToViewport();
    if (typeof ResizeObserver === "undefined") {
      window.addEventListener("resize", clampToViewport);
      return () => window.removeEventListener("resize", clampToViewport);
    }
    const ro = new ResizeObserver(clampToViewport);
    ro.observe(document.documentElement);
    return () => ro.disconnect();
  }, [open, clampToViewport]);

  const onDragStart = (e: React.MouseEvent<HTMLDivElement>) => {
    const panel = dragRef.current;
    if (!panel) return;
    const r = panel.getBoundingClientRect();
    const sx = e.clientX;
    const sy = e.clientY;
    const startRight = window.innerWidth - r.right;
    const startBottom = window.innerHeight - r.bottom;

    const move = (ev: MouseEvent) => {
      offsetRef.current = {
        x: startRight - (ev.clientX - sx),
        y: startBottom - (ev.clientY - sy),
      };
      clampToViewport();
    };

    const up = () => {
      window.removeEventListener("mousemove", move);
      window.removeEventListener("mouseup", up);
    };

    window.addEventListener("mousemove", move);
    window.addEventListener("mouseup", up);
  };

  if (!open) return null;

  return (
    <div
      ref={dragRef}
      className="twk-panel"
      data-omelette-chrome=""
      style={{
        right: offsetRef.current.x,
        bottom: offsetRef.current.y,
        position: "fixed",
        zIndex: 2147483646,
        width: 280,
        display: "flex",
        flexDirection: "column",
        background: "rgba(250,250,250,0.85)",
        color: "#18181b",
        backdropFilter: "blur(24px) saturate(160%)",
        border: "0.5px solid rgba(0,0,0,0.1)",
        borderRadius: 14,
        boxShadow: "0 12px 40px rgba(0,0,0,0.18)",
        fontFamily: "ui-sans-serif, system-ui, -apple-system, sans-serif",
        fontSize: 11.5,
        overflow: "hidden",
      }}
    >
      <div
        className="twk-hd"
        onMouseDown={onDragStart}
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          padding: "10px 8px 10px 14px",
          cursor: "move",
          userSelect: "none",
        }}
      >
        <b style={{ fontSize: 12, fontWeight: 600 }}>{title}</b>
        <button
          className="twk-x"
          aria-label="Close tweaks"
          onMouseDown={(e) => e.stopPropagation()}
          onClick={onClose}
          style={{
            appearance: "none",
            border: 0,
            background: "transparent",
            color: "rgba(0,0,0,0.45)",
            width: 22,
            height: 22,
            borderRadius: 6,
            cursor: "pointer",
            fontSize: 13,
            lineHeight: 1,
          }}
        >
          ✕
        </button>
      </div>
      <div
        className="twk-body"
        style={{
          padding: "2px 14px 14px",
          display: "flex",
          flexDirection: "column",
          gap: 10,
          overflowY: "auto",
          maxHeight: "380px",
        }}
      >
        {children}
      </div>
    </div>
  );
}

export function TweakSection({
  label,
  children,
}: {
  label: string;
  children?: ReactNode;
}) {
  return (
    <>
      <div
        className="twk-sect"
        style={{
          fontSize: 10,
          fontWeight: 600,
          letterSpacing: ".06em",
          textTransform: "uppercase",
          color: "rgba(0,0,0,0.45)",
          padding: "10px 0 0",
        }}
      >
        {label}
      </div>
      {children}
    </>
  );
}

export function TweakRow({
  label,
  value,
  children,
  inline = false,
}: {
  label: string;
  value?: string | number;
  children: ReactNode;
  inline?: boolean;
}) {
  return (
    <div
      className={inline ? "twk-row twk-row-h" : "twk-row"}
      style={{
        display: "flex",
        flexDirection: inline ? "row" : "column",
        alignItems: inline ? "center" : "stretch",
        justifyContent: inline ? "space-between" : "stretch",
        gap: inline ? 10 : 5,
      }}
    >
      <div
        className="twk-lbl"
        style={{
          display: "flex",
          justifyContent: "space-between",
          alignItems: "baseline",
          color: "rgba(0,0,0,0.72)",
        }}
      >
        <span style={{ fontWeight: 500 }}>{label}</span>
        {value != null && (
          <span className="twk-val" style={{ color: "rgba(0,0,0,0.5)" }}>
            {value}
          </span>
        )}
      </div>
      {children}
    </div>
  );
}

interface TweakSliderProps {
  label: string;
  value: number;
  min?: number;
  max?: number;
  step?: number;
  unit?: string;
  onChange: (val: number) => void;
}

export function TweakSlider({
  label,
  value,
  min = 0,
  max = 100,
  step = 1,
  unit = "",
  onChange,
}: TweakSliderProps) {
  return (
    <TweakRow label={label} value={`${value}${unit}`}>
      <input
        type="range"
        className="twk-slider"
        min={min}
        max={max}
        step={step}
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        style={{
          appearance: "none",
          width: "100%",
          height: 4,
          margin: "6px 0",
          borderRadius: 999,
          background: "rgba(0,0,0,0.12)",
          outline: "none",
        }}
      />
    </TweakRow>
  );
}

interface TweakRadioProps {
  label: string;
  value: string;
  options: (string | TweakOptionItem)[];
  onChange: (val: string) => void;
}

export function TweakRadio({
  label,
  value,
  options,
  onChange,
}: TweakRadioProps) {
  const opts = options.map((o) =>
    typeof o === "object" ? o : { value: o, label: o },
  );
  const idx = Math.max(
    0,
    opts.findIndex((o) => o.value === value),
  );
  const n = opts.length;

  return (
    <TweakRow label={label}>
      <div
        className="twk-seg"
        role="radiogroup"
        style={{
          position: "relative",
          display: "flex",
          padding: 2,
          borderRadius: 8,
          background: "rgba(0,0,0,0.06)",
          userSelect: "none",
        }}
      >
        <div
          className="twk-seg-thumb"
          style={{
            position: "absolute",
            top: 2,
            bottom: 2,
            borderRadius: 6,
            background: "rgba(255,255,255,0.9)",
            boxShadow: "0 1px 2px rgba(0,0,0,0.12)",
            transition: "left .15s cubic-bezier(.3,.7,.4,1), width .15s",
            left: `calc(2px + ${idx} * (100% - 4px) / ${n})`,
            width: `calc((100% - 4px) / ${n})`,
          }}
        />
        {opts.map((o) => (
          <button
            key={o.value}
            type="button"
            role="radio"
            aria-checked={o.value === value}
            onClick={() => onChange(o.value)}
            style={{
              appearance: "none",
              position: "relative",
              zIndex: 1,
              flex: 1,
              border: 0,
              background: "transparent",
              color: "inherit",
              font: "inherit",
              fontWeight: 500,
              minHeight: 22,
              borderRadius: 6,
              cursor: "pointer",
              padding: "4px 6px",
              lineHeight: 1.2,
            }}
          >
            {o.label}
          </button>
        ))}
      </div>
    </TweakRow>
  );
}

interface TweakColorProps {
  label: string;
  value: string;
  options: string[];
  onChange: (val: string) => void;
}

export function TweakColor({
  label,
  value,
  options,
  onChange,
}: TweakColorProps) {
  return (
    <TweakRow label={label}>
      <div className="twk-chips" style={{ display: "flex", gap: 6 }}>
        {options.map((o, i) => {
          const on = o === value;
          return (
            <button
              key={i}
              type="button"
              className="twk-chip"
              role="radio"
              aria-checked={on}
              data-on={on ? "1" : "0"}
              aria-label={o}
              title={o}
              style={{
                position: "relative",
                appearance: "none",
                flex: 1,
                minWidth: 0,
                height: 32,
                borderRadius: 6,
                cursor: "pointer",
                background: o,
                border: "none",
                boxShadow: on
                  ? "0 0 0 2px #fff, 0 0 0 4px var(--fg)"
                  : "0 0 0 0.5px rgba(0,0,0,0.12)",
              }}
              onClick={() => onChange(o)}
            >
              {on && (
                <svg
                  viewBox="0 0 14 14"
                  aria-hidden="true"
                  style={{
                    position: "absolute",
                    top: "50%",
                    left: "50%",
                    transform: "translate(-50%, -50%)",
                    width: 14,
                    height: 14,
                  }}
                >
                  <path
                    d="M3 7.2 5.8 10 11 4.2"
                    fill="none"
                    strokeWidth="2.2"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    stroke={
                      o === "#ffffff" || o === "#fafafa" ? "#000" : "#fff"
                    }
                  />
                </svg>
              )}
            </button>
          );
        })}
      </div>
    </TweakRow>
  );
}
