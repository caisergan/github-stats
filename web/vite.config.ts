/// <reference types="vitest/config" />
import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";

// Vite evaluates this config in Node, so `process` exists at runtime. Declare it
// locally to read env without adding @types/node just for this.
declare const process: { env: Record<string, string | undefined>; cwd(): string };

// In dev, Vite serves the SPA and proxies API/auth to the Go server on :8080.
// The frontend dev-server port is configurable from the repo-root `.env` via
// WEB_PORT (default 5175). NOTE: this is the FRONTEND port — distinct from the
// backend's ADDR/BASE_URL (the Go server, :8080). In test, Vitest runs the same
// config with a jsdom environment.
export default defineConfig(({ mode }) => {
  // `.env` lives at the repo root (one level up from web/). Empty prefix loads
  // every key, not just VITE_-prefixed ones (WEB_PORT is not exposed to the bundle).
  const rootEnv = loadEnv(mode, process.cwd() + "/..", "");
  const webPort = Number(rootEnv.WEB_PORT ?? process.env.WEB_PORT) || 5175;

  return {
    plugins: [react()],
    build: { outDir: "dist", emptyOutDir: true },
    server: {
      port: webPort,
      strictPort: true,
      proxy: {
        "/api": "http://localhost:8080",
        "/auth": "http://localhost:8080",
      },
    },
    test: {
      environment: "jsdom",
      globals: true,
      setupFiles: ["./vitest.setup.ts"],
      css: false,
      include: ["src/**/*.{test,spec}.{ts,tsx}"],
    },
  };
});
