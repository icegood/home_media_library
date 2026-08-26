package watcher

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"

	"media-library/backend/internal/domain"
)

type stubStore struct {
	mu    sync.Mutex
	roots []domain.WatchedRoot
	libs  map[int]domain.Library
}

func (s *stubStore) WatchedRoots(ctx context.Context) ([]domain.WatchedRoot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.WatchedRoot(nil), s.roots...), nil
}

func (s *stubStore) Library(ctx context.Context, id int) (domain.Library, error) {
	if lib, ok := s.libs[id]; ok {
		return lib, nil
	}
	return domain.Library{ID: id}, nil
}

func (s *stubStore) setRoots(roots []domain.WatchedRoot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.roots = roots
}

type stubStarter struct {
	mu     sync.Mutex
	starts []int
}

func (s *stubStarter) StartScan(library domain.Library) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.starts = append(s.starts, library.ID)
	return nil
}

func (s *stubStarter) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.starts)
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}

func TestWatcherTriggersDebouncedScan(t *testing.T) {
	dir := t.TempDir()
	store := &stubStore{roots: []domain.WatchedRoot{{LibraryID: 7, Path: dir}}, libs: map[int]domain.Library{7: {ID: 7, Name: "watched"}}}
	starter := &stubStarter{}
	w := New(store, starter)
	w.SetDebounce(80 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)
	waitFor(t, 3*time.Second, func() bool { return w.ownerOf(dir) == 7 }) // watch registered

	os.WriteFile(filepath.Join(dir, "a.jpg"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.jpg"), []byte("x"), 0o644) // same debounce window
	waitFor(t, 3*time.Second, func() bool { return starter.count() >= 1 })
	time.Sleep(2 * w.debounce)
	if got := starter.count(); got != 1 {
		t.Fatalf("debounce collapsed %d events into %d scans", 2, got)
	}
}

func TestWatcherStopsWhenFlagCleared(t *testing.T) {
	dir := t.TempDir()
	store := &stubStore{roots: []domain.WatchedRoot{{LibraryID: 7, Path: dir}}, libs: map[int]domain.Library{7: {ID: 7}}}
	starter := &stubStarter{}
	w := New(store, starter)
	w.SetDebounce(60 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)
	waitFor(t, 3*time.Second, func() bool { return w.ownerOf(dir) == 7 })

	store.setRoots(nil)
	w.Refresh()
	waitFor(t, 3*time.Second, func() bool { return w.ownerOf(dir) == 0 })

	os.WriteFile(filepath.Join(dir, "late.jpg"), []byte("x"), 0o644)
	time.Sleep(500 * time.Millisecond)
	if got := starter.count(); got != 0 {
		t.Fatalf("scan triggered after unwatching: %d calls", got)
	}
}

func TestWatcherKeepsSubtreeAcrossResync(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	store := &stubStore{roots: []domain.WatchedRoot{{LibraryID: 7, Path: root}}}
	w := New(store, &stubStarter{})
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	defer fsw.Close()

	if err := w.sync(context.Background(), fsw); err != nil {
		t.Fatal(err)
	}
	if got := w.ownerOf(sub); got != 7 {
		t.Fatalf("subtree not watched after initial sync: ownerOf=%d, want 7", got)
	}

	// resync must keep subtree watches; desired only lists the root path.
	if err := w.sync(context.Background(), fsw); err != nil {
		t.Fatal(err)
	}
	if got := w.ownerOf(sub); got != 7 {
		t.Fatalf("subtree watch dropped after resync: ownerOf=%d, want 7", got)
	}
}
