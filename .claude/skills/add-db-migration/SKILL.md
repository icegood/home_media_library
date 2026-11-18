---
name: add-db-migration
description: "Use when the media library database schema or seed/lookup data (e.g. media_mime_types) must change. Creates paired SQLite + Postgres migrations and keeps both stores in parity."
---

# Add a DB migration

Migrations: `backend/internal/store/migrations/{sqlite,postgres}/NNN_name.sql`,
embedded and auto-applied on startup via `schema_migrations`.

1. Find the next number: `ls backend/internal/store/migrations/sqlite | tail -1`
   (three digits, e.g. `017_`). Use the **same number and name** in both dirs.
2. Write both files. Start each with a `--` comment explaining *why*.
   Dialect mapping:

   | SQLite | Postgres |
   |---|---|
   | `NUMERIC NOT NULL DEFAULT 0` (bool) | `BOOLEAN NOT NULL DEFAULT FALSE` |
   | `1` / `0` | `TRUE` / `FALSE` |
   | `INTEGER PRIMARY KEY AUTOINCREMENT` | `BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY` (imports need `OVERRIDING SYSTEM VALUE`) |
   | `INSERT OR IGNORE` | `INSERT ... ON CONFLICT DO NOTHING` |

   Check an existing pair (e.g. `016_library_watch.sql`) for the house style.
3. Backfill data so upgrades never silently change behaviour for existing users.
4. Never edit a migration that already shipped — add a new one.
5. Update `domain` types, then both `sqlite.go` and `postgres.go` queries
   (`?` vs `$N`, `LastInsertId` vs `RETURNING id`).
6. Never add relative-path or media-kind columns (computed / looked up).
7. Add/extend tests in `sqlite_test.go` and `postgres_test.go`.
8. Build+test: `sh deploy/start.sh local-build`.
