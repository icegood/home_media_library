-- Filesystem watching is a per-library setting, not a per-root one: a single
-- flag on the library drives the watcher for every root it contains.
ALTER TABLE libraries ADD COLUMN watch BOOLEAN NOT NULL DEFAULT FALSE;

-- Adopt the old per-root flags: a library that watched any of its roots keeps
-- watching now, so the upgrade never silently disables an opted-in library.
UPDATE libraries SET watch = TRUE WHERE EXISTS (
  SELECT 1 FROM library_roots lr
  WHERE lr.library_id = libraries.id AND COALESCE(lr.watch, FALSE) = TRUE
);
