import type { WindowSpec } from "../api";

interface Props {
  window: WindowSpec;
  excludeBots: boolean;
  onWindow: (w: WindowSpec) => void;
  onExcludeBots: (v: boolean) => void;
}

const WINDOWS: { value: WindowSpec; label: string }[] = [
  { value: "30d", label: "30 days" },
  { value: "90d", label: "90 days" },
  { value: "6m", label: "6 months" },
  { value: "1y", label: "1 year" },
  { value: "all", label: "All time" },
];

export default function WindowControls({ window, excludeBots, onWindow, onExcludeBots }: Props) {
  return (
    <div className="controls">
      <label>
        Window
        <select value={window} onChange={(e) => onWindow(e.target.value as WindowSpec)}>
          {WINDOWS.map((w) => (
            <option key={w.value} value={w.value}>{w.label}</option>
          ))}
        </select>
      </label>
      <label>
        <input
          type="checkbox"
          checked={excludeBots}
          onChange={(e) => onExcludeBots(e.target.checked)}
        />
        Exclude bots
      </label>
    </div>
  );
}
