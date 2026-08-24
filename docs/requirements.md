# Functional requirements

Single source of truth for what the app must do, including the UI-consistency
rules introduced during the 2026 polish pass. Verification paths: backend
`go test ./...`, frontend `npm test && npm run build` (both run inside image
builds via `sh deploy/start.sh local-build`), and end-to-end Playwright suites
via `sh deploy/start.sh e2e`.

## Auth & users

- First-run setup endpoint creates the initial admin once, then is disabled forever.
- Login issues an HttpOnly `media_session` JWT cookie (claims `uid` + `role`).
- Roles: `admin` and `regular`. Every library/media/map/thumbnail/content request passes the same access check; non-admins only see libraries shared with them.
- Admin panel manages users, per-library user access, login timeout.

## Libraries & scanning

- Media files stay in their original folders; operators mount them into the api container and add them as libraries referencing explicit absolute paths. Relative paths are never stored; both DB stores compute them from the library-root prefix.
- Scanner runs as a background job: walks folders with progress/pause/cancel, upserts folder + media indexes (slim tasks `{filePath, mimeType, parentID}`), extracts ExifTool/FFprobe metadata, then a thumbnail-create job runs.
- Duplicate-job guard: starting a job that is already active for the same category+library is rejected.
- Metadata renewal supports resume-by-offset after restart (deterministic ordering by relative path).

## Statistics - one shape everywhere

- Classification comes ONLY from the `media_mime_types(value, media_type)` table via JOIN. Never classify by parsing `mime_type` strings.
- The only statistics shape is `{images, videos}` (`domain.KindStats` / web `KindStats`). No other counts (folders/files) are exposed anywhere.
- All stats surfaces use it: embedded `/api/v1/libraries` listing (computed server-side for all libraries in one grouped recursive pass), per-library `/stats` endpoint, folder menus, favorite-view menus.
- Folder and favorite-view menu lines render `Images: X . Videos: Y` inline; values are always backend-computed recursively, never client-side tallies.

## Browsing

- Views per library: Folders tree, Timeline (date groups), List/Tile layouts, content-visibility + virtual scrolling for large sets.
- Fixed filters bar auto-hides under the header (`VV` handle); its height publishes `--filters-h` on every page that has one, including Map.
- Bulk selection: checkboxes top-left on every card where selection applies (media and folder cards, tile and list). Selection feeds bulk GPS set/shift and download.
- Item dropdowns (three-dot): ONE component (`CardMenu`) for library tiles, folder entries, admin library rows and favorite rows; portal popup, closes on outside click and on menu-item click, fetches lazy contents only while open.
- Placement rules: three-dot trigger top-right on every card (list rows: right edge); selection checkbox top-left wherever applicable.

## Media viewer

- Opens from any surface preserving context (folder range, root/kind scope, favorites origin chain via `?fav=`, map selections via `?list=`).
- Images: zoom/pan. Videos: direct play when browser codecs allow, otherwise FFmpeg transcode to the user's chosen fallback codec (h264/h265/vp9); server-side seek offset support.
- Fullscreen control for videos; keyboard arrows page within the active range only.

## Favorites

- Named favorite views; media and folders can be members of multiple views.
- Favorites index: create editor as a separate full-width panel above the list, submit right-aligned inside the editor.
- Favorite view page: folders + media cards, both selectable (top-left checkboxes), display-mode switch Folders/Timeline/Map, kind filter, sort.
- Removing a member from a view never deletes the underlying media/folder.

## Map

- Markers cluster by zoom; progressive rendering for large sets; place search ordered nearest-first; GPS picker popup.
- Area selection draws a rectangle and queries the server for contained media.
- Results panel: right-side overlay anchored below header+filters on ALL viewports (phone: capped at `min(320px, 82vw)` so it never covers the screen).
- Clicking a result opens the viewer paging through EXACTLY the selected items in selected order; the selection is handed over verbatim (sessionStorage key `media-library-map-selection`, URL param `list=`).

## Thumbnails & metadata

- Thumbnail files live at `THUMBNAIL_DIR/media/<id/1000>/<id>_<index>.jpg`; folder covers at `THUMBNAIL_DIR/folders/<id/1000>/<id>_0.jpg` (3-way ffmpeg hstack JPEGs).
- Default-thumb pictures per kind; refresh flows support "missing only" and "recreate existing".

## Watch folders

- Opt-in per library root (`watch` flag on `library_roots`, off by default), toggled via the admin library editor ("Watch for changes").
- Enabled roots are watched recursively with fsnotify; file/dir events debounce (~3s) into one incremental rescan of that library using the regular scan job (duplicate guard applies).
- The watch set re-syncs every 30s and immediately after library create/update/delete; roots missing on disk are retried.

## ZIP download

- `POST /api/v1/archive` accepts `{ids[], folders[]}`: selected media plus everything inside the given folders (recursive), capped at 1000 files.
- Entries keep library-relative paths (folder structure preserved); per-item read access is enforced for every member.

## Jobs & scheduler

- One pool with cancel semantics (`context.Canceled` propagated), pause via condition variable, throttled progress persistence, cron validation before firing.

## Settings & themes

- Themes: light / dark / forest (+ system-follow). All component colors come from CSS custom properties defined per theme; hardcoded theme-specific colors are forbidden.
- Active/selected controls use `--primary-text` background with `--primary-contrast` text; primary buttons glow uses `--btn-shadow-color`.
- Global unified focus ring (`:focus-visible` outline in `--primary`).
- User settings: UI zoom, stream chunk size, date format, video fallback codec, default thumbnails.
- Network settings toggle HTTP/HTTPS; gateway Caddyfile regenerates and hot-reloads.

## Runtime layout

- Single bind mount `./runtime`: DB (`app-data`), generated Caddyfile + logs (`app-config`), Caddy ACME state, thumbnails. Back up this directory.
- Backend container runs as an unprivileged host user resolved from `MEDIA_UID`/`MEDIA_GID`.
