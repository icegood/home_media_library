# media_library — project rules

Self-hosted multi-user photo/video/document library. Go API + React SPA + Caddy
gateway, shipped as a Docker Compose stack (and an Android Capacitor shell).

Rules are hierarchical: this file holds project-wide rules; each feature area has
its own `CLAUDE.md` that is loaded when you work in that directory. Read the
nearest one before editing.

| Area | Rules file |
|---|---|
| Go backend (packages, jobs, two store handles) | `backend/CLAUDE.md` |
| REST API, auth, access checks, OpenAPI | `backend/internal/api/CLAUDE.md` |
| DB stores + migrations (SQLite & Postgres) | `backend/internal/store/CLAUDE.md` |
| Scanner, metadata, thumbnails | `backend/internal/scanner/CLAUDE.md` |
| Folder watcher | `backend/internal/watcher/CLAUDE.md` |
| Playback / transcoding | `backend/internal/transcode/CLAUDE.md` |
| Gateway / Caddy / HTTPS | `backend/internal/gatewayconfig/CLAUDE.md` |
| Web SPA (React, api.ts, i18n) | `web/CLAUDE.md` |
| Playwright e2e | `web/e2e/CLAUDE.md` |
| Android shell | `web/android/CLAUDE.md` |
| Build / deploy / runtime | `deploy/CLAUDE.md` |
| Docs (requirements, OpenAPI copy) | `docs/CLAUDE.md` |

## Skills (`.claude/skills/`, also visible as `.github/skills/`)

- `short-answers` — always: answer very short, no narrated thinking, no filler.
- `media-library-architecture` — full architecture map, data model, flows.
- `update-ui-after-build` — after any code change: build+test+deploy locally.
- `e2e-run` — run Playwright suites (only when asked).
- `add-feature` — end-to-end checklist for a user-visible change.
- `add-db-migration` — paired SQLite + Postgres migration.
- `add-api-endpoint` — route + handler + store + OpenAPI + `api.ts`.
- `add-ui-text` — new UI strings and their 14 translations.
- `ship-release` — "commit and deploy as usual" to the NAS.

## Hard rules (project-wide)

1. **Build/test/deploy only via `deploy/start.sh`.** Never run raw `docker build`,
   `docker compose up --build`, `go test`, `npm test`, `npm run build` by hand. No
   host Go/Node toolchain. `sh deploy/start.sh local-build` runs `go test ./...`
   and `npm test && npm run build` inside the image builds.
2. **Never prune Docker caches** (`docker builder prune`, `system prune`, image
   removal) to "force" a rebuild unless the user asks. Incremental builds must work.
3. **Browser/Playwright verification is opt-in** — only when explicitly requested.
4. **Both DB stores move together.** Any schema or query change lands in SQLite
   *and* Postgres (`backend/internal/store/`), with paired migrations.
5. **Never store relative paths.** Both stores compute them from the library-root prefix.
6. **Every library/media/map/thumbnail/content request passes the same access check.**
7. **`docs/requirements.md` is the functional source of truth** — update it with
   every user-visible behaviour change.
8. **Do not bump `VERSION`** unless asked. Bump `ANDROID_VERSION` only when the
   native shell changes (`web/android/**`, `web/capacitor.config.ts`).
9. **Commit message format:** numbered items on one line —
   `1) <what changed and why> 2) <next change>`; describe behaviour, not files.
   Commit/push only when asked.

## Repository hygiene

Commit: `backend/` (`cmd/ internal/ go.mod go.sum Dockerfile .dockerignore`),
`web/` (`src/ e2e/ scripts/ package.json package-lock.json Dockerfile*
nginx.conf.template index.html vite.config.ts playwright.config.ts tsconfig*.json
capacitor.config.ts .dockerignore`, `web/android/` shell), `deploy/`, `docs/`,
`.github/`, `.claude/skills/`, `CLAUDE.md` files, `AGENTS.md`, `README.md`, `LICENSE`,
`VERSION`, `ANDROID_VERSION`.

Ignore: `deploy/.env` (secrets), `.github-token`, `runtime/`, `build/`,
`web/node_modules/`, `web/coverage/`, `web/test-output/`,
`web/src/generated-version.ts`, `web/android/.gradle/`, `web/android/app/build/`,
`web/android/keystore.properties`, `.idea/`, `.vscode/`, `.claude/settings.local.json`.

GitHub tokens come from `~/.git-credentials` (or the gitignored `.github-token`);
never pull tokens from stray temp files.
