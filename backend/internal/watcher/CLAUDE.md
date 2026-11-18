# Folder watcher (`internal/watcher`)

- Opt-in **per library** (`libraries.watch`, off by default) — not per root.
- fsnotify watches every root of an opted-in library recursively; new
  subdirectories are added as they appear.
- Change bursts are debounced into **one incremental rescan** of that library via
  the scanner job path (duplicate-job guard applies).
- Runs on the job store handle. Library create/update calls
  `OnLibrariesChanged` → `watcher.Refresh`, so toggling `watch` starts/stops
  watching without a restart — keep that hook on any new library mutation.
