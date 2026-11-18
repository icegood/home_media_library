# REST API (`internal/api`)

`api.go` holds routing (Go 1.22 `mux.HandleFunc("METHOD /path", …)`), handlers,
auth middleware and job orchestration. `swagger.go` embeds `openapi.yaml`.

## Rules

- **Routes:** register in the mux block near the top of `api.go`; authenticated
  routes wrap with `a.auth(...)`, admin-only ones with `a.auth(a.admin(...))`
  under `/api/v1/admin/`. Prefix `/api/v1/`.
- **Access check on every data route:** use `requireRead` (library),
  `requireMediaRead`, `requireFolderRead`. Admin bypasses grants; regular users
  need a `LibraryAccess` row. Never serve library/media/map/thumbnail/content
  without one of these.
- **Errors:** use `problem(w, status, "message")`. Message texts are shown in the
  UI and translated client-side — when you add/change one, add the English text
  to `web/src/i18n-messages.ts` (see `add-ui-text` skill).
- **JSON:** `writeJSON(w, status, v)`; reject bad bodies with `400 "invalid JSON"`.
- **Auth:** `media_session` HttpOnly JWT cookie (`uid` + `role`). The setup
  endpoint works once (no users), then is disabled forever.
- **OpenAPI:** `internal/api/openapi.yaml` and `docs/openapi.yaml` are identical
  copies — edit both (or copy one over the other) for every endpoint change.
- **Jobs:** long work starts a background job on the *job* store handle, reports
  progress in `JobStatus`, supports pause/cancel, and goes through `jobpool`.
  Rejected while a duplicate job (category+library) is active.
- **Thumbnails** are files (`thumbnailPath`, `folderThumbnailPath`), never DB blobs.
- Tests: `api_test.go` (httptest). Add a test per new route, including a
  forbidden-access case for regular users.
