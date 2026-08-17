---
name: update-ui-after-build
description: "Use when: UI changes were made in the web frontend and you need to build, test, and deploy them through the local stack. Browser or Playwright checks are only done when explicitly requested."
---

# Update UI after build

Use this workflow whenever frontend changes are made in the web app and the local stack must be rebuilt and redeployed.

## What to verify

1. Confirm the source code changed in the frontend files.
2. Rebuild the frontend and ensure the new assets are emitted.
3. Deploy via the project's official start script so the container images and stack are refreshed.
4. Do not open the app in the browser or use Playwright unless the user explicitly asks for a browser/UI check.

## Required sequence for this repository

For this project, always follow this order. Never run `docker build ...`,
`docker compose up --build ...`, `npm test`, or `npm run build` directly — the official
deployment entry point is `deploy/start.sh`, which derives image names/versions, runs the
tests inside the image builds, builds both backend and web, and brings the stack up.

1. Run the whole build+test+deploy with the official script:
   - `sh deploy/start.sh local-build`
2. Stop after the script succeeds unless the user explicitly asks for browser verification.

## Important note

Do not assume the UI is visually correct just because the code changed or the build succeeded. However, browser or Playwright verification is opt-in and should only be performed when explicitly requested.

## Extra rule for this repository

If the user asks for browser verification and the visible page still looks old after a rebuild, treat it as a deployment issue until proven otherwise. Check:
- whether the new bundle files were copied into the running web container;
- whether the browser page was reloaded with a hard refresh;
- whether the served HTML references the new asset filenames.

## Checklist

- [ ] Source code updated
- [ ] Stack built, tested, and deployed with `sh deploy/start.sh local-build` (no direct `docker build`, `docker compose up --build`, `npm test`, or `npm run build`)
- [ ] Browser/Playwright verification skipped unless explicitly requested
