import { Calendar, Filter } from "lucide-react";
import type { WindowSpec } from "../api";

interface Props {
  window: WindowSpec;
  excludeBots: boolean;
  onWindow: (w: WindowSpec) => void;
  onExcludeBots: (v: boolean) => void;
}

const WINDOWS: { value: WindowSpec; label: string }[] = [
  { value: "30d", label: "30 Days" },
  { value: "90d", label: "90 Days" },
  { value: "6m", label: "6 Months" },
  { value: "1y", label: "1 Year" },
  { value: "all", label: "All Time" },
];

export default function WindowControls({ window, excludeBots, onWindow, onExcludeBots }: Props) {
  return (
    <div className="flex flex-wrap items-center gap-4 bg-surface/50 border border-border/40 p-3 rounded-xl custom-glass">
      {/* Time Window Dropdown Selector */}
      <div className="flex items-center gap-2">
        <Calendar size={14} className="text-accent" />
        <span className="text-xs font-semibold text-muted">Analysis Window</span>
        <select
          value={window}
          onChange={(e) => onWindow(e.target.value as WindowSpec)}
          className="bg-bg hover:bg-surface-hover/50 text-text text-xs border border-border/80 rounded-lg py-1 pl-2 pr-8 outline-none focus:border-accent transition-all duration-200 cursor-pointer"
        >
          {WINDOWS.map((w) => (
            <option key={w.value} value={w.value}>{w.label}</option>
          ))}
        </select>
      </div>

      <div className="h-4 w-px bg-border/40" />

      {/* Exclude Bots Checkbox Filter */}
      <label className="flex items-center gap-2 cursor-pointer select-none text-xs font-semibold text-muted hover:text-text transition-colors duration-200">
        <div className="relative">
          <input
            type="checkbox"
            checked={excludeBots}
            onChange={(e) => onExcludeBots(e.target.checked)}
            className="sr-only peer"
          />
          <div className="w-8 h-4.5 bg-border rounded-full peer peer-checked:bg-accent transition-all duration-200" />
          <div className="absolute top-0.5 left-0.5 w-3.5 h-3.5 bg-text peer-checked:bg-white rounded-full peer-checked:translate-x-3.5 transition-all duration-200" />
        </div>
        <Filter size={14} className="text-accent shrink-0 ml-1" />
        <span>Exclude bot activities</span>
      </label>
    </div>
  );
}
