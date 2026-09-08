package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"media-library/backend/internal/domain"
	"media-library/backend/internal/store"
)

// seedSubtreeFixture builds one library layout shared by the SQLite and
// Postgres subtree regressions: a root holding root.jpg, a "trip" folder with
// one.jpg and a nested deep/two.jpg, and an unrelated "other" folder with
// three.jpg. Returns the root folder id and the trip folder id.
func seedSubtreeFixture(t *testing.T, repository store.Store) (rootFolderID, tripID int) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "photos")
	library := domain.Library{ID: domain.InvalidID, Name: "Photos", Roots: []domain.LibraryRoot{{ID: domain.InvalidID, Path: root}}}
	var err error
	library, err = repository.CreateLibrary(context.Background(), library)
	if err != nil {
		t.Fatal(err)
	}
	rootFolderID = library.Roots[0].ID
	trip, err := repository.UpsertFolder(context.Background(), domain.MediaFolder{ID: domain.InvalidID, ParentID: rootFolderID, Path: filepath.Join(root, "trip"), RelativePath: "trip"})
	if err != nil {
		t.Fatal(err)
	}
	deep, err := repository.UpsertFolder(context.Background(), domain.MediaFolder{ID: domain.InvalidID, ParentID: trip.ID, Path: filepath.Join(root, "trip", "deep"), RelativePath: "trip/deep"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := repository.UpsertFolder(context.Background(), domain.MediaFolder{ID: domain.InvalidID, ParentID: rootFolderID, Path: filepath.Join(root, "other"), RelativePath: "other"})
	if err != nil {
		t.Fatal(err)
	}
	for _, seed := range []struct {
		folderID int
		path     string
		name     string
	}{
		{rootFolderID, filepath.Join(root, "root.jpg"), "root.jpg"},
		{trip.ID, filepath.Join(root, "trip", "one.jpg"), "one.jpg"},
		{deep.ID, filepath.Join(root, "trip", "deep", "two.jpg"), "two.jpg"},
		{other.ID, filepath.Join(root, "other", "three.jpg"), "three.jpg"},
	} {
		if _, err := repository.UpsertMedia(context.Background(), domain.Media{ID: domain.InvalidID, FolderID: seed.folderID, Path: seed.path, Name: seed.name, Kind: domain.KindImage, MIMEType: "image/jpeg"}); err != nil {
			t.Fatal(err)
		}
	}
	return rootFolderID, trip.ID
}

// verifyMediaForSubtree asserts the folder-scoped job query scans every
// column of every row (the subtree query and the media scan destinations must
// agree), stays inside the subtree, and returns paths relative to the scope
// folder itself.
func verifyMediaForSubtree(t *testing.T, repository store.Store) {
	t.Helper()
	rootFolderID, tripID := seedSubtreeFixture(t, repository)
	items, err := repository.MediaForSubtree(context.Background(), tripID)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, item := range items {
		got[item.RelativePath] = true
	}
	if len(items) != 2 || !got["one.jpg"] || !got["deep/two.jpg"] {
		t.Fatalf("subtree media = %#v", items)
	}
	all, err := repository.MediaForSubtree(context.Background(), rootFolderID)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 {
		t.Fatalf("whole-library subtree media = %d, want 4", len(all))
	}
}
