import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import RefreshButton from "./RefreshButton";
import * as api from "../api";

type Fake = {
  onmessage: ((e: MessageEvent) => void) | null;
  onerror: ((e: Event) => void) | null;
  close: ReturnType<typeof vi.fn>;
};

function installEventSource(): Fake {
  const fake: Fake = { onmessage: null, onerror: null, close: vi.fn() };
  vi.stubGlobal("EventSource", vi.fn(() => fake) as unknown as typeof EventSource);
  return fake;
}

function emit(fake: Fake, ev: api.SyncEvent) {
  act(() => {
    fake.onmessage?.({ data: JSON.stringify(ev) } as MessageEvent);
  });
}

describe("RefreshButton", () => {
  beforeEach(() => {
    vi.spyOn(api, "refreshRepo").mockResolvedValue();
  });

  it("triggers a refresh, streams progress, and calls onComplete on done", async () => {
    const fake = installEventSource();
    const onComplete = vi.fn();
    render(<RefreshButton repoID={7} onComplete={onComplete} />);

    await userEvent.click(screen.getByRole("button", { name: /refresh now/i }));
    await waitFor(() => expect(api.refreshRepo).toHaveBeenCalledWith(7));

    emit(fake, { repo_id: 7, phase: "commits", message: "commits: page 1", done: false });
    expect(screen.getByText(/commits: page 1/)).toBeInTheDocument();

    emit(fake, { repo_id: 7, phase: "done", message: "complete", done: true });
    expect(onComplete).toHaveBeenCalledTimes(1);
    expect(fake.close).toHaveBeenCalled();
  });

  it("disables the button while a refresh is in flight", async () => {
    installEventSource();
    render(<RefreshButton repoID={7} onComplete={() => {}} />);
    const btn = screen.getByRole("button", { name: /refresh now/i });
    await userEvent.click(btn);
    await waitFor(() => expect(btn).toBeDisabled());
  });
});
