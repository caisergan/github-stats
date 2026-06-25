/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// In dev, Vite serves the SPA on :5175 and proxies API/auth to the Go server.
// (This is the frontend dev-server port — NOT the backend BASE_URL, which is the
// Go server's OAuth/cookie origin on :8080.) In test, Vitest runs the same
// config with a jsdom environment.
export default defineConfig({
  plugins: [react()],
  build: { outDir: "dist", emptyOutDir: true },
  server: {
    port: 5175,
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
});
