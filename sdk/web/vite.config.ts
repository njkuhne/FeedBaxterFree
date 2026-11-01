import { defineConfig } from "vitest/config";

export default defineConfig(({ mode }) => {
  const iife = mode === "iife";
  return {
    build: {
      target: "es2020",
      sourcemap: false,
      outDir: "dist",
      emptyOutDir: !iife,
      minify: iife,
      lib: iife
        ? { entry: "src/global.ts", formats: ["iife" as const], name: "FeedBaxter", fileName: () => "feedbaxter.js" }
        : { entry: "src/index.ts", formats: ["es" as const], fileName: () => "index.js" },
    },
    test: {
      environment: "jsdom",
      globals: false,
      include: ["src/**/*.test.ts"],
    },
  };
});
