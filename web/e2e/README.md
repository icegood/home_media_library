# Playwright end-to-end tests

Canonical run (Docker, sanctioned — requires the stack to be up):

    sh deploy/start.sh local-build   # build + start the stack first
    sh deploy/start.sh e2e           # run all *.pw.ts specs against it

- The runner image (web/Dockerfile.playwright) is version-locked to the
  @playwright/test version in web/package-lock.json.
- BASE_URL defaults to http://localhost:$WEB_PORT from deploy/.env; override
  per-run with E2E_BASE_URL=...
- Videos, traces and reports land in web/test-output/.

Manual host runs are possible but not sanctioned:

    cd web && npm ci && npx playwright install --with-deps
    BASE_URL=http://localhost:18080 npx playwright test --project=chromium
