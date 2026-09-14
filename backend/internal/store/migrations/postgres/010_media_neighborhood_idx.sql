-- Keyset (taken_at, lower(name), id) index backing the media neighbors window
-- used by the viewer's root= navigation, matching the ORDER BY comparisons.
CREATE INDEX IF NOT EXISTS idx_media_neighborhood ON media(taken_at, lower(name), id);