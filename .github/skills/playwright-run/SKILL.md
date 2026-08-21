# Playwright e2e (how to run)

This skill describes how to run Playwright end-to-end tests for the web UI in this repository.

Principles
- Do not run Playwright tests in the published nginx web image. Tests require a Node runtime and browser dependencies.
- Preferred: run Playwright inside a dedicated test container that uses the official Playwright Docker images (they include browser binaries and OS deps).
- The project's canonical build/deploy is still `sh deploy/start.sh local-build` (builds images and brings the stack up). Start the app first, then run Playwright tests against the running app.

Options

1) Run tests locally on the host (developer machine)

  cd web
  npm ci
  # install browsers (may need extra packages on Linux)
  npx playwright install --with-deps
  # set BASE_URL if the app is served on a non-default port/host
  BASE_URL=http://localhost:8080 npx playwright test --project=chromium

Notes:
- `BASE_URL` defaults to http://localhost:8080, which is the web service's internal port (the gateway or compose will publish WEB_PORT). When running tests against a stack started with `sh deploy/start.sh local-build`, set BASE_URL to the published host port (e.g. http://localhost:8080 or http://127.0.0.1:8080 depending on your compose settings).

2) Run tests inside Docker (recommended for CI and reproducible environment)

A minimal approach using Playwright's official docker image:

  # from repository root
  # build and start the stack first
  sh deploy/start.sh local-build

  # build a test image that contains the web sources and Playwright
  docker build -t media-lib-playwright -f - web <<'EOF'
  FROM mcr.microsoft.com/playwright:focal
  WORKDIR /src
  COPY web/package.json web/package-lock.json ./
  RUN npm ci
  COPY web/ ./
  RUN npx playwright install --with-deps
  EOF

  # run tests, pointing at the host where the web service is published
  # --network host is easiest on Linux; on macOS/Windows use host.docker.internal
  docker run --rm --network host -e BASE_URL=http://localhost:8080 media-lib-playwright npm run e2e

Notes:
- On non-Linux hosts, replace `--network host` with `-e BASE_URL=http://host.docker.internal:8080` so the container can reach the host-published web port.
- The dev/build stack (sh deploy/start.sh local-build) publishes the web service to the host; confirm the published port and use that in BASE_URL.

3) Run tests inside the compose network (advanced)

You can also add a `playwright` service to `deploy/compose.local.yaml` that builds from `../web` and uses the Node base with Playwright installed, then `docker compose run --rm playwright npm run e2e` to run tests inside the same network as other services.

Troubleshooting
- Browser fails to start: missing system dependencies. Use the Playwright docker image (it bundles deps) or run `npx playwright install --with-deps` on a compatible distro.
- Tests cannot reach the app: ensure the stack is up and the BASE_URL points to the host:port published by the stack (see deploy/.env for WEB_PORT/GATEWAY settings).
- Headful debugging: run with HEADLESS="false" or set Playwright `headless: false` in playwright.config.ts.

Useful commands summary
- Build+start stack: sh deploy/start.sh local-build
- Local dev run (host): cd web && npm ci && npx playwright install --with-deps && BASE_URL=http://localhost:8080 npx playwright test
- Docker run (Linux): docker run --rm --network host -e BASE_URL=http://localhost:8080 media-lib-playwright npm run e2e

