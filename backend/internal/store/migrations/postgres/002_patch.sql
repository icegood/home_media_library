CREATE EXTENSION IF NOT EXISTS postgis;

ALTER TABLE media ADD COLUMN geom geometry(Point, 4326)
  GENERATED ALWAYS AS (
    CASE WHEN gps = '' THEN NULL
      ELSE ST_SetSRID(ST_MakePoint(
        split_part(gps, ',', 2)::double precision,
        split_part(gps, ',', 1)::double precision), 4326)
    END
  ) STORED;

CREATE INDEX media_geom_idx ON media USING gist (geom);

CREATE TABLE IF NOT EXISTS scheduled_tasks (
  id BIGINT GENERATED ALWAYS AS IDENTITY (START WITH 0 MINVALUE 0) PRIMARY KEY,
  name TEXT NOT NULL,
  task_type TEXT NOT NULL CHECK (task_type IN ('scan', 'thumbnail-create', 'vacuum')),
  library_id BIGINT NOT NULL DEFAULT 0,
  cron TEXT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT true,
  last_run_at TIMESTAMPTZ,
  next_run_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS scheduled_tasks_enabled_idx ON scheduled_tasks(enabled);
CREATE INDEX IF NOT EXISTS scheduled_tasks_next_run_idx ON scheduled_tasks(next_run_at);

-- Widen background_jobs.category so database-maintenance (vacuum) jobs can be
-- persisted.
ALTER TABLE background_jobs DROP CONSTRAINT IF EXISTS background_jobs_category_check;
ALTER TABLE background_jobs ADD CONSTRAINT background_jobs_category_check
  CHECK (category IN ('scan', 'thumbnail-create', 'orphan-thumbnail-cleanup', 'vacuum'));
