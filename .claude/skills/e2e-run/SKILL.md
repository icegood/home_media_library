---
name: e2e-run
description: "Use only when the user explicitly asks to run Playwright / end-to-end / browser tests for the media library web UI."
---

# Run Playwright e2e

1. Make sure the stack is up with current code: `sh deploy/start.sh local-build`.
2. Run all specs: `sh deploy/start.sh e2e`.
   - Builds `media-library-playwright:local` from `web/Dockerfile.playwright`,
     version-locked to `@playwright/test` in `web/package-lock.json`.
   - BASE_URL = `http://localhost:$WEB_PORT` (from `deploy/.env`); override with
     `E2E_BASE_URL=...`.
3. Report pass/fail per spec. On failure, point to videos/traces/reports in
   `web/test-output/` and read the trace/error before guessing a fix.

## Never

- Run specs inside the published nginx web image.
- Install browsers or run `npx playwright` on the host.
- Hand-roll `docker build`/`docker run` for the runner — `start.sh e2e` is the
  only sanctioned path.

## Writing specs

- `web/e2e/*.pw.ts`, shared helpers in `web/e2e/helpers.ts`.
- Assert English canonical strings; derive expectations from `docs/requirements.md`.
