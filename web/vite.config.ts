import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  server: {
    host: true,
    proxy: {
      "/api": process.env.VITE_API_PROXY_TARGET ?? "http://localhost:8080",
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: "./src/test-setup.ts",
    coverage: {
      // Gate on line coverage; text table lands in build logs, the HTML
      // report under web/coverage/ shows per-line details for developers.
      reporter: ["text", "html"],
      reportsDirectory: "coverage",
      include: ["src"],
      exclude: ["src/main.tsx", "src/generated-version.ts", "src/vite-env.d.ts", "src/types.ts", "src/test-support/**", "**/*.test.*"],
      thresholds: {lines: 95}
    }
  },
});
