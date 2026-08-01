PRAGMA foreign_keys = ON;

CREATE TABLE user_roles (
  value TEXT PRIMARY KEY
);

INSERT INTO user_roles(value) VALUES ('admin'), ('regular');

CREATE TABLE media_mime_types (
  value TEXT PRIMARY KEY,
  media_type TEXT NOT NULL CHECK (media_type IN ('image', 'video'))
);

INSERT INTO media_mime_types(value, media_type) VALUES
  ('image/jpeg', 'image'),
  ('image/png', 'image'),
  ('image/gif', 'image'),
  ('image/webp', 'image'),
  ('image/heic', 'image'),
  ('video/mp4', 'video'),
  ('video/x-m4v', 'video'),
  ('video/quicktime', 'video'),
  ('video/x-matroska', 'video'),
  ('video/webm', 'video'),
  ('video/x-msvideo', 'video'),
  ('video/mpeg', 'video');

CREATE TABLE media_mime_extensions (
  extension TEXT PRIMARY KEY CHECK (extension = lower(extension) AND substr(extension, 1, 1) = '.'),
  mime_type TEXT NOT NULL REFERENCES media_mime_types(value) ON UPDATE CASCADE ON DELETE RESTRICT
);

INSERT INTO media_mime_extensions(extension, mime_type) VALUES
  ('.jpg', 'image/jpeg'),
  ('.jpeg', 'image/jpeg'),
  ('.png', 'image/png'),
  ('.gif', 'image/gif'),
  ('.webp', 'image/webp'),
  ('.heic', 'image/heic'),
  ('.mp4', 'video/mp4'),
  ('.m4v', 'video/x-m4v'),
  ('.mov', 'video/quicktime'),
  ('.mkv', 'video/x-matroska'),
  ('.webm', 'video/webm'),
  ('.avi', 'video/x-msvideo'),
  ('.mpg', 'video/mpeg'),
  ('.mpeg', 'video/mpeg');

CREATE TABLE server_settings (
  id INTEGER PRIMARY KEY CHECK (id = 0),
  value_json TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  login TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL REFERENCES user_roles(value) ON UPDATE CASCADE
);

CREATE TABLE user_settings (
  user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  value_json TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE libraries (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL
);
CREATE UNIQUE INDEX libraries_name_unique ON libraries(lower(name));

CREATE TABLE media_folders (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  parent_id INTEGER REFERENCES media_folders(id) ON DELETE CASCADE,
  path TEXT NOT NULL UNIQUE
);

CREATE TABLE library_roots (
  library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
  folder_id INTEGER NOT NULL REFERENCES media_folders(id) ON DELETE RESTRICT,
  PRIMARY KEY (library_id, folder_id)
);

CREATE TABLE library_access (
  library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  PRIMARY KEY (library_id, user_id)
);

CREATE TABLE media (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  folder_id INTEGER NOT NULL REFERENCES media_folders(id) ON DELETE CASCADE,
  path TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  mime_type TEXT NOT NULL REFERENCES media_mime_types(value) ON UPDATE CASCADE,
  size INTEGER NOT NULL,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  gps TEXT NOT NULL DEFAULT '',
  taken_at TEXT NOT NULL DEFAULT '',
  metadata_error TEXT NOT NULL DEFAULT '',
  thumbnail_error TEXT NOT NULL DEFAULT ''
);

CREATE TABLE favorite_views (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  UNIQUE (user_id, name)
);

CREATE TABLE favorite_view_items (
  favorite_view_id INTEGER NOT NULL REFERENCES favorite_views(id) ON DELETE CASCADE,
  media_id INTEGER NOT NULL REFERENCES media(id) ON DELETE CASCADE,
  PRIMARY KEY (favorite_view_id, media_id)
);

CREATE TABLE thumbnails (
  media_id INTEGER NOT NULL REFERENCES media(id) ON DELETE CASCADE,
  thumbnail_index INTEGER NOT NULL,
  mime_type TEXT NOT NULL DEFAULT 'image/jpeg',
  PRIMARY KEY (media_id, thumbnail_index)
);

CREATE TABLE folder_thumbnails (
  folder_id INTEGER NOT NULL REFERENCES media_folders(id) ON DELETE CASCADE,
  thumbnail_index INTEGER NOT NULL,
  source_media_id INTEGER NOT NULL REFERENCES media(id) ON DELETE CASCADE,
  PRIMARY KEY (folder_id, thumbnail_index)
);

CREATE TABLE folder_thumbnail_files (
  folder_id INTEGER PRIMARY KEY REFERENCES media_folders(id) ON DELETE CASCADE,
  mime_type TEXT NOT NULL DEFAULT 'image/jpeg'
);

CREATE TABLE background_jobs (
  id TEXT PRIMARY KEY,
  category TEXT NOT NULL CHECK (category IN ('scan', 'thumbnail-create', 'orphan-thumbnail-cleanup')),
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

INSERT INTO sqlite_sequence(name, seq) VALUES
  ('users', -1),
  ('libraries', -1),
  ('media_folders', -1),
  ('media', -1),
  ('favorite_views', -1);

CREATE INDEX media_folders_parent_idx ON media_folders(parent_id);
CREATE INDEX library_roots_folder_idx ON library_roots(folder_id);
CREATE INDEX media_folder_idx ON media(folder_id);
CREATE INDEX favorite_views_user_idx ON favorite_views(user_id);
CREATE INDEX favorite_view_items_media_idx ON favorite_view_items(media_id);
CREATE INDEX thumbnails_media_idx ON thumbnails(media_id);
CREATE INDEX folder_thumbnails_folder_idx ON folder_thumbnails(folder_id);
CREATE INDEX folder_thumbnails_source_idx ON folder_thumbnails(source_media_id);
CREATE INDEX background_jobs_status_idx ON background_jobs(status);
CREATE INDEX background_jobs_library_idx ON background_jobs(library_id);
