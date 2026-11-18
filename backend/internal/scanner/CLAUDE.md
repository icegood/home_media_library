# Scanner, metadata, thumbnails

Covers `internal/scanner`, `internal/metadata`, and thumbnail generation in
`internal/api`.

## Pipeline

1. Admin creates/edits a library (new roots → scan starts).
2. Background scan job walks roots (progress, pause/cancel), sniffs MIME,
   upserts folders + media (slim tasks `{filePath, mimeType, parentID}`), marks
   vanished files unavailable. Directory layout is never changed.
3. Metadata: ExifTool (EXIF) + FFprobe (streams) → `metadata_json`; EXIF GPS
   seeds the canonical GPS string.
4. A thumbnail-create job runs after the scan.

## Rules

- Scans use the **job** store handle and `jobpool`; never the interactive handle.
- Media files are never moved, renamed or written.
- Thumbnails: `THUMBNAIL_DIR/media/<id/1000>/<id>_<index>.jpg`; images use index
  `0` (first frame only — animated GIFs must not break ffmpeg); videos may have
  up to 100 indexed frames (default first at 5 s, interval ≥120 s). Width/height
  are global admin settings.
- Folder covers: `THUMBNAIL_DIR/folders/<id/1000>/<id>_0.jpg`, a 3-way ffmpeg
  `hstack` of descendant thumbnails (including nested subfolders).
- New supported file types: add the extension/MIME to `media_mime_types` via a
  paired migration (see `add-db-migration`), not a hard-coded list.
- Metadata renew is resumable by offset (deterministic relative-path ordering).
