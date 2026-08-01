---
name: update-ui-after-build
description: "Use when: UI changes were made in the web frontend and you need to verify them in the running app, especially after rebuilds or Docker-based deployment."
---

# Update UI after build

Use this workflow whenever frontend changes are made in the web app and the result must be visible in the browser.

## What to verify

1. Confirm the source code changed in the frontend files.
2. Rebuild the frontend and ensure the new assets are emitted.
3. Copy the built assets into the running web container or restart the relevant services.
4. Open the app in the browser and verify the visible UI, not just the source code.

## Required sequence for this repository

For this project, always follow this order:

1. Run tests for the web app:
   - `cd web && npm test -- --run src/App.test.tsx`
2. Build the frontend:
   - `cd web && npm run build`
3. Rebuild/restart the containerized web stack:
   - `cd .. && docker compose up --build -d web gateway`
4. Copy the built frontend into the running web container:
   - `docker compose cp web/dist/. web:/usr/share/nginx/html/`
5. Reload the browser page and verify the visible UI.

## Important note

Do not assume the UI is updated just because the code changed or the build succeeded. A stale browser view, cached assets, or an old container copy can make the app look unchanged.

## Extra rule for this repository

If the visible browser page still looks old after a rebuild, treat it as a deployment issue until proven otherwise. Check:
- whether the new bundle files were copied into the running web container;
- whether the browser page was reloaded with a hard refresh;
- whether the served HTML references the new asset filenames.

## Checklist

- [ ] Source code updated
- [ ] Web tests passed
- [ ] Frontend build completed
- [ ] Docker web/gateway services refreshed
- [ ] Built assets copied into the running container
- [ ] Browser page reloaded and visually verified
