PRAGMA foreign_keys = ON;

ALTER TABLE media ADD COLUMN gps_lat REAL;
ALTER TABLE media ADD COLUMN gps_lng REAL;

UPDATE media SET
  gps_lat = CASE WHEN instr(gps, ',') > 0
    THEN CAST(trim(substr(gps, 1, instr(gps, ',') - 1)) AS REAL) END,
  gps_lng = CASE WHEN instr(gps, ',') > 0
    THEN CAST(trim(substr(gps, instr(gps, ',') + 1)) AS REAL) END
WHERE gps <> '';

CREATE VIRTUAL TABLE media_geo USING rtree(id, minLat, maxLat, minLng, maxLng);

INSERT INTO media_geo(id, minLat, maxLat, minLng, maxLng)
  SELECT id, gps_lat, gps_lat, gps_lng, gps_lng FROM media WHERE gps_lat IS NOT NULL AND gps_lng IS NOT NULL;

CREATE TRIGGER media_geo_after_insert AFTER INSERT ON media
BEGIN
  INSERT INTO media_geo(id, minLat, maxLat, minLng, maxLng)
    SELECT new.id, new.gps_lat, new.gps_lat, new.gps_lng, new.gps_lng
    WHERE new.gps_lat IS NOT NULL AND new.gps_lng IS NOT NULL;
END;

CREATE TRIGGER media_geo_after_update AFTER UPDATE OF gps_lat, gps_lng ON media
BEGIN
  DELETE FROM media_geo WHERE id = old.id;
  INSERT INTO media_geo(id, minLat, maxLat, minLng, maxLng)
    SELECT new.id, new.gps_lat, new.gps_lat, new.gps_lng, new.gps_lng
    WHERE new.gps_lat IS NOT NULL AND new.gps_lng IS NOT NULL;
END;

CREATE TRIGGER media_geo_after_delete AFTER DELETE ON media
BEGIN
  DELETE FROM media_geo WHERE id = old.id;
END;

CREATE TABLE IF NOT EXISTS scheduled_tasks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  task_type TEXT NOT NULL CHECK (task_type IN ('scan', 'thumbnail-create', 'vacuum')),
  library_id INTEGER NOT NULL DEFAULT 0,
  cron TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
  last_run_at TEXT,
  next_run_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS scheduled_tasks_enabled_idx ON scheduled_tasks(enabled);
CREATE INDEX IF NOT EXISTS scheduled_tasks_next_run_idx ON scheduled_tasks(next_run_at);

-- Widen background_jobs.category so database-maintenance (vacuum) jobs can be
-- persisted. SQLite cannot alter a CHECK constraint, so rebuild the table.
CREATE TABLE background_jobs_new (
  id TEXT PRIMARY KEY,
  category TEXT NOT NULL CHECK (category IN ('scan', 'thumbnail-create', 'orphan-thumbnail-cleanup', 'vacuum')),
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

INSERT INTO background_jobs_new
  SELECT id, category, type, library_id, library_name, root_path, status, paused, cancelable, current_path, processed, total, error, started_at, finished_at, options_json
  FROM background_jobs;

DROP TABLE background_jobs;
ALTER TABLE background_jobs_new RENAME TO background_jobs;

CREATE INDEX IF NOT EXISTS background_jobs_status_idx ON background_jobs(status);
CREATE INDEX IF NOT EXISTS background_jobs_library_idx ON background_jobs(library_id);
