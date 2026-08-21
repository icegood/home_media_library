This folder contains Playwright end-to-end tests. Run locally with:

  # from web/ directory
  npm ci
  npx playwright install --with-deps
  npx playwright test --project=chromium

When running in Docker, use the official Playwright images or the provided Dockerfile.playwright (see repository skill documentation).
