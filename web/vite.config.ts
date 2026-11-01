import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

// Node global, declared here to avoid pulling in @types/node for two lookups.
declare const process: { env: Record<string, string | undefined> };

// FBX_VITE_PORT and FBX_API_PORT let a second dev setup run beside the default (5173 → 8090).
const vitePort = Number(process.env.FBX_VITE_PORT ?? "5173");
const api = `http://localhost:${process.env.FBX_API_PORT ?? "8090"}`;

export default defineConfig({
  plugins: [react()],
  server: {
    port: vitePort,
    strictPort: true,
    proxy: {
      "/api": { target: api, changeOrigin: false },
      "/v1": { target: api, changeOrigin: false },
    },
  },
  build: {
    assetsDir: "assets",
    target: "es2022",
    sourcemap: false,
    assetsInlineLimit: 0,
  },
  test: {
    environment: "jsdom",
    globals: false,
    include: ["src/**/*.test.{ts,tsx}"],
    setupFiles: ["src/test-setup.ts"],
  },
});
