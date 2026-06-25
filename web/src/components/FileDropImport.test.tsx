import { describe, it, expect, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { FileDropImport } from "./FileDropImport";

function makeFile(name: string, content: string): File {
  return new File([content], name, { type: "text/plain" });
}

describe("FileDropImport", () => {
  it("parses a dropped package.json and reports resolved repos", async () => {
    vi.spyOn(global, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ resolved: ["acme/x"], unresolved: ["react"] }), { status: 200 }),
    );
    const onResult = vi.fn();
    render(<FileDropImport onResult={onResult} />);
    const input = screen.getByTestId("file-input") as HTMLInputElement;
    const file = makeFile("package.json", '{"dependencies":{"@acme/x":"1"}}');
    Object.defineProperty(input, "files", { value: [file] });
    input.dispatchEvent(new Event("change", { bubbles: true }));
    await waitFor(() => expect(onResult).toHaveBeenCalledWith({ resolved: ["acme/x"], unresolved: ["react"] }));
  });
});
