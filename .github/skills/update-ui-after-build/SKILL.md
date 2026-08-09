---
name: update-ui-after-build
description: "Use when: UI changes were made in the web frontend and you need to verify them in the running app, especially after rebuilds or Docker-based deployment."
---

# Update UI after build

Use this workflow whenever frontend changes are made in the web app and the result must be visible in the browser.

## What to verify

1. Confirm the source code changed in the frontend files.
2. Rebuild the frontend and ensure the new assets are emitted.
3. Deploy via the project's official start script so the container images and stack are refreshed.
4. Open the app in the browser and verify the visible UI, not just the source code.

## Required sequence for this repository

For this project, always follow this order. Never run `docker build ...`,
`docker compose up --build ...`, `npm test`, or `npm run build` directly — the official
deployment entry point is `deploy/start.sh`, which derives image names/versions, runs the
tests inside the image builds, builds both backend and web, and brings the stack up.

1. Run the whole build+test+deploy with the official script:
   - `sh deploy/start.sh local-build`
2. Reload the browser page with a hard refresh and verify the visible UI.

## Important note

Do not assume the UI is updated just because the code changed or the build succeeded. A stale browser view, cached assets, or an old container copy can make the app look unchanged.

## Extra rule for this repository

If the visible browser page still looks old after a rebuild, treat it as a deployment issue until proven otherwise. Check:
- whether the new bundle files were copied into the running web container;
- whether the browser page was reloaded with a hard refresh;
- whether the served HTML references the new asset filenames.

## Checklist

- [ ] Source code updated
- [ ] Stack built, tested, and deployed with `sh deploy/start.sh local-build` (no direct `docker build`, `docker compose up --build`, `npm test`, or `npm run build`)
- [ ] Browser page reloaded and visually verified