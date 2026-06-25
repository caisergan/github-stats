import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Select } from "./UI";

const OPTS = [
  { value: "activity", label: "Most active" },
  { value: "stars", label: "Most stars" },
  { value: "name", label: "Name (A–Z)" },
];

describe("Select (custom dropdown)", () => {
  it("shows the selected label and renders no listbox until opened", () => {
    render(<Select value="activity" options={OPTS} onChange={vi.fn()} />);
    expect(screen.getByRole("button", { name: /most active/i })).toBeInTheDocument();
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("opens on click, marks the current value, and selecting fires onChange + closes", async () => {
    const onChange = vi.fn();
    render(<Select value="activity" options={OPTS} onChange={onChange} />);

    await userEvent.click(screen.getByRole("button", { name: /most active/i }));
    expect(screen.getByRole("listbox")).toBeInTheDocument();
    expect(screen.getByRole("option", { name: /most active/i })).toHaveAttribute(
      "aria-selected",
      "true",
    );

    await userEvent.click(screen.getByRole("option", { name: /name/i }));
    expect(onChange).toHaveBeenCalledWith("name");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("closes when clicking outside without selecting", async () => {
    const onChange = vi.fn();
    render(
      <div>
        <Select value="activity" options={OPTS} onChange={onChange} />
        <button>outside</button>
      </div>,
    );
    await userEvent.click(screen.getByRole("button", { name: /most active/i }));
    expect(screen.getByRole("listbox")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "outside" }));
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    expect(onChange).not.toHaveBeenCalled();
  });

  it("supports keyboard: arrow-down + Enter selects the next option", async () => {
    const onChange = vi.fn();
    render(<Select value="activity" options={OPTS} onChange={onChange} />);
    const trigger = screen.getByRole("button", { name: /most active/i });
    trigger.focus();

    await userEvent.keyboard("{ArrowDown}"); // open (active = current = "activity")
    await userEvent.keyboard("{ArrowDown}"); // move to "stars"
    await userEvent.keyboard("{Enter}");
    expect(onChange).toHaveBeenCalledWith("stars");
  });
});
