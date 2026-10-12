/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// The build output is committed (frontend/dist) so `go build ./...` can embed it
// without a Node toolchain present. Stable asset filenames keep that committed
// bundle from churning a new hash on every rebuild.
export default defineConfig({
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
  // In dev, proxy the API so the app can call same-origin /v1 paths.
  server: {
    proxy: {
      "/v1": "http://localhost:8080",
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/setupTests.ts"],
    css: false,
  },
});
