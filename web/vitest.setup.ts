import "@testing-library/jest-dom/vitest";
import { afterEach, beforeEach, vi } from "vitest";
import { cleanup } from "@testing-library/react";

// Seed the non-httpOnly gs_csrf cookie so api.ts's CSRF helper reads it
// directly instead of triggering an extra GET /api/csrf fetch. This keeps the
// mocked-fetch tests asserting on the intended mutating request as call[0].
beforeEach(() => {
  document.cookie = "gs_csrf=test-csrf-token";
});

// Unmount any React tree and clear all mocks after every test for isolation.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
