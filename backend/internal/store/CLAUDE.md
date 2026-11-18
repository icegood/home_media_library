# DB stores (`internal/store`)

`store.go` defines the `Store` interface. Two full implementations:
`sqlite.go` (`NewSQLite`, default `DB_DRIVER=sqlite`) and `postgres.go`
(`NewPostgres`, `DB_DRIVER=postgres`, `DB_DSN` postgres:// URL).
`helpers.go`, `logging.go`, `media_neighbors.go` are shared.

## Rules

- **Parity is mandatory.** Every interface method, query and schema change is
  implemented in both `sqlite.go` and `postgres.go` in the same change, with tests
  in `sqlite_test.go` / `postgres_test.go`.
- **Migrations:** `migrations/sqlite/NNN_name.sql` and
  `migrations/postgres/NNN_name.sql` — same number and name, added together.
  Never edit a migration that has shipped; add a new one. They are embedded
  (`//go:embed`) and auto-applied on startup via `schema_migrations`.
  See the `add-db-migration` skill.
- **Dialect differences:** `?` vs `$N`; `LastInsertId` vs `RETURNING id`;
  `INSERT OR IGNORE` vs `ON CONFLICT DO NOTHING`; booleans `NUMERIC 0/1` vs
  `BOOLEAN`; `OVERRIDING SYSTEM VALUE` for imported ids; no boolean arithmetic in
  `ORDER BY`. Share SQL text where possible.
- **No relative-path columns.** Relative paths are computed from the enclosing
  library-root prefix at query time.
- **Media kind** comes from `media_mime_types.media_type` — never store it on `media`.
- **GPS** is one canonical `latitude,longitude` string column (not JSON);
  metadata goes in `metadata_json` under `exif` / `ffprobe`.
- **Stats shape** is always per-kind `{images, videos, documents}`.
- Multi-row rewrites (root path moves, bulk updates) run in one transaction.
- Deterministic ordering (by relative path) where resumable jobs page by offset.
