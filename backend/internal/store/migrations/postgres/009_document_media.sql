ALTER TYPE media_type ADD VALUE IF NOT EXISTS 'document';

INSERT INTO media_mime_types(value, media_type) VALUES
  ('text/plain', 'document'),
  ('text/markdown', 'document'),
  ('application/pdf', 'document');

INSERT INTO media_mime_extensions(extension, mime_type) VALUES
  ('.txt', 'text/plain'),
  ('.md', 'text/markdown'),
  ('.pdf', 'application/pdf');