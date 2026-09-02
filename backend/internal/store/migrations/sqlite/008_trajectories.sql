CREATE TABLE IF NOT EXISTS trajectory_starts (
  folder_id INTEGER NOT NULL REFERENCES media_folders(id) ON DELETE CASCADE,
  media_id INTEGER NOT NULL REFERENCES media(id) ON DELETE CASCADE,
  name TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (folder_id, media_id)
);

CREATE INDEX IF NOT EXISTS trajectory_starts_media_idx ON trajectory_starts(media_id);

CREATE TABLE IF NOT EXISTS trajectory_ends (
  folder_id INTEGER NOT NULL REFERENCES media_folders(id) ON DELETE CASCADE,
  media_id INTEGER NOT NULL REFERENCES media(id) ON DELETE CASCADE,
  PRIMARY KEY (folder_id, media_id)
);

CREATE INDEX IF NOT EXISTS trajectory_ends_media_idx ON trajectory_ends(media_id);