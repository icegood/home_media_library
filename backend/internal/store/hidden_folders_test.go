package store_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"media-library/backend/internal/domain"
	"media-library/backend/internal/store"
)

// checkHiddenFolders runs the same show_as_child checks against any store:
// a hidden subfolder is left out of its parent's listing and subtree media
// (but still listed when its own subtree is asked for), out of library-wide
// media, and appears in HiddenFolders until shown again. Jobs (MediaForSubtree)
// keep seeing everything.
func checkHiddenFolders(t *testing.T, repository store.Store) {
	t.Helper()
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "photos")
	library, err := repository.CreateLibrary(ctx, domain.Library{ID: domain.InvalidID, Name: "Photos", Roots: []domain.LibraryRoot{{ID: domain.InvalidID, Path: root}}})
	if err != nil {
		t.Fatal(err)
	}
	rootID := library.Roots[0].ID
	open, err := repository.UpsertFolder(ctx, domain.MediaFolder{ID: domain.InvalidID, ParentID: rootID, Path: filepath.Join(root, "open"), RelativePath: "open"})
	if err != nil {
		t.Fatal(err)
	}
	private, err := repository.UpsertFolder(ctx, domain.MediaFolder{ID: domain.InvalidID, ParentID: rootID, Path: filepath.Join(root, "private"), RelativePath: "private"})
	if err != nil {
		t.Fatal(err)
	}
	add := func(folder domain.MediaFolder, name string) domain.Media {
		item, err := repository.UpsertMedia(ctx, domain.Media{ID: domain.InvalidID, FolderID: folder.ID, Path: filepath.Join(folder.Path, name), Name: name, Kind: domain.KindImage, MIMEType: "image/jpeg", TakenAt: "2025-01-01T00:00:00Z"})
		if err != nil {
			t.Fatal(err)
		}
		return item
	}
	visible := add(open, "a.jpg")
	secret := add(private, "b.jpg")
	ids := func(items []domain.Media) map[int]bool {
		out := map[int]bool{}
		for _, item := range items {
			out[item.ID] = true
		}
		return out
	}
	if err := repository.SetFolderShowAsChild(ctx, private.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := repository.SetFolderShowAsChild(ctx, 999999, false); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("hiding a missing folder = %v, want ErrNotFound", err)
	}
	entries, err := repository.EntriesForFolder(ctx, 0, library.ID, rootID)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries.Entries {
		if entry.Type == "folder" && entry.ID == private.ID {
			t.Fatal("hidden folder listed in its parent")
		}
	}
	for name, load := range map[string]func() ([]domain.Media, error){
		"library": func() ([]domain.Media, error) { return repository.MediaForLibrary(ctx, 0, library.ID) },
		"root":    func() ([]domain.Media, error) { return repository.MediaForFolder(ctx, 0, library.ID, rootID) },
	} {
		items, err := load()
		if err != nil {
			t.Fatal(err)
		}
		if got := ids(items); got[secret.ID] || !got[visible.ID] {
			t.Fatalf("%s media = %v, want the hidden one left out", name, got)
		}
	}
	own, err := repository.MediaForFolder(ctx, 0, library.ID, private.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ids(own)[secret.ID] {
		t.Fatal("a hidden folder opened directly must list its own media")
	}
	all, err := repository.MediaForSubtree(ctx, rootID)
	if err != nil {
		t.Fatal(err)
	}
	if !ids(all)[secret.ID] {
		t.Fatal("jobs must still see media in hidden folders")
	}
	if ok, err := repository.CanReadFolder(ctx, 0, private.ID, true); err != nil || !ok {
		t.Fatalf("admin CanReadFolder = %v, %v", ok, err)
	}
	hidden, err := repository.HiddenFolders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(hidden) != 1 || hidden[0].ID != private.ID || hidden[0].LibraryID != library.ID || hidden[0].LibraryName != "Photos" || hidden[0].RelativePath != "photos/private" || hidden[0].Name != "private" {
		t.Fatalf("hidden folders = %#v", hidden)
	}
	if err := repository.SetFolderShowAsChild(ctx, private.ID, true); err != nil {
		t.Fatal(err)
	}
	items, err := repository.MediaForLibrary(ctx, 0, library.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ids(items)[secret.ID] {
		t.Fatal("a shown-again folder must be back in library media")
	}
	if hidden, _ := repository.HiddenFolders(ctx); len(hidden) != 0 {
		t.Fatalf("hidden folders after unhide = %#v", hidden)
	}
}

func TestSQLiteHiddenFolders(t *testing.T) {
	repository, err := store.NewSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repository.Close() })
	checkHiddenFolders(t, repository)
}

func TestPostgresHiddenFolders(t *testing.T) {
	checkHiddenFolders(t, openPostgres(t, true))
}
