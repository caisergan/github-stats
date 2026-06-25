import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { CollectionManager } from "./CollectionManager";

describe("CollectionManager", () => {
  it("creates a collection", async () => {
    const onCreate = vi.fn().mockResolvedValue(undefined);
    render(<CollectionManager onCreate={onCreate} />);
    fireEvent.change(screen.getByPlaceholderText(/new collection/i), { target: { value: "Backend" } });
    fireEvent.click(screen.getByRole("button", { name: /create/i }));
    await waitFor(() => expect(onCreate).toHaveBeenCalledWith("Backend"));
  });
});
