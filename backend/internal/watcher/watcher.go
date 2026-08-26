// Package watcher triggers incremental library rescans when files change on
// disk inside opted-in library roots. Watching is off by default per root and
// managed through the library_roots.watch flag.
package watcher

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"media-library/backend/internal/domain"
)

type Store interface {
	WatchedRoots(ctx context.Context) ([]domain.WatchedRoot, error)
	Library(ctx context.Context, id int) (domain.Library, error)
}

type Starter interface {
	StartScan(library domain.Library) error
}

type Watcher struct {
	store    Store
	starter  Starter
	debounce time.Duration
	resync   time.Duration

	mu      sync.Mutex
	watched map[string]int // dir path -> owning library id
	timers  map[int]*time.Timer
	refresh chan struct{}
}

func New(store Store, starter Starter) *Watcher {
	return &Watcher{
		store:    store,
		starter:  starter,
		debounce: 3 * time.Second,
		resync:   30 * time.Second,
		watched:  map[string]int{},
		timers:   map[int]*time.Timer{},
		refresh:  make(chan struct{}, 1),
	}
}

func (w *Watcher) SetDebounce(d time.Duration) { w.debounce = d }

// Refresh asks the watcher to re-read the watched-root configuration
// (non-blocking; safe to call from request handlers).
func (w *Watcher) Refresh() {
	select {
	case w.refresh <- struct{}{}:
	default:
	}
}

func (w *Watcher) Run(ctx context.Context) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("watcher: cannot create fsnotify watcher: %v", err)
		return
	}
	defer fsw.Close()
	if err := w.sync(ctx, fsw); err != nil {
		log.Printf("watcher: initial sync failed: %v", err)
	}
	ticker := time.NewTicker(w.resync)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			w.clearTimers()
			return
		case event, ok := <-fsw.Events:
			if !ok {
				return
			}
			w.handleEvent(ctx, fsw, event)
		case err, ok := <-fsw.Errors:
			if !ok {
				return
			}
			log.Printf("watcher: fsnotify error: %v", err)
		case <-ticker.C:
			if err := w.sync(ctx, fsw); err != nil {
				log.Printf("watcher: resync failed: %v", err)
			}
		case <-w.refresh:
			if err := w.sync(ctx, fsw); err != nil {
				log.Printf("watcher: refresh sync failed: %v", err)
			}
		}
	}
}

// sync reconciles the active watch set with the configured roots.
func (w *Watcher) sync(ctx context.Context, fsw *fsnotify.Watcher) error {
	roots, err := w.store.WatchedRoots(ctx)
	if err != nil {
		return err
	}
	desired := map[string]int{}
	for _, root := range roots {
		desired[filepath.Clean(root.Path)] = root.LibraryID
	}
	w.mu.Lock()
	sep := string(filepath.Separator)
	for path := range w.watched {
		keep := false
		for rootPath := range desired {
			if path == rootPath || strings.HasPrefix(path, rootPath+sep) {
				keep = true
				break
			}
		}
		if !keep {
			_ = fsw.Remove(path)
			delete(w.watched, path)
		}
	}
	w.mu.Unlock()
	for path, libID := range desired {
		w.mu.Lock()
		_, exists := w.watched[path]
		w.mu.Unlock()
		if exists {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			continue // root not mounted yet; retried on next resync
		}
		if err := w.addTree(fsw, path, libID); err != nil {
			log.Printf("watcher: watching %q: %v", path, err)
		}
	}
	return nil
}

// addTree registers watches for root and every subdirectory below it.
func (w *Watcher) addTree(fsw *fsnotify.Watcher, root string, libID int) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries are skipped, not fatal
		}
		if !d.IsDir() {
			return nil
		}
		if err := fsw.Add(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			log.Printf("watcher: watch %q: %v", path, err)
			return nil
		}
		w.mu.Lock()
		w.watched[path] = libID
		w.mu.Unlock()
		return nil
	})
}

func (w *Watcher) handleEvent(ctx context.Context, fsw *fsnotify.Watcher, event fsnotify.Event) {
	if event.Has(fsnotify.Chmod) {
		return
	}
	if event.Has(fsnotify.Create) {
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			libID := w.ownerOf(event.Name)
			if libID > 0 {
				_ = w.addTree(fsw, event.Name, libID)
			}
		}
	}
	if event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
		w.mu.Lock()
		for path := range w.watched {
			if path == event.Name || strings.HasPrefix(path, event.Name+string(filepath.Separator)) {
				_ = fsw.Remove(path)
				delete(w.watched, path)
			}
		}
		w.mu.Unlock()
	}
	libID := w.ownerOf(event.Name)
	if libID > 0 {
		w.schedule(ctx, libID)
	}
}

// ownerOf resolves the deepest tracked ancestor directory for a path.
func (w *Watcher) ownerOf(name string) int {
	name = filepath.Clean(name)
	best := 0
	bestLen := -1
	w.mu.Lock()
	defer w.mu.Unlock()
	for path, libID := range w.watched {
		if name == path || strings.HasPrefix(name, path+string(filepath.Separator)) {
			if len(path) > bestLen {
				bestLen = len(path)
				best = libID
			}
		}
	}
	return best
}

// schedule (re-)arms the debounce timer for a library; only the last event
// within the debounce window triggers a scan.
func (w *Watcher) schedule(ctx context.Context, libraryID int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if timer, ok := w.timers[libraryID]; ok {
		timer.Reset(w.debounce)
		return
	}
	w.timers[libraryID] = time.AfterFunc(w.debounce, func() {
		w.mu.Lock()
		delete(w.timers, libraryID)
		w.mu.Unlock()
		w.trigger(ctx, libraryID)
	})
}

func (w *Watcher) trigger(ctx context.Context, libraryID int) {
	library, err := w.store.Library(ctx, libraryID)
	if err != nil {
		log.Printf("watcher: load library %d: %v", libraryID, err)
		return
	}
	if err := w.starter.StartScan(library); err != nil {
		log.Printf("watcher: start scan for library %d: %v", libraryID, err)
		return
	}
	log.Printf("watcher: filesystem changes detected, scan started for library %d (%s)", library.ID, library.Name)
}

func (w *Watcher) clearTimers() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for id, timer := range w.timers {
		timer.Stop()
		delete(w.timers, id)
	}
}
