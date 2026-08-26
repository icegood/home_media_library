---
name: media-library-architecture
description: "Use when working on the media library project (backend, web, gateway, compose) and you need the architecture map, data model, request flows, runtime layout, or repository hygiene (what to commit vs ignore)."
---

# Media Library Architecture

Self-hosted, multi-user photo/video library. Administrators map arbitrary disk
folders into libraries and grant per-user read access. Media files stay in
their original folders; the database only stores indexes, metadata and grants.
The web app also packages as an Android app via Capacitor.

## Components

- `backend/` — Go 1.23 REST API (`cmd/server/main.go`). Packages:
  - `internal/api` — HTTP handlers, auth, access checks, background jobs
  - `internal/store` — `Store` interface with a full SQLite implementation
    (`sqlite.go`, default via `DB_DRIVER=sqlite`) and a full Postgres
    implementation (`postgres.go`, `DB_DRIVER=postgres`, `DB_DSN` as a
    postgres:// URL). Migrations live at
    `backend/internal/store/migrations/{sqlite,postgres}/` and run automatically
    on startup against the embedded `schema_migrations` table. Relative paths are
    never stored; both DB stores compute them on the fly from the enclosing
    library-root prefix.
  - `internal/domain` — model types and validation (`CanonicalGPS`, etc.)
  - `internal/scanner` — folder walk, upsert of folders/media, MIME sniffing
  - `internal/metadata` — ExifTool + FFprobe extraction
  - `internal/transcode` — FFmpeg transcode to h264/h265/vp9
  - `internal/gatewayconfig` — generates the Caddyfile + reloads via cksum
  - `internal/embyimport` — reads Emby SQLite DBs, builds `ImportSnapshot`
  - `internal/applog` — leveled, rotating file log
- `web/` — React 19 + TypeScript + Vite SPA (Vitest). `src/api.ts` wraps all
  REST calls; `src/App.tsx` is a single-file SPA with routes: `/` (libraries),
  `/library/:id` (browser), `/library/:id/timeline`, `/library/:id/view/:folderId`
  (media viewer), `/favorites*`, `/map`, `/admin`.
- `gateway` — Caddy 2.10. The deploy compose inlines a small watcher for the
  generated Caddyfile and reloads Caddy on change; certs via Let's Encrypt.
- `deploy/compose.yaml` — production api + web + gateway services; single bind
  mount `./runtime` -> `/runtime` plus read-only media sources (user defined). `deploy/compose.local.yaml` adds local source build contexts for dev.

## Data model

- `User` (admin|regular), `Library` with many `LibraryRoot` -> `MediaFolder`
  links. `LibraryAccess` is a pure read grant (admin bypasses it). IDs are
  integers and are the API identifiers.
- `MediaFolder` = global scanned directory tree (incl. empty folders), reused
  across libraries -> no media duplication. `Media` = one physical file leaf,
  keyed by folder; image/video derived from `media_mime_types.media_type`.
- Thumbnails are files under `THUMBNAIL_DIR/media/<id/1000>/<id>_<index>.jpg`
  (folder covers under `THUMBNAIL_DIR/folders/<id/1000>/<id>_0.jpg`), not in DB,
  not in-memory cache); `thumbnails`/`folder_thumbnails`/`folder_thumbnail_files`
  are cross-reference tables. Folder covers = 3-way hstack JPEG from descendant
  thumbnails.
- Persistence (default): SQLite DB at `DB_DSN` (`/runtime/app-data/media-library.db`).

## Key flows

- **Auth**: setup endpoint creates initial admin (then disabled forever).
  Login sets HttpOnly `media_session` JWT cookie (claims carry `uid` + `role`).
- **Access**: every library/media/map/thumbnail/content request passes
  `CanRead*`/`requireMediaRead`; file serving re-validates the resolved path is
  beneath a readable root (traversal/symlink safe).
- **Scanner pipeline**: admin creates library -> background job walks folder
  (progress + pause/cancel via in-memory `JobStatus` map), upserts folders +
  media, extracts metadata/GPS, then a thumbnail-create job runs.
- **Folder watcher**: per-root `watch` flag (off by default) on `library_roots`.
  `backend/internal/watcher` fsnotify-watches opted-in roots recursively,
  debounces change bursts into one incremental library rescan via the scanner.
- **Playback**: `/play` probes source; if browser-advertised codecs match and
  container/audio are direct-play safe, serves original bytes; otherwise FFmpeg
  transcodes to the requesting user's configured fallback codec (set per account
  in user settings).
- **Settings → Network access** toggles HTTP/HTTPS; gateway config is
  regenerated and Caddy hot-reloads without a container restart.

## Runtime layout (single bind mount, back it up)

- `/runtime/app-data` — SQLite DB (`media-library.db`)
- `/runtime/app-config` — generated Caddyfile + app.log
- `/runtime/caddy-data`, `/runtime/caddy-config` — Caddy/ACME state
- `/runtime/thumbnails` — generated thumbnails

## Repository hygiene

Commit (source): `backend/` (`cmd/ internal/ migrations/ go.mod go.sum Dockerfile`),
`web/` (`src/ package.json package-lock.json Dockerfile nginx.conf.template index.html
vite.config.ts tsconfig*.json capacitor.config.ts`, plus `web/android/` shell if
present), `deploy/` (including `compose.local.yaml`), `docs/`, `.github/`.

Ignore (generated/output): `deploy/.env` (secrets), `.github-token`, `runtime/`,
`web/android/.gradle/`, `web/android/app/build/`, `.idea/`, `.vscode/`.

## Verification

All compile/tests run in containers and are driven only by the official script
`deploy/start.sh` — never run `docker build`, `docker compose up --build`, `npm test`, or
`npm run build` directly. `sh deploy/start.sh local-build` builds backend + web (running
`go test ./...` and `npm test && npm run build` inside the image builds) and deploys the
stack. After frontend changes use the `update-ui-after-build` skill to verify the running UI.
