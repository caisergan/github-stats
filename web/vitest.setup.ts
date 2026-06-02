import "@testing-library/jest-dom/vitest";
import { afterEach, vi } from "vitest";
import { cleanup } from "@testing-library/react";

// Unmount any React tree and clear all mocks after every test for isolation.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
