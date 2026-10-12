/// <reference types="vitest/config" />
import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";

// The build output is committed (frontend/dist) so `go build ./...` can embed it
// without a Node toolchain present. Stable asset filenames keep that committed
// bundle from churning a new hash on every rebuild.
export default defineConfig(({ mode }) => {
  // The API the dev server proxies to. Inside the compose dev stack Vite runs in
  // its own container and reaches the air-reloaded API by service name, so compose
  // sets API_PROXY=http://dev:8080. A host-run Vite defaults to the API on
  // localhost:8080 (use http://localhost:8081 for the dev sandbox).
  const env = loadEnv(mode, ".", "API_");
  const apiTarget = env.API_PROXY || "http://localhost:8080";

  return {
    plugins: [react()],
    build: {
      outDir: "dist",
      emptyOutDir: true,
      rollupOptions: {
        output: {
          entryFileNames: "assets/app.js",
          chunkFileNames: "assets/[name].js",
          assetFileNames: "assets/[name][extname]",
        },
      },
    },
    // In dev, proxy the API and the served docs/spec so the app — including the
    // spec-driven Workbench, which fetches /openapi.json — uses same-origin paths
    // exactly as it does when served by the Go binary.
    server: {
      proxy: {
        "/v1": { target: apiTarget, changeOrigin: true },
        "/openapi": { target: apiTarget, changeOrigin: true },
        "/docs": { target: apiTarget, changeOrigin: true },
        "/swagger": { target: apiTarget, changeOrigin: true },
      },
    },
    test: {
      environment: "jsdom",
      globals: true,
      setupFiles: ["./src/setupTests.ts"],
      css: false,
    },
  };
});
