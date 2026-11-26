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
- After moving files locally, an admin can replace one exact library root path from the Admin → Libraries menu. The operation transactionally rewrites the root, descendant folder, and media paths in SQLite or PostgreSQL without rescanning; it does not move files itself and is rejected while the library has active jobs.
- The library dialog (Edit) always starts from the stored library, re-read when it opens, and the admin list reloads whenever the dialog saves or closes, so saving never writes back a stale root list and drops roots added earlier. Saving with newly added roots starts a library scan.
- The root-folder picker has a path box: paste a path (e.g. `/media/data1/0ira`) and press Enter or Open to jump there. In the library editors each subfolder has a checkbox; checks persist while browsing, and "Add checked" fills the row the picker was opened from with the first checked folder and adds the rest as new root rows (paths already listed are skipped).
- Dialogs close on a backdrop click only when the press both starts and ends on the backdrop: selecting text in a field (e.g. copying part of a root path) and releasing the mouse outside the panel keeps the dialog open.
- Scanner runs as a background job: walks folders with progress/pause/cancel, upserts folder + media indexes (slim tasks `{filePath, mimeType, parentID}`), extracts ExifTool/FFprobe metadata, then a thumbnail-create job runs.
- A browser tab left open across a deploy shows a "new version — Reload" banner: on focus and every 5 minutes the page compares its bundle with the one the server serves, so an old tab cannot keep saving with old code (that is how roots were lost after the root-list fix shipped).
- Duplicate-job guard: starting a job that is already active for the same category+library is rejected.
- Every folder has a "show as child" flag, on for all folders by default (an unset flag means on). An admin turns it off from the folder card menu in folder view ("Hide from parent…", after a confirm; the card disappears at once, regular users have no such item) or in Admin → Hidden folders by pasting a folder link (`/library/L/folder/F`, `/library/L/timeline/F`, a map link with `folder=F`) or the folder id; that list shows every hidden folder (library, path) with Open / Timeline / Map links and a "Show in parent" button. A hidden folder and everything below it is left out, for everyone, of its parent's folder list (a hidden library root of the library page), of every ancestor's and the library's Timeline, of maps, neighbour paging and folder covers, of stats/counts, and of bulk edits on a selected ancestor. Regular users cannot open it or anything inside it even by direct URL (403); admins open it from the admin list, and opened directly its own folder view, Timeline and map show its content (its own hidden subfolders stay left out). Scans, folder refreshes, metadata and thumbnail jobs keep processing it and never reset the flag.
- Metadata renewal supports resume-by-offset after restart (deterministic ordering by relative path).

## Statistics - one shape everywhere

- Classification comes ONLY from the `media_mime_types(value, media_type)` table via JOIN. Never classify by parsing `mime_type` strings.
- The only statistics shape is `{images, videos, documents}` (`domain.KindStats` / web `KindStats`). No other counts (folders/files) are exposed anywhere.
- All stats surfaces use it: embedded `/api/v1/libraries` listing (computed server-side for all libraries in one grouped recursive pass), per-library `/stats` endpoint, folder menus, favorite-view menus.
- Folder and favorite-view menu lines render `Images: X · Videos: Y · Documents: Z` inline; values are always backend-computed recursively, never client-side tallies.

## Browsing

- Views per library: Folders tree, Timeline (date groups), List/Tile layouts, content-visibility + virtual scrolling for large sets.
- Coming back from the viewer to a folder grid or Timeline (browser Back or a breadcrumb) focuses the item that was open and centres its row, even in a large timeline: the date group holding that item is drawn in the first frame, and focused; only the groups within about 1600 px above and below the screen get their cards, the rest are placeholders of their estimated height (date rail and scroll length stay right) that fill in as they come near while scrolling, and keep their cards afterwards, the item is focused as soon as its card exists, kept centred while the layout settles, and the scroll stops as soon as the user scrolls, clicks or presses a key. The same applies to a favorite view's timeline. Going up from a subfolder to its parent (breadcrumb or browser Back) focuses and centres the subfolder you came from; a breadcrumb target not among the loaded entries of a big folder is paged in until it appears. Going up from a subfolder's Timeline to an ancestor's Timeline (breadcrumb or Back) focuses the first of that subfolder's items in the ancestor's order. The last two Timelines' media lists are kept in memory, so going back to one shows and focuses at once; the fresh list is loaded in the background and replaces the shown one only if it differs.
- Timeline groups by date along a vertical ruler; its filter bar scopes the view by kind (All / Images / Videos / Documents) and by GPS presence (All / Geotagged / No GPS). Opening an item keeps the current folder/kind/GPS scope in the viewer (`root=`, `kind=`, `gps=` URL params), so prev/next paging stays inside the active filter.
- Folders view kind scope (All / Images / Videos / Documents) filters the entry cards client-side and keeps the scope when opening the viewer (`kind=` URL param, no `root`).
- Fixed filters bar auto-hides under the header (`VV` handle); its height publishes `--filters-h` on every page that has one, including Map.
- Bulk selection: checkboxes top-left on every card where selection applies (media and folder cards, tile and list). Selection feeds the bulk bar and download. The bulk bar has GPS, Time shift (h/m) and Notes fields and ONE Apply button: every non-empty field is applied to the selection in a single `PATCH /media/bulk` (GPS, then the time shift, then notes; all validated before anything is written); Notes replace the notes of every selected item with the same text; the Notes field is multi-line (one line high, growing up to 6 lines, then scrolling): Enter adds a line, Ctrl/⌘+Enter applies, and the line breaks are kept.
- Item dropdowns (three-dot): ONE component (`CardMenu`) for library tiles, folder entries, admin library rows and favorite rows; portal popup, closes on outside click and on menu-item click, fetches lazy contents only while open.
- Placement rules: three-dot trigger top-right on every card (list rows: right edge); selection checkbox top-left wherever applicable.

## Media viewer

- Opens from any surface preserving context (folder range, root/kind/GPS scope, favorites origin chain via `?fav=`, map selections via `?list=`).
- Images: zoom/pan (zoom 50%–3000%). Videos: direct play when browser codecs allow, otherwise FFmpeg transcode to the user's chosen fallback codec (h264/h265/vp9); server-side seek offset support.
- Fullscreen control for videos; keyboard arrows page within the active range only.
- Notes: a media card (Folders, Timeline, Favorites; tile and list) with notes shows a 📝 button (tile: left, under the selection checkbox) that opens the notes in a popover; it closes on an outside click, Escape or ×. On the map, the cluster/area list panel shows a `📝 N` button when any of its items has notes, opening one popover with every note (item name, date, text) in the panel's order.
- GPS: a media card (Folders, Timeline, Favorites; tile and list) with GPS coordinates shows a 📍 link (tile: left, under the 📝 button or in its place when there are no notes; list: beside it) that opens the map focused on that item, scoped like the viewer's "Open on map" (library folder or favorite view). In the viewer the info panel's "Open on map" link right next to the GPS field uses the same 📍 pictogram.
- A video element that leaves the stage (paging away, a seek slot replaced) stops its download at once (`src` removed + `load()`), so a clip skipped past never keeps streaming and starving the item on screen.

## Favorites

- Named favorite views; media and folders can be members of multiple views.
- Favorites index: create editor as a separate full-width panel above the list, submit right-aligned inside the editor.
- Favorite view page: folders + media cards, both selectable (top-left checkboxes), display-mode switch Folders/Timeline/Map, kind filter, sort.
- Kind filter (Folders, Timeline, Favorites) is a multi-checkbox dropdown of Images, Videos, Documents; any subset can be checked (at least one stays checked; the button reads "All" when every kind is checked). The viewer receives it as `kind=` with a comma list (e.g. `image,video`), and `/media/neighbors` accepts the same list.
- Removing a member from a view never deletes the underlying media/folder.

## Map

- Markers cluster by zoom; on a folder/item map at zoom 18+ (or the map's maximum zoom) only media at identical coordinates share a marker, so points a few metres apart (e.g. revisiting a spot later on a trajectory) can always be told apart; progressive rendering for large sets; place search ordered nearest-first; GPS picker popup.
- When the active basemap is Esri — Satellite, the map bar shows an "Imagery" date picker listing the Esri World Imagery Wayback releases (newest first, plus "latest"); picking a date shows that archived imagery. The choice is kept in the URL (`imagery=YYYY-MM-DD` or `latest`); a folder map without one starts at the release nearest the median capture date of its geotagged media. Other basemaps have no dated archive and show no picker.
- Coordinate search (map-menu "Set point by coordinates" and the picked-point popup) accepts the canonical `lat,lng`, Google Maps degree+decimal-minute text (`N 050° 4.035, E 19° 56.614`), DMS and hemisphere forms, and Google Maps/`geo:` links (`q=`/`ll=`/`query=`, trailing `@lat,lng,z`, `!3d…!4d…`). Input is normalized to the canonical `latitude,longitude`; out-of-range coordinates are rejected.
- In a folder-scoped map the "No GPS" toggle lists that folder's un-geotagged media in a left side panel (loaded from the folder media endpoint); a no-GPS viewer paging keeps `gps=nogps` scope.
- "Walk" toggle (walking mode) shows the user's own position (dot + accuracy circle; the first fix centers the map, "Center on me" recenters), a "Radius" field (meters typed in, applied on Enter or blur, default 1000 m, remembered per device; drawn as a dashed circle, changing it zooms to fit) and a "Near you" panel listing only the map's geotagged media within that radius, nearest first (up to 200), with distance and compass direction (e.g. `350 m · NE`); marker popups show the distance from you. Any point can replace the device position: in walk mode the picked-point popup (map click or "Set point") and the place-search result offer "Walk from here", drawn as an orange dot with a "Near this point" list; "Use my location" returns to the device position. This also makes walking mode usable without GPS or on plain-HTTP browsers. On phones the panel is a bottom sheet. Location comes from the browser Geolocation API (secure origins only: HTTPS/localhost) or, in the Android app, from the native `MLLocation` bridge (works with plain-HTTP servers; runtime location permission, paused while the app is in the background).
- Coming back from the viewer to a folder map (browser Back or a folder breadcrumb) zooms the map in on the item that was open (zoom at least 17, once per arrival); items without GPS leave the usual fit.
- Drawn trajectories show a direction arrow on every edge, pointing from the earlier item to the later one (taken-at order); edges too short on screen get none, and with travel-order numbers on the arrow sits at three quarters of the edge beside the number. Each travel-order number also shows the time of the latest photo reached by that step (the edge's later item), e.g. `3 · 14:05`; when the trajectory spans several days the label adds the day (`3 · 12.07 14:05`, following the user's date format).
- Area selection draws a rectangle and queries the server for contained media: only media whose own GPS point lies inside the rectangle (GPS-less trajectory start/end markers are never part of a selection).
- Results panel: right-side overlay anchored below header+filters on ALL viewports (phone: capped at `min(320px, 82vw)` so it never covers the screen).
- Clicking a result opens the viewer paging through EXACTLY the selected items in selected order; the selection is handed over verbatim (sessionStorage key `media-library-map-selection`, URL param `list=`).
- Map tile sources per theme (light/dark) with an admin-supplied CARTO API key; per-user "Maximum zoom" (1–24, `mapMaxZoom` in the settings API, default 19) caps how far the map may zoom. Recommended: 19 for OpenStreetMap, 20 for CARTO and Esri (the provider's last native tile level; higher values just upscale those tiles).

## Thumbnails & metadata

- Thumbnail files live at `THUMBNAIL_DIR/media/<id/1000>/<id>_<index>.jpg`; folder covers at `THUMBNAIL_DIR/folders/<id/1000>/<id>_0.jpg` (3-way ffmpeg hstack JPEGs).
- Default-thumb pictures per kind; refresh flows support "missing only" and "recreate existing". Metadata refresh's "create existing" rewrites metadata only; use thumbnail refresh to regenerate image files.
- Recreated media thumbnails use the source media's stored orientation metadata, do not apply per-user viewer adjustments, clear any prior thumbnail error and retry, and feed the rendered media thumbnails into folder covers. Thumbnail responses are private but revalidate so replacements appear immediately instead of remaining cached under stable URLs.
- Image thumbnails take only the first frame, so animated GIFs (and other multi-frame images) thumbnail like a still instead of failing in ffmpeg.
- Original media content is privately cached for prefetching but revalidates with the source file modification time, so external file edits are visible without a long stale-content lease.

## Watch folders

- Opt-in per library (`watch` flag on `libraries`, off by default), toggled via the admin library editor ("Watch for changes"). One toggle covers every root of the library; there is no per-root flag.
- A watched library has all of its roots watched recursively with fsnotify; file/dir events debounce (~3s) into one incremental rescan of that library using the regular scan job (duplicate guard applies).
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
- Form and dialog errors (validation like an unparsable GPS, failed requests, ZIP/refresh failures) use one common format: an `ErrorPopup` toast in a fixed stack at the bottom centre of the screen, above everything, so it never overlaps or shifts the form's controls; it closes itself after 10 s or on its × button and clears the owner's error state. Persistent states (a failed settings load, a job's error, a missing reset token) stay inline.
- User settings, grouped into tabs (Appearance, Browsing, Playback, Map, Account): UI zoom, stream chunk size, date format, video fallback codec, show transcoded reason, default thumbnails, map tile sources per light/dark theme, map maximum zoom.
- Play-mode video badge (bottom left, above the controls, opens upward): orange "Transcoded" listing why the original cannot play, or green "Direct play"; hidden when "Show transcoded reason" is off.
- Languages: UI is authored in English; a per-user setting (`auto` or a fixed language: `en`, `ua`, `de`, `nl`, `fi`, `sv`, `pl`, `che` (Czech), `slo` (Slovak), `hu`, `es`, `it`, `sl` (Slovenian), `no`, `pt`; `auto` follows the browser) applies live translations to all rendered text and common attributes without page reload. New UI strings must keep English as the canonical source and add an entry to every table in `web/src/i18n.ts`. Error and status messages (client validation and backend `problem()` texts) are translated in `web/src/i18n-messages.ts`, and further UI labels, help text, calendar names and attribute templates in `web/src/i18n-ui.ts` (one row per string, all 14 languages); `window.confirm` texts go through `translateMessage`; keys containing `%s` are templates (e.g. `Map of %s`, `ZIP download failed: %s`) whose arguments are translated again.
- Network settings toggle HTTP/HTTPS; gateway Caddyfile regenerates and hot-reloads.

## Android application

- Native Android app built with Capacitor (v7) and bundled as an APK via `deploy/Dockerfile.android`.
- On first launch the app shows a **server picker** that asks for the self-hosted server URL (stored in `localStorage` as `ml.server.url`); once validated, the WebView navigates to that origin and the existing cookie-based auth takes over.
- Cleartext HTTP is allowed so the app can reach LAN servers that do not have TLS.
- Release signing reads from a gitignored `web/android/keystore.properties`; when absent the APK is signed with the debug key.
- Build: `sh deploy/start.sh android` (runs inside a container; host needs only Docker, Java 25 optional, and `~/keystrokes/fmiw-release.jks` for release signing).

## Runtime layout

- Single bind mount `./runtime`: DB (`app-data`), generated Caddyfile + logs (`app-config`), Caddy ACME state, thumbnails. Back up this directory.
- Backend container runs as an unprivileged host user resolved from `MEDIA_UID`/`MEDIA_GID`.
