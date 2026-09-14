-- Keyset (taken_at, name, id) index backing the media neighbors window used by
-- the viewer's root= navigation. Collated so the ORDER BY <taken_at> <name COLLATE
-- NOCASE> <id> comparisons can walk the index instead of sorting the whole scope.
CREATE INDEX IF NOT EXISTS idx_media_neighborhood ON media(taken_at, name COLLATE NOCASE, id);