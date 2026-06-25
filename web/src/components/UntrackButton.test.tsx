import { describe, it, expect, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import UntrackButton from "./UntrackButton";

function setup(
  onUntrack: (id: number) => Promise<void> = vi.fn().mockResolvedValue(undefined),
  onDone: () => void = vi.fn(),
) {
  render(
    <UntrackButton repoID={5} repoName="octo/demo" onUntrack={onUntrack} onDone={onDone} />,
  );
  return { onUntrack, onDone };
}

describe("UntrackButton", () => {
  it("explains the consequence and does not act on the first click", async () => {
    const { onUntrack } = setup();
    expect(screen.queryByText(/permanently deletes/i)).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: /untrack repo/i }));

    expect(screen.getByText(/permanently deletes/i)).toBeInTheDocument();
    expect(screen.getByText(/octo\/demo/)).toBeInTheDocument();
    expect(onUntrack).not.toHaveBeenCalled();
  });

  it("untracks the repo and navigates away on confirm", async () => {
    const onUntrack = vi.fn().mockResolvedValue(undefined);
    const onDone = vi.fn();
    setup(onUntrack, onDone);

    await userEvent.click(screen.getByRole("button", { name: /untrack repo/i }));
    await userEvent.click(screen.getByRole("button", { name: /yes, untrack/i }));

    await waitFor(() => expect(onUntrack).toHaveBeenCalledWith(5));
    expect(onDone).toHaveBeenCalled();
  });

  it("surfaces an error and stays put when the untrack fails", async () => {
    const onUntrack = vi.fn().mockRejectedValue(new Error("purge failed"));
    const onDone = vi.fn();
    setup(onUntrack, onDone);

    await userEvent.click(screen.getByRole("button", { name: /untrack repo/i }));
    await userEvent.click(screen.getByRole("button", { name: /yes, untrack/i }));

    await waitFor(() => expect(screen.getByText(/purge failed/i)).toBeInTheDocument());
    expect(onDone).not.toHaveBeenCalled();
  });

  it("cancel dismisses the confirmation without deleting", async () => {
    const { onUntrack } = setup();
    await userEvent.click(screen.getByRole("button", { name: /untrack repo/i }));
    await userEvent.click(screen.getByRole("button", { name: /cancel/i }));

    expect(screen.queryByText(/permanently deletes/i)).not.toBeInTheDocument();
    expect(onUntrack).not.toHaveBeenCalled();
  });
});
