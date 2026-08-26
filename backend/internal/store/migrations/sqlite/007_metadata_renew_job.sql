-- Widen background_jobs.category so metadata metadata-renewal jobs can be
-- persisted. The API's metadata-renew feature (startMetadataRenewJob) creates
-- jobs with category 'metadata-renew', but the CHECK constraint never learned
-- about it, so every persist attempt failed with "CHECK constraint failed:
-- category IN (...)" and the log flooded with "could not persist job". SQLite
-- cannot alter a CHECK constraint, so rebuild the table with the same shape as
-- 002_patch.sql plus the new category.
CREATE TABLE background_jobs_new (
  id TEXT PRIMARY KEY,
  category TEXT NOT NULL CHECK (category IN ('scan', 'thumbnail-create', 'orphan-thumbnail-cleanup', 'vacuum', 'metadata-renew')),
  type TEXT NOT NULL,
  library_id INTEGER NOT NULL DEFAULT 0,
  library_name TEXT NOT NULL,
  root_path TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL CHECK (status IN ('running', 'paused', 'cancelling', 'cancelled', 'failed', 'done')),
  paused INTEGER NOT NULL DEFAULT 0 CHECK (paused IN (0, 1)),
  cancelable INTEGER NOT NULL DEFAULT 1 CHECK (cancelable IN (0, 1)),
  current_path TEXT NOT NULL DEFAULT '',
  processed INTEGER NOT NULL DEFAULT 0,
  total INTEGER NOT NULL DEFAULT 0,
  error TEXT NOT NULL DEFAULT '',
  started_at TEXT NOT NULL,
  finished_at TEXT,
  options_json TEXT NOT NULL DEFAULT '{}'
);

INSERT INTO background_jobs_new (id, category, type, library_id, library_name, root_path, status, paused, cancelable, current_path, processed, total, error, started_at, finished_at, options_json)
  SELECT id, category, type, library_id, library_name, root_path, status, paused, cancelable, current_path, processed, total, error, started_at, finished_at, options_json
  FROM background_jobs;

DROP TABLE background_jobs;
ALTER TABLE background_jobs_new RENAME TO background_jobs;

CREATE INDEX IF NOT EXISTS background_jobs_status_idx ON background_jobs(status);
CREATE INDEX IF NOT EXISTS background_jobs_library_idx ON background_jobs(library_id);