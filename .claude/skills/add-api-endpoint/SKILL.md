---
name: add-api-endpoint
description: "Use when adding or changing a REST endpoint of the media library backend. Covers route, access check, store, OpenAPI (both copies), web api.ts and tests."
---

# Add / change an API endpoint

1. **Route** in the mux block of `backend/internal/api/api.go`:
   - user: `mux.Handle("GET /api/v1/...", a.auth(http.HandlerFunc(a.handler)))`
   - admin: `mux.Handle("POST /api/v1/admin/...", a.auth(a.admin(http.HandlerFunc(a.handler))))`
2. **Handler:**
   - Parse path values with `r.PathValue("id")`; bad input → `problem(w, 400, "...")`.
   - Access check before touching data: `requireRead` / `requireMediaRead` /
     `requireFolderRead` (return early when false).
   - Respond with `writeJSON(w, status, v)`.
   - Long work → background job (job store handle, `jobpool`, JobStatus,
     duplicate-job guard).
   - Library mutations → call `OnLibrariesChanged` so the watcher refreshes.
3. **Store:** new interface method in `store.go` implemented in `sqlite.go` and
   `postgres.go` (see `add-db-migration` if schema changes).
4. **OpenAPI:** edit `backend/internal/api/openapi.yaml`, then
   `cp backend/internal/api/openapi.yaml docs/openapi.yaml` (they must be identical).
5. **Tests:** `api_test.go` — success case + forbidden case for a regular user
   without access (+ admin-only check where relevant).
6. **Web:** add a typed wrapper in `web/src/api.ts` (types in `types.ts`), test
   in `api.test.ts`. New `problem()` texts → `add-ui-text` (messages table).
7. `docs/requirements.md` if behaviour is user-visible.
8. Build+test: `sh deploy/start.sh local-build`.
