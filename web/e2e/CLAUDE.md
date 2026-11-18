# Playwright e2e (`web/e2e`)

- Specs are `*.pw.ts`; shared helpers in `helpers.ts`.
- Run only via `sh deploy/start.sh e2e` against a stack already up
  (`sh deploy/start.sh local-build`). Runner image: `web/Dockerfile.playwright`,
  locked to the `@playwright/test` version in `package-lock.json`.
- Only run when the user explicitly asks (see `e2e-run` skill).
- Never install browsers on the host; never run specs in the nginx web image.
- Artifacts land in `web/test-output/` (gitignored).
- Assert English canonical strings; functional expectations come from
  `docs/requirements.md`.
