import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { PatForm } from "./PatForm";

describe("PatForm", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it("shows the honesty note (PAT is not a rate-limit bypass)", () => {
    render(<PatForm status={{ has_pat: false }} onChange={() => {}} />);
    expect(screen.getByText(/not.*(raise|bypass|increase).*(rate|limit)/i)).toBeTruthy();
  });

  it("saves a token via the API", async () => {
    const fetchMock = vi.spyOn(global, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ has_pat: true, login: "octocat" }), { status: 200 }),
    );
    const onChange = vi.fn();
    render(<PatForm status={{ has_pat: false }} onChange={onChange} />);
    fireEvent.change(screen.getByLabelText(/token/i), { target: { value: "ghp_x" } });
    fireEvent.click(screen.getByRole("button", { name: /save/i }));
    await waitFor(() => expect(onChange).toHaveBeenCalled());
    expect(fetchMock).toHaveBeenCalledWith("/api/settings/pat", expect.objectContaining({ method: "PUT" }));
  });
});
