-- Widen background_jobs.category so metadata metadata-renewal jobs can be
-- persisted. The API's metadata-renew feature creates jobs with category
-- 'metadata-renew', but the CHECK constraint never learned about it, so every
-- persist attempt failed with "CHECK constraint failed" and the log flooded
-- with "could not persist job" (see sqlite/007_metadata_renew_job.sql).
ALTER TABLE background_jobs DROP CONSTRAINT IF EXISTS background_jobs_category_check;
ALTER TABLE background_jobs ADD CONSTRAINT background_jobs_category_check
  CHECK (category IN ('scan', 'thumbnail-create', 'orphan-thumbnail-cleanup', 'vacuum', 'metadata-renew'));