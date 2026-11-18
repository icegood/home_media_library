# Backend (Go 1.23)

Entry point: `cmd/server/main.go`. Module path `media-library/backend`.
Tests run only inside the image build (`sh deploy/start.sh local-build`).

## Packages (`internal/`)

| Package | Role | Rules |
|---|---|---|
| `api` | HTTP handlers, auth, access checks, job orchestration, thumbnails | `internal/api/CLAUDE.md` |
| `store` | `Store` interface; SQLite + Postgres implementations; migrations | `internal/store/CLAUDE.md` |
| `domain` | Model types + validation (`CanonicalGPS`, …) | — |
| `scanner` / `metadata` | Folder walk, MIME sniff, ExifTool/FFprobe | `internal/scanner/CLAUDE.md` |
| `watcher` | fsnotify → debounced incremental rescan | `internal/watcher/CLAUDE.md` |
| `transcode` | FFmpeg transcode h264/h265/vp9 | `internal/transcode/CLAUDE.md` |
| `gatewayconfig` | Caddyfile generation + reload | `internal/gatewayconfig/CLAUDE.md` |
| `jobpool` | Single app-wide worker pool; capacity = worker-pool-size setting | — |
| `scheduler` | Cron-style scheduled tasks; calls `Runner` (implemented by `api`) | — |
| `poi` | Overpass/POI lookup for the map | — |
| `embyimport` | Reads Emby SQLite DBs into an `ImportSnapshot` | — |
| `mail` | Password-reset mail | — |
| `applog` | Leveled rotating file log (`/runtime/app-config/logs/app.log`) | — |

## Rules

- **Two store handles.** `main.go` opens an *interactive* handle (HTTP requests)
  and a separate *job* handle (scans, metadata renew, thumbnails, watcher,
  scheduler, job-state persistence). Background code must use the job handle;
  never let a long job hold the interactive connection. SQLite handles use
  `SetMaxOpenConns(1)`, WAL, `synchronous=NORMAL`.
- **All concurrent media work goes through `jobpool`** — do not spawn ad-hoc
  goroutine pools; paused jobs must park queued work without holding capacity.
- **Duplicate-job guard:** a job already active for the same category+library is rejected.
- **Scheduled tasks reuse the job system** via `scheduler.Runner`; don't add a parallel path.
- **Filesystem access boundary is the OS user** (`MEDIA_UID`/`MEDIA_GID`); file
  serving must still re-validate resolved paths are beneath a readable root
  (traversal/symlink safe).
- Every behaviour change gets a Go test next to the code (`*_test.go`); store
  changes need tests for both SQLite and Postgres where the suite supports it.
- Keep `domain` free of DB/HTTP imports.
