# AGENTS.md

Workspace: self-hosted multi-user media library (`media_library`).

## Skills

- `update-ui-after-build` (`.github/skills/update-ui-after-build/SKILL.md`) — after
  any frontend change, build, test, and deploy through `deploy/start.sh local-build`.
  Browser or Playwright verification is only done when explicitly requested.
- `media-library-architecture` (`.github/skills/media-library-architecture/SKILL.md`) —
  full architecture map: components, data model, flows, runtime layout, hygiene.
- `short-answers` (`.github/skills/short-answers/SKILL.md`) — answer very short,
  without describing thinking and without filler phrases like "user asked".

## Architecture

Compose stack with one host bind `./runtime` → `/runtime`: api + web always run;
the Caddy gateway runs only when the `https` compose profile is active.

- `backend/` — Go 1.23 REST API (`cmd/server/main.go`). Backed by a full SQLite
  store (`store.NewSQLite`, `DB_DRIVER=sqlite` default) or a Postgres store
  (`store.NewPostgres`, `DB_DRIVER=postgres`, `DB_DSN` as a postgres:// URL).
  The server opens two dedicated store handles on the same database: the
  interactive handle serves HTTP requests, and a separate job handle feeds all
  background work (scans, metadata renew, thumbnail creation, watcher,
  scheduler, job-state persistence) so a long-running job can never starve or
  wedge the connection a user is waiting on. SQLite handles use
  `SetMaxOpenConns(1)` each. Migrations live at
  `backend/internal/store/migrations/{sqlite,postgres}/` and run automatically on
  startup via the embedded `schema_migrations` table. Relative paths are never
  stored; both DB stores compute them on the fly from the library-root prefix.
- `web/` — React 19 + TS + Vite SPA. `api.ts` wraps all REST calls; `App.tsx` is a
  single-file SPA (`/`, `/library/:id`, `/favorites*`, `/map`, `/admin`).
- `gateway` — Caddy 2.10; the deploy compose inlines a small Caddy watcher that
  reloads on generated-Caddyfile change. TLS terminates here (Let's Encrypt).

Key facts:
- Media files stay in original folders; operators mount them into the api
  container and add them as libraries in the admin UI. Libraries reference
  explicit absolute paths, and the api container runs as the host user
  (`MEDIA_UID`/`MEDIA_GID`), which is the filesystem access boundary. DB stores
  users, libraries, permissions, folder/media indexes, metadata, GPS.
- Auth: HttpOnly `media_session` JWT cookie (claims `uid`+`role`). Setup endpoint
  creates the initial admin once, then is disabled forever.
- Every library/media/map/thumbnail/content request passes the same access check.
- Thumbnails are files at `THUMBNAIL_DIR/media/<id/1000>/<id>_<index>.jpg` (folder
  covers at `THUMBNAIL_DIR/folders/<id/1000>/<id>_0.jpg`), not in DB or
  cache; folder covers are 3-way ffmpeg hstack JPEGs.
- Video: `/play` direct-plays if browser codecs + container/audio allow, else FFmpeg
  transcodes to the requesting user's chosen fallback codec (h264/h265/vp9, set per
  account in user settings).
- Scanner: background job walks folder (progress, pause/cancel), upserts folders +
  media, extracts ExifTool/FFprobe metadata, then a thumbnail-create job runs.
- Folder watcher: per-root `watch` flag (off by default) — fsnotify watches each
  opted-in library root recursively and debounces filesystem changes into an
  incremental rescan of that library (see `backend/internal/watcher`).
- Settings → Network access toggles HTTP/HTTPS; gateway config regenerates and
  Caddy hot-reloads.

## Runtime layout (single bind mount; back it up)

- `/runtime/app-data` — DB (`media-library.db`)
- `/runtime/app-config` — generated Caddyfile, `logs/app.log`
- `/runtime/caddy-data`, `/runtime/caddy-config` — Caddy/ACME state
- `/runtime/thumbnails` — generated thumbnails

Container user: the backend image runs as an unprivileged host user resolved at
container start by `docker-entrypoint.sh` from the `MEDIA_UID`/`MEDIA_GID`
environment variables, so the process can write the host-mounted runtime volume.
No UID/GID build arguments exist. Keep `MEDIA_UID`/`MEDIA_GID` in `.env` equal to
the host user.

## Repository hygiene

Commit (source): `backend/` (`cmd/ internal/ migrations/ go.mod go.sum Dockerfile
.dockerignore`), `web/` (`src/ package.json package-lock.json Dockerfile nginx.conf.template
index.html vite.config.ts tsconfig*.json capacitor.config.ts .dockerignore`, plus
`web/android/` if present), `deploy/` (including `compose.local.yaml`), `docs/`,
`.github/`, `AGENTS.md`, `README.md`, `LICENSE`.

Ignore (generated/output): `deploy/.env` (secrets), `.github-token`, `runtime/`,
`web/android/.gradle/`, `web/android/app/build/`, `.idea/`, `.vscode/`.

## Verification

Build, test, and deploy run ONLY through the official script `deploy/start.sh`; never run
raw `docker build`, `docker compose up --build`, `npm test`, or `npm run build` by hand. No
host Go/Node toolchain is used.
- `sh deploy/start.sh local-build` — builds backend + web (each image build runs its tests:
  `go test ./...` and `npm test && npm run build`) and brings the stack up.
- `sh deploy/start.sh prod` — pull published images and restart the stack.
- `sh deploy/start.sh e2e` — run the Playwright suites in web/e2e against the
  running stack inside a dedicated Docker runner (web/Dockerfile.playwright);
  artifacts land in web/test-output/. Requires the stack to be up. Functional
  requirements live in docs/requirements.md.
