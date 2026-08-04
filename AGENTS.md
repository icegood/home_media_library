# AGENTS.md

Workspace: self-hosted multi-user media library (`media_library`).

## Skills

- `update-ui-after-build` (`.github/skills/update-ui-after-build/SKILL.md`) — after
  any frontend change, verify in the running app: run web tests, `npm run build`,
  `docker compose up --build -d web gateway`, copy `web/dist/` into the running web
  container, hard-reload the browser. A stale browser/container means deployment
  issue until proven otherwise.
- `media-library-architecture` (`.github/skills/media-library-architecture/SKILL.md`) —
  full architecture map: components, data model, flows, runtime layout, hygiene.
- `short-answers` (`.github/skills/short-answers/SKILL.md`) — answer very short,
  without describing thinking and without filler phrases like "user asked".

## Architecture

Three-container Compose stack with one host bind `./runtime` → `/runtime`:

- `backend/` — Go 1.23 REST API (`cmd/server/main.go`). Backed by a full SQLite
  store (`store.NewSQLite`, `DB_DRIVER=sqlite` default) or a Postgres store
  (`store.NewPostgres`, `DB_DRIVER=postgres`, `DB_DSN` as a postgres:// URL).
  Migrations live at
  `backend/internal/store/migrations/{sqlite,postgres}/` and run automatically on
  startup via the embedded `schema_migrations` table. Relative paths are never
  stored; both DB stores compute them on the fly from the library-root prefix.
- `web/` — React 19 + TS + Vite SPA. `api.ts` wraps all REST calls; `App.tsx` is a
  single-file SPA (`/`, `/library/:id`, `/favorites*`, `/map`, `/admin`).
- `gateway` — Caddy 2.10; the deploy compose inlines a small Caddy watcher that
  reloads on generated-Caddyfile change. TLS terminates here (Let's Encrypt).

Key facts:
- Media files stay in original folders (mounted read-only, `MEDIA_ROOT`); libraries
  reference explicit absolute paths, and the api container runs as the host user
  (`MEDIA_UID`/`MEDIA_GID`), which is the filesystem access boundary. DB stores
  users, libraries, permissions, folder/media indexes, metadata, GPS.
- Auth: HttpOnly `media_session` JWT cookie (claims `uid`+`role`). Setup endpoint
  creates the initial admin once, then is disabled forever.
- Every library/media/map/thumbnail/content request passes the same access check.
- Thumbnails are files at `THUMBNAIL_DIR/<media_id>/<index>.jpg`, not in DB or
  cache; folder covers are 3-way ffmpeg hstack JPEGs.
- Video: `/play` direct-plays if browser codecs + container/audio allow, else FFmpeg
  transcodes to admin-chosen fallback (h264/h265/vp9).
- Scanner: background job walks folder (progress, pause/cancel), upserts folders +
  media, extracts ExifTool/FFprobe metadata, then a thumbnail-create job runs.
- Settings → Network access toggles HTTP/HTTPS; gateway config regenerates and
  Caddy hot-reloads.

## Runtime layout (single bind mount; back it up)

- `/runtime/app-data` — DB (`media-library.db`)
- `/runtime/app-config` — generated Caddyfile, `logs/app.log`
- `/runtime/caddy-data`, `/runtime/caddy-config` — Caddy/ACME state
- `/runtime/thumbnails` — generated thumbnails

Container user: the backend image runs as `app` with build args `UID`/`GID`
(default 1000) so the process can write the host-mounted runtime volume. Build with
`docker build --build-arg UID=$(id -u) --build-arg GID=$(id -g) -t media-library-api ./backend`.

## Repository hygiene

Commit (source): `backend/` (`cmd/ internal/ migrations/ go.mod go.sum Dockerfile
.dockerignore`), `web/` (`src/ package.json package-lock.json Dockerfile nginx.conf
index.html vite.config.ts tsconfig*.json capacitor.config.ts .dockerignore`, plus
`web/android/` if present), `compose.local.yaml`, `deploy/`, `docs/`,
`.github/`, `AGENTS.md`, `README.md`, `LICENSE`.

Ignore (generated/output): `.env` (secrets), `runtime/`, `thumbnails/`,
`runtime.discarded-*/`, `thumbnails.discarded-*/`, `sample-media/`, `web/node_modules/`,
`web/dist/`, `*.tsbuildinfo`, `backend/bin/`, `web/android/.gradle/`,
`web/android/app/build/`, `*.db`, `*.db-shm`, `*.db-wal`, `.idea/`, `.vscode/`.

## Verification

All build/test is done via Docker; no host Go/Node toolchain is used:
- `docker build -t media-library-api ./backend` (runs `go test ./...`)
- `docker build -t media-library-web ./web` (runs `npm test && npm run build`)
- Local frontend dev/test: `cd web && npm test -- --run src/App.test.tsx`
