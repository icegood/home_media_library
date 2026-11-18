# Build, deploy, runtime (`deploy/`)

## `start.sh` — the only entry point

- `sh deploy/start.sh local-build` — build backend + web images (tests run inside:
  `go test ./...`, `npm test && npm run build`) and bring the stack up.
- `sh deploy/start.sh prod` — pull `ghcr.io/icegood/home-media-library-{api,web}:$PROJECT_VERSION`,
  restart, drop images of other versions.
- `sh deploy/start.sh e2e` — Playwright runner against the running stack.
- `sh deploy/start.sh android` — signed APK.

Never replace these with raw `docker build` / `docker compose up --build`.
Never prune Docker caches unless asked.

## Files

- `compose.yaml` — production api + web (+ `gateway` under the `https` profile).
- `compose.local.yaml` — adds local build contexts.
- `.env` (from `.env.default`) — secrets, `PROJECT_VERSION`, `WEB_PORT`,
  `MEDIA_UID`/`MEDIA_GID` (must equal the host user). Never commit.

## Runtime (single bind `./runtime` → `/runtime`; back it up)

- `app-data/` — `media-library.db` (WAL: back up `-wal`/`-shm` with it, or stop first)
- `app-config/` — generated Caddyfile, `logs/app.log`
- `caddy-data/`, `caddy-config/` — Caddy/ACME state
- `thumbnails/` — generated thumbnails

## Container user

The backend image runs as an unprivileged user resolved at start by
`docker-entrypoint.sh` from `MEDIA_UID`/`MEDIA_GID`. No UID/GID build args.

## Production

Images are built by `.github/workflows/publish-images.yml` (dispatch, or push of
`VERSION`). NAS deploy steps: `ship-release` skill.
