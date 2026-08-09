# Architecture

## Domain

`User` has either the `admin` or `regular` role. `Library` has one or more
links to explicit absolute folders on disk. Each linked folder appears as a
top-level folder. Database primary keys are integers and
those same IDs are used by the API. `LibraryAccess` is a pure many-to-many read
grant between users and libraries: a row grants access, and no row means no
regular-user access. Administrators always have read access regardless of
`LibraryAccess` rows. An administrator can manage libraries, users and grants.
Both roles may edit coordinates for media they can read.

`MediaFolder` stores the global scanned directory tree, including empty folders.
When an admin selects a folder as a library root, the backend validates and
upserts that folder into `media_folders` first, then creates `LibraryRoot` as a
link to that folder by `media_folders.id`.
`Media` represents one physical image or video leaf and points to its containing
folder, so reusing a folder in multiple libraries does not duplicate media. It
stores its physical path, MIME type, size, timestamps, extracted EXIF/media
metadata and optional GPS as one canonical `latitude,longitude` string. Image
vs video type is derived from the `media_mime_types.media_type` lookup instead
of being stored as a second field on each media row. Scanning never changes the
directory layout.

## Security boundaries

- The API container runs as an unprivileged host user (`MEDIA_UID`/`MEDIA_GID`);
  that OS user is the filesystem access boundary — the backend serves whatever
  that user can read and rejects nothing at the app level.
- Every library, metadata, map, thumbnail and content request passes through
  the same access check.
- Media folders are mounted into the API container by the operator and added
  as libraries in the admin UI.
- Passwords use bcrypt; sessions are signed, expiring bearer tokens.
- HTTPS deployment terminates TLS at Caddy, which obtains and renews its
  certificate directly through Let's Encrypt ACME. API and web containers are
  private on the Compose network.
- Production should add request-size/rate limits appropriate to its users.

## Database choice

The repository contract is database-independent. `DB_DRIVER` selects the backing
store: `sqlite` is the default (`store.NewSQLite`); `postgres` is fully
implemented (`store.NewPostgres`, `DB_DSN` as a postgres:// URL) for
concurrency/high availability. Migrations live in
`backend/internal/store/migrations/{sqlite,postgres}/`, are embedded into the
binary, and run automatically on startup against a `schema_migrations` table.
The two DB stores keep the schema in sync; relative paths are never a column —
both compute them on the fly from the enclosing library-root prefix. Shared SQL
where possible, driver-specific where necessary (`?` vs `$N`, `RETURNING id` vs
`LastInsertId`, `ON CONFLICT` vs `INSERT OR IGNORE`, `OVERRIDING SYSTEM VALUE`
for imported ids, and boolean-arithmetic-free ORDER BY expressions).

## Scanner pipeline

1. Admin creates a library mapping.
2. A background scan walks directories and supported files under that directory.
3. Workers sniff MIME type, extract EXIF/video metadata, take EXIF GPS as the
   initial coordinate when available, and create thumbnails into the configured
   external thumbnail root.
4. A transaction upserts seen files and marks disappeared files unavailable.

The scanner runs ExifTool for image/video EXIF tags and FFprobe for video/audio
stream details. Metadata is stored in `metadata_json` under `exif` and
`ffprobe`; GPS is stored separately as the canonical `latitude,longitude`
string so map queries do not need to parse JSON.

Thumbnails are not stored in the database and are not kept in a transient cache.
They live under `THUMBNAIL_DIR` using `<media_id>/<thumbnail_index>.jpg`.
`thumbnails` is a child table of `media`: image files use thumbnail index `0`;
videos may have many indexed thumbnails such as `0`, `1`, `2`, etc, but never
more than ten. Video thumbnail timing is globally configurable: the default
first frame is at 5 seconds and the default/minimum interval is 120 seconds.
Thumbnail width and height are global admin settings, not per-thumbnail columns.
Folders expose one derived JPEG cover made by horizontally concatenating three
equal-width slices from selected descendant thumbnails, including media found
through nested subfolders. `folder_thumbnails` is a pure cross table from folder
to source media; its `thumbnail_index` is the ordered slot inside the generated
folder cover. `folder_thumbnail_files` stores the generated folder cover file.
Browser views can switch between sorted list view and tile view using these
thumbnail-sized tiles.

## Roadmap

- Add durable background jobs and scan progress events.
- Add durable thumbnail cleanup for deleted media.
- Add HTTP range requests, cache headers and optional HLS transcoding.
- Add segment caching/HLS for arbitrary seeking within on-the-fly transcodes;
  direct-play video already supports HTTP byte-range seeking.
- Add user-management screens, password reset and token revocation.
- Add offline Android caching and background uploads if write support is added.
- Add end-to-end browser/Android tests and backup/restore commands.
