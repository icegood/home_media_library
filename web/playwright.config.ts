import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  testMatch: ['**/*.pw.ts'],
  timeout: 30_000,
  outputDir: process.env.PW_OUT || 'test-output',
  use: {
    baseURL: process.env.BASE_URL || 'http://localhost:8080',
    // Allow HEADLESS env to control headless vs headed runs
    headless: process.env.HEADLESS !== 'false',
    viewport: { width: 1280, height: 800 },
    video: process.env.PW_VIDEO || 'on',
    trace: process.env.PW_TRACE || 'on',
  },
  projects: [
    { name: 'chromium', use: { browserName: 'chromium' } },
  ],
});
