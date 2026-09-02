PRAGMA foreign_keys = OFF;

CREATE TABLE media_mime_types_new (
  value TEXT PRIMARY KEY,
  media_type TEXT NOT NULL CHECK (media_type IN ('image', 'video', 'document'))
);

INSERT INTO media_mime_types_new(value, media_type) SELECT value, media_type FROM media_mime_types;

INSERT INTO media_mime_types_new(value, media_type) VALUES
  ('text/plain', 'document'),
  ('text/markdown', 'document'),
  ('application/pdf', 'document');

DROP TABLE media_mime_types;
ALTER TABLE media_mime_types_new RENAME TO media_mime_types;

INSERT INTO media_mime_extensions(extension, mime_type) VALUES
  ('.txt', 'text/plain'),
  ('.md', 'text/markdown'),
  ('.pdf', 'application/pdf');

PRAGMA foreign_keys = ON;