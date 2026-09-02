package store_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"media-library/backend/internal/domain"
	"media-library/backend/internal/store"
)

func openSQLite(t *testing.T) (*store.SQLite, string) {
	t.Helper()
	dbFile := filepath.Join(t.TempDir(), "media-library.db")
	repository, err := store.NewSQLite(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repository.Close() })
	return repository, dbFile
}

func TestSQLiteInitialAdminPersistsAcrossRestart(t *testing.T) {
	repository, dbFile := openSQLite(t)
	if required, err := repository.SetupRequired(context.Background()); err != nil || !required {
		t.Fatalf("setup should be required on empty db: required=%v err=%v", required, err)
	}
	if _, err := repository.CreateInitialAdmin(context.Background(), domain.User{
		ID: domain.InvalidID, Login: "owner",
	}, "a-secure-password"); err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := store.NewSQLite(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	required, err := restarted.SetupRequired(context.Background())
	if err != nil || required {
		t.Fatalf("setup reopened after restart: required=%v err=%v", required, err)
	}
	user, err := restarted.Authenticate(context.Background(), "owner", "a-secure-password")
	if err != nil || user.Role != domain.RoleAdmin {
		t.Fatalf("persisted login failed: user=%#v err=%v", user, err)
	}
	if _, err := restarted.CreateInitialAdmin(context.Background(), domain.User{
		ID: domain.InvalidID, Login: "intruder",
	}, "x"); err != store.ErrConflict {
		t.Fatalf("second admin should be rejected: err=%v", err)
	}
}

func TestSQLiteApplicationSettingsPersistAcrossRestart(t *testing.T) {
	repository, dbFile := openSQLite(t)
	settings, err := repository.ServerSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.SMTPHost = "smtp.example.com"
	if err := repository.SaveServerSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := store.NewSQLite(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	settings, err = restarted.ServerSettings(context.Background())
	if err != nil || settings.SMTPHost != "smtp.example.com" {
		t.Fatalf("persisted smtpHost = %q, err=%v", settings.SMTPHost, err)
	}
}

func TestSQLiteBackgroundJobsPersistAcrossRestart(t *testing.T) {
	repository, dbFile := openSQLite(t)
	library, err := repository.CreateLibrary(context.Background(), domain.Library{Name: "Trips"})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC()
	job := domain.BackgroundJob{
		ID: "job-1", Category: "scan", Type: "scan", LibraryID: library.ID, LibraryName: library.Name,
		Status: "running", Cancelable: true, CurrentPath: "/media/a", Processed: 7, Total: 20,
		StartedAt: started, Options: map[string]any{"recreateExisting": false},
	}
	if err := repository.SaveJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := store.NewSQLite(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	jobs, err := restarted.UnfinishedJobs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ID != "job-1" || jobs[0].Processed != 7 || jobs[0].Status != "running" {
		t.Fatalf("unexpected persisted jobs: %#v", jobs)
	}
	finished := started.Add(time.Minute)
	jobs[0].Status = "done"
	jobs[0].Cancelable = false
	jobs[0].FinishedAt = &finished
	if err := restarted.SaveJob(context.Background(), jobs[0]); err != nil {
		t.Fatal(err)
	}
	if err := restarted.DeleteFinishedJobsBefore(context.Background(), finished.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	jobs, err = restarted.Jobs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Fatalf("finished job was not pruned: %#v", jobs)
	}
}

func TestSQLiteLibraryStatePersistsInternalPathsAcrossRestart(t *testing.T) {
	repository, dbFile := openSQLite(t)
	mediaRoot := t.TempDir()
	root := filepath.Join(mediaRoot, "photos")
	folderPath := filepath.Join(root, "Camera")
	mediaPath := filepath.Join(folderPath, "one.jpg")
	library := domain.Library{ID: domain.InvalidID, Name: "Photos", Roots: []domain.LibraryRoot{{ID: domain.InvalidID, Path: root}}}
	library, err := repository.CreateLibrary(context.Background(), library)
	if err != nil {
		t.Fatal(err)
	}
	folder, err := repository.UpsertFolder(context.Background(), domain.MediaFolder{ID: domain.InvalidID, ParentID: library.Roots[0].ID, Path: folderPath, RelativePath: "Camera"})
	if err != nil {
		t.Fatal(err)
	}
	media, err := repository.UpsertMedia(context.Background(), domain.Media{
		ID: domain.InvalidID, FolderID: folder.ID, Path: mediaPath, RelativePath: "Camera/one.jpg",
		Name: "one.jpg", Kind: domain.KindImage, MIMEType: "image/jpeg",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := store.NewSQLite(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	loadedFolder, err := restarted.Folder(context.Background(), folder.ID)
	if err != nil {
		t.Fatal(err)
	}
	loadedMedia, err := restarted.Media(context.Background(), media.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedFolder.Path != folderPath || loadedMedia.Path != mediaPath {
		t.Fatalf("paths not persisted: folder=%q media=%q", loadedFolder.Path, loadedMedia.Path)
	}
	if loadedMedia.Kind != domain.KindImage || loadedMedia.MIMEType != "image/jpeg" {
		t.Fatalf("mime/kind not persisted: %#v", loadedMedia)
	}
}

func TestSQLiteImportedEmbySHA1PasswordAuthenticatesAndUpgradesToBcrypt(t *testing.T) {
	repository, dbFile := openSQLite(t)
	result, err := repository.ImportSnapshot(context.Background(), domain.ImportSnapshot{Users: []domain.User{{
		ID: 1, Login: "ice", Role: domain.RoleRegular,
		PasswordHash: "emby-sha1:5baa61e4c9b93f3f0682250b6cf8331b7ee68fd8",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Users) != 1 || result.Users[0].TemporaryPassword != "" {
		t.Fatalf("import should keep Emby password without temporary password: %#v", result.Users)
	}
	if _, err := repository.Authenticate(context.Background(), "ice", "password"); err != nil {
		t.Fatalf("Emby SHA1 login failed: %v", err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := store.NewSQLite(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if _, err := restarted.Authenticate(context.Background(), "ice", "password"); err != nil {
		t.Fatalf("upgraded bcrypt login failed after restart: %v", err)
	}
}

func TestSQLiteCreateLibraryRegistersRootFolderFirst(t *testing.T) {
	repository, _ := openSQLite(t)
	rootPath := filepath.Join(t.TempDir(), "photos")
	library := domain.Library{
		ID: domain.InvalidID, Name: "Photos",
		Roots: []domain.LibraryRoot{{ID: domain.InvalidID, Path: rootPath}},
	}
	var err error
	library, err = repository.CreateLibrary(context.Background(), library)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := repository.Library(context.Background(), library.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Roots) != 1 || stored.Roots[0].ID == domain.InvalidID {
		t.Fatalf("root folder id was not assigned: %#v", stored.Roots)
	}
	folder, err := repository.Folder(context.Background(), stored.Roots[0].ID)
	if err != nil {
		t.Fatalf("root folder was not added to media_folders first: %v", err)
	}
	if folder.Path != rootPath {
		t.Fatalf("folder path = %q, want %q", folder.Path, rootPath)
	}
}

func TestSQLiteLibraryNamesAreUniqueCaseInsensitive(t *testing.T) {
	repository, _ := openSQLite(t)
	first, err := repository.CreateLibrary(context.Background(), domain.Library{Name: "Photos",
		Roots: []domain.LibraryRoot{{ID: domain.InvalidID, Path: filepath.Join(t.TempDir(), "photos")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateLibrary(context.Background(), domain.Library{Name: " photos ",
		Roots: []domain.LibraryRoot{{ID: domain.InvalidID, Path: filepath.Join(t.TempDir(), "other")}},
	}); err != store.ErrConflict {
		t.Fatalf("duplicate library name error = %v, want ErrConflict", err)
	}
	second, err := repository.CreateLibrary(context.Background(), domain.Library{Name: "Videos",
		Roots: []domain.LibraryRoot{{ID: domain.InvalidID, Path: filepath.Join(t.TempDir(), "videos")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	second.Name = "PHOTOS"
	if err := repository.UpdateLibrary(context.Background(), second); err != store.ErrConflict {
		t.Fatalf("duplicate rename error = %v, want ErrConflict", err)
	}
	first.Name = " photos "
	if err := repository.UpdateLibrary(context.Background(), first); err != nil {
		t.Fatalf("same library rename with different casing should be allowed: %v", err)
	}
}

func TestSQLiteRegularLibraryListingDoesNotEraseStoredRootPaths(t *testing.T) {
	repository, _ := openSQLite(t)
	rootPath := filepath.Join(t.TempDir(), "photos")
	library := domain.Library{
		ID: domain.InvalidID, Name: "Photos",
		Roots: []domain.LibraryRoot{{ID: domain.InvalidID, Path: rootPath}},
	}
	var err error
	library, err = repository.CreateLibrary(context.Background(), library)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ImportSnapshot(context.Background(), domain.ImportSnapshot{Users: []domain.User{{
		ID: 1, Login: "regular", Role: domain.RoleRegular,
	}}}); err != nil {
		t.Fatal(err)
	}
	if err := repository.SetAccess(context.Background(), library.ID, 1, true); err != nil {
		t.Fatal(err)
	}
	visible, err := repository.LibrariesForUser(context.Background(), 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) != 1 || visible[0].Roots[0].Path != "" {
		t.Fatalf("regular response should hide root path: %#v", visible)
	}
	stored, err := repository.Library(context.Background(), library.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Roots[0].Path != rootPath {
		t.Fatalf("stored root path was erased by regular listing: %q, want %q", stored.Roots[0].Path, rootPath)
	}
}

func TestSQLiteLibraryStats(t *testing.T) {
	repository, _ := openSQLite(t)
	root := filepath.Join(t.TempDir(), "photos")
	library := domain.Library{ID: domain.InvalidID, Name: "Photos", Roots: []domain.LibraryRoot{{ID: domain.InvalidID, Path: root}}}
	var err error
	library, err = repository.CreateLibrary(context.Background(), library)
	if err != nil {
		t.Fatal(err)
	}
	folder, err := repository.UpsertFolder(context.Background(), domain.MediaFolder{ID: domain.InvalidID, ParentID: library.Roots[0].ID, Path: filepath.Join(root, "Camera"), RelativePath: "Camera"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.UpsertMedia(context.Background(), domain.Media{ID: domain.InvalidID, FolderID: folder.ID, Path: filepath.Join(root, "Camera", "one.jpg"), Name: "one.jpg", Kind: domain.KindImage, MIMEType: "image/jpeg"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.UpsertMedia(context.Background(), domain.Media{ID: domain.InvalidID, FolderID: folder.ID, Path: filepath.Join(root, "Camera", "two.mp4"), Name: "two.mp4", Kind: domain.KindVideo, MIMEType: "video/mp4"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.UpsertMedia(context.Background(), domain.Media{ID: domain.InvalidID, FolderID: folder.ID, Path: filepath.Join(root, "Camera", "readme.pdf"), Name: "readme.pdf", Kind: domain.KindDocument, MIMEType: "application/pdf"}); err != nil {
		t.Fatal(err)
	}
	stats, err := repository.LibraryStats(context.Background(), library.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Images != 1 || stats.Videos != 1 || stats.Documents != 1 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
}

func TestSQLiteLibraryRootWatch(t *testing.T) {
	repository, _ := openSQLite(t)
	rootPath := filepath.Join(t.TempDir(), "photos")
	library := domain.Library{ID: domain.InvalidID, Name: "Watched", Roots: []domain.LibraryRoot{
		{ID: domain.InvalidID, Path: rootPath, Watch: true},
	}}
	created, err := repository.CreateLibrary(context.Background(), library)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := repository.Library(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Roots) != 1 || !stored.Roots[0].Watch {
		t.Fatalf("watch flag not persisted on create: %#v", stored.Roots)
	}
	watched, err := repository.WatchedRoots(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(watched) != 1 || watched[0].LibraryID != created.ID || watched[0].Path == "" {
		t.Fatalf("unexpected watched roots: %#v", watched)
	}
	// Turning the flag off must remove it from the watched set.
	stored.Roots[0].Watch = false
	if err := repository.UpdateLibrary(context.Background(), stored); err != nil {
		t.Fatal(err)
	}
	watched, err = repository.WatchedRoots(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(watched) != 0 {
		t.Fatalf("watch flag not cleared: %#v", watched)
	}
}

func TestSQLiteDeleteLibraryRemovesOnlyUnsharedMedia(t *testing.T) {
	repository, _ := openSQLite(t)
	root := t.TempDir()
	shared := filepath.Join(root, "shared")
	private := filepath.Join(root, "private")
	first := domain.Library{ID: domain.InvalidID, Name: "First", Roots: []domain.LibraryRoot{{ID: domain.InvalidID, Path: shared}, {ID: domain.InvalidID, Path: private}}}
	second := domain.Library{ID: domain.InvalidID, Name: "Second", Roots: []domain.LibraryRoot{{ID: domain.InvalidID, Path: shared}}}
	var err error
	first, err = repository.CreateLibrary(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	second, err = repository.CreateLibrary(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	first, _ = repository.Library(context.Background(), first.ID)
	second, _ = repository.Library(context.Background(), second.ID)
	var sharedRootID, privateRootID int
	for _, root := range first.Roots {
		if root.Path == shared {
			sharedRootID = root.ID
		}
		if root.Path == private {
			privateRootID = root.ID
		}
	}
	if sharedRootID == 0 || privateRootID == 0 {
		t.Fatalf("could not locate roots: %#v", first.Roots)
	}
	if sharedRootID != second.Roots[0].ID {
		t.Fatalf("shared root must have same folder id: %d != %d", sharedRootID, second.Roots[0].ID)
	}
	sharedMedia, err := repository.UpsertMedia(context.Background(), domain.Media{ID: domain.InvalidID, FolderID: sharedRootID, Path: filepath.Join(shared, "shared.jpg"), Name: "shared.jpg", Kind: domain.KindImage, MIMEType: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	privateMedia, err := repository.UpsertMedia(context.Background(), domain.Media{ID: domain.InvalidID, FolderID: privateRootID, Path: filepath.Join(private, "private.jpg"), Name: "private.jpg", Kind: domain.KindImage, MIMEType: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.DeleteLibrary(context.Background(), first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Media(context.Background(), sharedMedia.ID); err != nil {
		t.Fatalf("shared media should remain because another library owns its root: %v", err)
	}
	if _, err := repository.Media(context.Background(), privateMedia.ID); err == nil {
		t.Fatal("private media should be deleted with its only library")
	}
}

func TestSQLiteFavoriteViews(t *testing.T) {
	repository, _ := openSQLite(t)
	root := filepath.Join(t.TempDir(), "photos")
	library := domain.Library{ID: domain.InvalidID, Name: "Photos", Roots: []domain.LibraryRoot{{ID: domain.InvalidID, Path: root}}}
	var err error
	library, err = repository.CreateLibrary(context.Background(), library)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ImportSnapshot(context.Background(), domain.ImportSnapshot{Users: []domain.User{{
		ID: 1, Login: "regular", Role: domain.RoleRegular,
	}}}); err != nil {
		t.Fatal(err)
	}
	if err := repository.SetAccess(context.Background(), library.ID, 1, true); err != nil {
		t.Fatal(err)
	}
	media, err := repository.UpsertMedia(context.Background(), domain.Media{ID: domain.InvalidID, FolderID: library.Roots[0].ID, Path: filepath.Join(root, "one.jpg"), Name: "one.jpg", Kind: domain.KindImage, MIMEType: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	view, err := repository.CreateFavoriteView(context.Background(), 1, "My Favorites")
	if err != nil {
		t.Fatal(err)
	}
	favorite, err := repository.SetFavorite(context.Background(), 1, view.ID, media.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if !favorite.Favorite {
		t.Fatalf("media not marked favorite: %#v", favorite)
	}
	isFavorite, err := repository.IsFavorite(context.Background(), 1, media.ID)
	if err != nil || !isFavorite {
		t.Fatalf("IsFavorite = %v, err=%v", isFavorite, err)
	}
	views, err := repository.FavoriteViews(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].Name != "My Favorites" || views[0].Count != 1 {
		t.Fatalf("unexpected views: %#v", views)
	}
	list, err := repository.FavoriteMedia(context.Background(), 1, view.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != media.ID {
		t.Fatalf("favorite media list = %#v", list)
	}
	if _, err := repository.SetFavorite(context.Background(), 1, view.ID, media.ID, false); err != nil {
		t.Fatal(err)
	}
	isFavorite, err = repository.IsFavorite(context.Background(), 1, media.ID)
	if err != nil || isFavorite {
		t.Fatalf("media should no longer be favorite: %v, err=%v", isFavorite, err)
	}
}

func TestSQLiteMediaForLibraryFavoriteFlags(t *testing.T) {
	repository, _ := openSQLite(t)
	root := filepath.Join(t.TempDir(), "photos")
	library := domain.Library{ID: domain.InvalidID, Name: "Family", Roots: []domain.LibraryRoot{{ID: domain.InvalidID, Path: root}}}
	library, err := repository.CreateLibrary(context.Background(), library)
	if err != nil {
		t.Fatal(err)
	}
	mediaIDs := []int{}
	for _, name := range []string{"one.jpg", "two.jpg", "three.jpg"} {
		media, err := repository.UpsertMedia(context.Background(), domain.Media{ID: domain.InvalidID, FolderID: library.Roots[0].ID, Path: filepath.Join(root, name), Name: name, Kind: domain.KindImage, MIMEType: "image/jpeg"})
		if err != nil {
			t.Fatal(err)
		}
		mediaIDs = append(mediaIDs, media.ID)
	}
	if _, err := repository.ImportSnapshot(context.Background(), domain.ImportSnapshot{Users: []domain.User{{
		ID: 1, Login: "regular", Role: domain.RoleRegular,
	}}}); err != nil {
		t.Fatal(err)
	}
	if err := repository.SetAccess(context.Background(), library.ID, 1, true); err != nil {
		t.Fatal(err)
	}
	view, err := repository.CreateFavoriteView(context.Background(), 1, "My Favorites")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.SetFavorite(context.Background(), 1, view.ID, mediaIDs[0], true); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.SetFavorite(context.Background(), 1, view.ID, mediaIDs[2], true); err != nil {
		t.Fatal(err)
	}
	items, err := repository.MediaForLibrary(context.Background(), 1, library.ID)
	if err != nil {
		t.Fatal(err)
	}
	favorites := map[int]bool{}
	for _, item := range items {
		if item.Favorite {
			favorites[item.ID] = true
		}
	}
	if len(favorites) != 2 || !favorites[mediaIDs[0]] || favorites[mediaIDs[1]] || !favorites[mediaIDs[2]] {
		t.Fatalf("favorite flags from MediaForLibrary = %#v", favorites)
	}
	membership, err := repository.FavoriteViewsForMedia(context.Background(), 1, mediaIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(membership) != 1 || !membership[0].Contains || membership[0].ID != view.ID || membership[0].Count != 2 {
		t.Fatalf("favorite view membership for favorite media = %#v", membership)
	}
	membership, err = repository.FavoriteViewsForMedia(context.Background(), 1, mediaIDs[1])
	if err != nil {
		t.Fatal(err)
	}
	if len(membership) != 1 || membership[0].Contains {
		t.Fatalf("favorite view membership for non-favorite media = %#v", membership)
	}
}

func TestSQLiteMediaBatchAndFoldersByIDs(t *testing.T) {
	repository, _ := openSQLite(t)
	root := filepath.Join(t.TempDir(), "photos")
	library := domain.Library{ID: domain.InvalidID, Name: "Photos", Roots: []domain.LibraryRoot{{ID: domain.InvalidID, Path: root}}}
	var err error
	library, err = repository.CreateLibrary(context.Background(), library)
	if err != nil {
		t.Fatal(err)
	}
	subfolder, err := repository.UpsertFolder(context.Background(), domain.MediaFolder{
		ID: domain.InvalidID, ParentID: library.Roots[0].ID, Path: filepath.Join(root, "Camera"), RelativePath: "Camera",
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := repository.UpsertMedia(context.Background(), domain.Media{
		ID: domain.InvalidID, FolderID: subfolder.ID, Path: filepath.Join(root, "Camera", "one.jpg"), Name: "one.jpg", Kind: domain.KindImage, MIMEType: "image/jpeg",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.UpsertMedia(context.Background(), domain.Media{
		ID: domain.InvalidID, FolderID: subfolder.ID, Path: filepath.Join(root, "Camera", "two.jpg"), Name: "two.jpg", Kind: domain.KindImage, MIMEType: "image/jpeg",
	})
	if err != nil {
		t.Fatal(err)
	}
	batch, err := repository.MediaBatch(context.Background(), []int{first.ID, second.ID, first.ID, 999999})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch) != 2 {
		t.Fatalf("MediaBatch = %d items, want 2 (missing and duplicate ids skipped)", len(batch))
	}
	byID := map[int]domain.Media{}
	for _, item := range batch {
		byID[item.ID] = item
	}
	if byID[first.ID].Name != "one.jpg" || byID[second.ID].RelativePath != "Camera/two.jpg" {
		t.Fatalf("MediaBatch contents = %#v", batch)
	}
	if empty, err := repository.MediaBatch(context.Background(), nil); err != nil || len(empty) != 0 {
		t.Fatalf("MediaBatch(nil) = %#v, err=%v", empty, err)
	}
	folders, err := repository.FoldersByIDs(context.Background(), []int{subfolder.ID, library.Roots[0].ID, subfolder.ID, 999999})
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 2 || folders[subfolder.ID].Path != filepath.Join(root, "Camera") || folders[library.Roots[0].ID].Name == "" {
		t.Fatalf("FoldersByIDs = %#v", folders)
	}
	if empty, err := repository.FoldersByIDs(context.Background(), nil); err != nil || len(empty) != 0 {
		t.Fatalf("FoldersByIDs(nil) = %#v, err=%v", empty, err)
	}
}

func TestSQLiteFolderChain(t *testing.T) {
	repository, _ := openSQLite(t)
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "photos")
	library := domain.Library{ID: domain.InvalidID, Name: "Photos", Roots: []domain.LibraryRoot{{ID: domain.InvalidID, Path: root}}}
	library, err := repository.CreateLibrary(ctx, library)
	if err != nil {
		t.Fatal(err)
	}
	rootFolder := library.Roots[0]
	camera, err := repository.UpsertFolder(ctx, domain.MediaFolder{
		ID: domain.InvalidID, ParentID: rootFolder.ID, Path: filepath.Join(root, "Camera"), RelativePath: "Camera",
	})
	if err != nil {
		t.Fatal(err)
	}
	day1, err := repository.UpsertFolder(ctx, domain.MediaFolder{
		ID: domain.InvalidID, ParentID: camera.ID, Path: filepath.Join(root, "Camera", "Day1"), RelativePath: "Camera/Day1",
	})
	if err != nil {
		t.Fatal(err)
	}
	chain, err := repository.FolderChain(ctx, library.ID, day1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(chain) != 3 {
		t.Fatalf("FolderChain = %d items, want 3", len(chain))
	}
	if chain[0].ID != rootFolder.ID || chain[1].ID != camera.ID || chain[2].ID != day1.ID {
		t.Fatalf("FolderChain order = [%d %d %d], want [%d %d %d]", chain[0].ID, chain[1].ID, chain[2].ID, rootFolder.ID, camera.ID, day1.ID)
	}
	if chain[0].ParentID != domain.InvalidID || chain[2].ParentID != camera.ID {
		t.Fatalf("FolderChain parent ids = [%d %d %d], want [-1 %d %d]", chain[0].ParentID, chain[1].ParentID, chain[2].ParentID, rootFolder.ID, camera.ID)
	}
	if chain[0].RelativePath != "" || chain[1].RelativePath != "Camera" || chain[2].RelativePath != "Camera/Day1" {
		t.Fatalf("FolderChain relative paths = [%q %q %q], want [\"\" Camera Camera/Day1]", chain[0].RelativePath, chain[1].RelativePath, chain[2].RelativePath)
	}
	if chain[2].Name != "Day1" {
		t.Fatalf("FolderChain leaf name = %q, want Day1", chain[2].Name)
	}

	rootChain, err := repository.FolderChain(ctx, library.ID, rootFolder.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rootChain) != 1 || rootChain[0].ID != rootFolder.ID {
		t.Fatalf("FolderChain for root = %+v, want just the root", rootChain)
	}

	other := domain.Library{ID: domain.InvalidID, Name: "Other", Roots: []domain.LibraryRoot{{ID: domain.InvalidID, Path: filepath.Join(t.TempDir(), "other")}}}
	other, err = repository.CreateLibrary(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.FolderChain(ctx, other.ID, day1.ID); err != store.ErrNotFound {
		t.Fatalf("FolderChain across libraries err = %v, want ErrNotFound", err)
	}
	if _, err := repository.FolderChain(ctx, library.ID, 999999); err != store.ErrNotFound {
		t.Fatalf("FolderChain unknown folder err = %v, want ErrNotFound", err)
	}
}

func TestSQLiteCreateLibraryRejectsNestedRoots(t *testing.T) {
	repository, _ := openSQLite(t)
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "photos")
	nested := domain.Library{ID: domain.InvalidID, Name: "Photos", Roots: []domain.LibraryRoot{
		{ID: domain.InvalidID, Path: root},
		{ID: domain.InvalidID, Path: filepath.Join(root, "trips")},
	}}
	if _, err := repository.CreateLibrary(ctx, nested); !errors.Is(err, store.ErrNestedRoot) {
		t.Fatalf("CreateLibrary nested roots err = %v, want ErrNestedRoot", err)
	}

	library := domain.Library{ID: domain.InvalidID, Name: "Photos", Roots: []domain.LibraryRoot{{ID: domain.InvalidID, Path: root}}}
	library, err := repository.CreateLibrary(ctx, library)
	if err != nil {
		t.Fatal(err)
	}
	library.Roots = append(library.Roots, domain.LibraryRoot{ID: domain.InvalidID, Path: filepath.Join(root, "trips")})
	if err := repository.UpdateLibrary(ctx, library); !errors.Is(err, store.ErrNestedRoot) {
		t.Fatalf("UpdateLibrary nested roots err = %v, want ErrNestedRoot", err)
	}
	loaded, err := repository.Library(ctx, library.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Roots) != 1 || loaded.Roots[0].Path != root {
		t.Fatalf("Library after rejected update = %#v, want single root %q", loaded.Roots, root)
	}
}

func TestSQLiteFolderChainNestedAcrossLibraries(t *testing.T) {
	repository, _ := openSQLite(t)
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "photos")
	wide := domain.Library{ID: domain.InvalidID, Name: "Wide", Roots: []domain.LibraryRoot{{ID: domain.InvalidID, Path: root}}}
	wide, err := repository.CreateLibrary(ctx, wide)
	if err != nil {
		t.Fatal(err)
	}
	narrow := domain.Library{ID: domain.InvalidID, Name: "Narrow", Roots: []domain.LibraryRoot{{ID: domain.InvalidID, Path: filepath.Join(root, "trips")}}}
	narrow, err = repository.CreateLibrary(ctx, narrow)
	if err != nil {
		t.Fatal(err)
	}
	trips, err := repository.UpsertFolder(ctx, domain.MediaFolder{
		ID: domain.InvalidID, ParentID: wide.Roots[0].ID, Path: filepath.Join(root, "trips"), RelativePath: "trips",
	})
	if err != nil {
		t.Fatal(err)
	}
	day1, err := repository.UpsertFolder(ctx, domain.MediaFolder{
		ID: domain.InvalidID, ParentID: trips.ID, Path: filepath.Join(root, "trips", "2024"), RelativePath: "trips/2024",
	})
	if err != nil {
		t.Fatal(err)
	}

	wideChain, err := repository.FolderChain(ctx, wide.ID, day1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(wideChain) != 3 || wideChain[0].ID != wide.Roots[0].ID || wideChain[1].ID != trips.ID || wideChain[2].ID != day1.ID {
		t.Fatalf("wide chain = %#v, want [%d trips %d]", wideChain, wide.Roots[0].ID, day1.ID)
	}
	if wideChain[1].RelativePath != "trips" || wideChain[2].RelativePath != "trips/2024" {
		t.Fatalf("wide chain relative paths = [%q %q %q]", wideChain[0].RelativePath, wideChain[1].RelativePath, wideChain[2].RelativePath)
	}

	// Cross-library nesting is not rejected at create time: the inner library's
	// root is nested beneath the outer one, and the chain is cut at the nearest
	// ancestor that is a root of the requested library.
	narrowChain, err := repository.FolderChain(ctx, narrow.ID, day1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(narrowChain) != 2 || narrowChain[0].ID != trips.ID || narrowChain[1].ID != day1.ID {
		t.Fatalf("narrow chain = %#v, want [trips %d]", narrowChain, day1.ID)
	}
	if narrowChain[0].RelativePath != "" || narrowChain[1].RelativePath != "2024" {
		t.Fatalf("narrow chain relative paths = [%q %q], want [\"\" 2024]", narrowChain[0].RelativePath, narrowChain[1].RelativePath)
	}
	if narrowRootChain, err := repository.FolderChain(ctx, narrow.ID, trips.ID); err != nil || len(narrowRootChain) != 1 || narrowRootChain[0].ID != trips.ID {
		t.Fatalf("narrow root chain = %#v err=%v, want just trips", narrowRootChain, err)
	}
	if _, err := repository.FolderChain(ctx, narrow.ID, wide.Roots[0].ID); err != store.ErrNotFound {
		t.Fatalf("narrow foreign folder err = %v, want ErrNotFound", err)
	}
}

func TestSQLiteThumbnailUpsertAndRead(t *testing.T) {
	repository, _ := openSQLite(t)
	root := filepath.Join(t.TempDir(), "photos")
	library := domain.Library{ID: domain.InvalidID, Name: "Photos", Roots: []domain.LibraryRoot{{ID: domain.InvalidID, Path: root}}}
	var err error
	library, err = repository.CreateLibrary(context.Background(), library)
	if err != nil {
		t.Fatal(err)
	}
	media, err := repository.UpsertMedia(context.Background(), domain.Media{ID: domain.InvalidID, FolderID: library.Roots[0].ID, Path: filepath.Join(root, "clip.mp4"), Name: "clip.mp4", Kind: domain.KindVideo, MIMEType: "video/mp4"})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.UpsertThumbnail(context.Background(), domain.Thumbnail{MediaID: media.ID, Index: 3, MIMEType: "image/jpeg"}); err != nil {
		t.Fatal(err)
	}
	thumbnail, err := repository.Thumbnail(context.Background(), media.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if thumbnail.MediaID != media.ID || thumbnail.Index != 3 || thumbnail.MIMEType != "image/jpeg" {
		t.Fatalf("unexpected thumbnail: %#v", thumbnail)
	}
	if err := repository.UpsertThumbnail(context.Background(), domain.Thumbnail{MediaID: media.ID, Index: 3, MIMEType: "image/png"}); err != nil {
		t.Fatal(err)
	}
	thumbnail, err = repository.Thumbnail(context.Background(), media.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if thumbnail.MIMEType != "image/png" {
		t.Fatalf("thumbnail mime not updated: %#v", thumbnail)
	}
}

func TestSQLiteAccessControlForRegularUser(t *testing.T) {
	repository, _ := openSQLite(t)
	root := filepath.Join(t.TempDir(), "photos")
	library := domain.Library{ID: domain.InvalidID, Name: "Photos", Roots: []domain.LibraryRoot{{ID: domain.InvalidID, Path: root}}}
	var err error
	library, err = repository.CreateLibrary(context.Background(), library)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ImportSnapshot(context.Background(), domain.ImportSnapshot{Users: []domain.User{{
		ID: 1, Login: "regular", Role: domain.RoleRegular,
	}}}); err != nil {
		t.Fatal(err)
	}
	media, err := repository.UpsertMedia(context.Background(), domain.Media{ID: domain.InvalidID, FolderID: library.Roots[0].ID, Path: filepath.Join(root, "one.jpg"), Name: "one.jpg", Kind: domain.KindImage, MIMEType: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := repository.CanRead(context.Background(), 1, library.ID, false); err != nil || ok {
		t.Fatalf("read should be denied before access: ok=%v err=%v", ok, err)
	}
	if err := repository.SetAccess(context.Background(), library.ID, 1, true); err != nil {
		t.Fatal(err)
	}
	if ok, err := repository.CanRead(context.Background(), 1, library.ID, false); err != nil || !ok {
		t.Fatalf("read should be allowed after access: ok=%v err=%v", ok, err)
	}
	if ok, err := repository.CanReadMedia(context.Background(), 1, media.ID, false); err != nil || !ok {
		t.Fatalf("media read should be allowed after access: ok=%v err=%v", ok, err)
	}
}

func TestSQLitePruneFolderKeepsRoot(t *testing.T) {
	repository, _ := openSQLite(t)
	root := filepath.Join(t.TempDir(), "photos")
	library := domain.Library{ID: domain.InvalidID, Name: "Photos", Roots: []domain.LibraryRoot{{ID: domain.InvalidID, Path: root}}}
	var err error
	library, err = repository.CreateLibrary(context.Background(), library)
	if err != nil {
		t.Fatal(err)
	}
	folder, err := repository.UpsertFolder(context.Background(), domain.MediaFolder{ID: domain.InvalidID, ParentID: library.Roots[0].ID, Path: filepath.Join(root, "Camera"), RelativePath: "Camera"})
	if err != nil {
		t.Fatal(err)
	}
	media, err := repository.UpsertMedia(context.Background(), domain.Media{ID: domain.InvalidID, FolderID: folder.ID, Path: filepath.Join(root, "Camera", "one.jpg"), Name: "one.jpg", Kind: domain.KindImage, MIMEType: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	kept, err := repository.UpsertMedia(context.Background(), domain.Media{ID: domain.InvalidID, FolderID: folder.ID, Path: filepath.Join(root, "Camera", "keep.jpg"), Name: "keep.jpg", Kind: domain.KindImage, MIMEType: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.PruneFolder(context.Background(), library.Roots[0].ID,
		map[int]bool{}, map[int]bool{kept.ID: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Media(context.Background(), media.ID); err == nil {
		t.Fatal("unkept media should be pruned")
	}
	if _, err := repository.Media(context.Background(), kept.ID); err != nil {
		t.Fatalf("kept media should survive: %v", err)
	}
	if _, err := repository.Folder(context.Background(), library.Roots[0].ID); err != nil {
		t.Fatalf("root folder should survive prune: %v", err)
	}
}

func TestSQLiteScheduledTasksLifecycle(t *testing.T) {
	repository, _ := openSQLite(t)
	ctx := context.Background()
	library, err := repository.CreateLibrary(ctx, domain.Library{Name: "Family"})
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Hour)
	future := time.Now().UTC().Add(24 * time.Hour)
	created, err := repository.CreateScheduledTask(ctx, domain.ScheduledTask{
		Name: "Nightly scan", TaskType: "scan", LibraryID: library.ID,
		Cron: "0 3 * * *", Enabled: true, NextRunAt: past,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == 0 {
		t.Fatal("created task should have an id")
	}
	stored, err := repository.ScheduledTask(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Name != "Nightly scan" || stored.TaskType != "scan" || !stored.Enabled {
		t.Fatalf("unexpected stored task: %#v", stored)
	}
	due, err := repository.DueScheduledTasks(ctx, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].ID != created.ID {
		t.Fatalf("expected one due task: %#v", due)
	}
	if err := repository.MarkScheduledTaskRun(ctx, created.ID, past, future); err != nil {
		t.Fatal(err)
	}
	stored, _ = repository.ScheduledTask(ctx, created.ID)
	if stored.LastRunAt == nil || !stored.LastRunAt.UTC().Equal(past.UTC()) {
		t.Fatalf("last run not recorded: %#v", stored.LastRunAt)
	}
	if !stored.NextRunAt.UTC().Equal(future.UTC()) {
		t.Fatalf("next run not updated: %v", stored.NextRunAt)
	}
	if err := repository.DisableScheduledTask(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	due, err = repository.DueScheduledTasks(ctx, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("disabled task should not be due: %#v", due)
	}
	if err := repository.DeleteScheduledTask(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ScheduledTask(ctx, created.ID); err != store.ErrNotFound {
		t.Fatalf("deleted task should be not found: %v", err)
	}
}

func TestSQLiteScheduledTasksRemovedWithLibrary(t *testing.T) {
	repository, _ := openSQLite(t)
	ctx := context.Background()
	library, err := repository.CreateLibrary(ctx, domain.Library{Name: "Family"})
	if err != nil {
		t.Fatal(err)
	}
	libraryTask, err := repository.CreateScheduledTask(ctx, domain.ScheduledTask{
		Name: "Scan", TaskType: "scan", LibraryID: library.ID, Cron: "0 3 * * *",
		Enabled: true, NextRunAt: time.Now().UTC().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateScheduledTask(ctx, domain.ScheduledTask{
		Name: "Vacuum", TaskType: "vacuum", LibraryID: 0, Cron: "0 4 * * *",
		Enabled: true, NextRunAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := repository.DeleteScheduledTasksForLibrary(ctx, library.ID); err != nil {
		t.Fatal(err)
	}
	tasks, err := repository.ScheduledTasks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].ID == libraryTask.ID {
		t.Fatalf("library tasks should be removed, remaining: %#v", tasks)
	}
}

func TestSQLiteVacuumJobCategoryPersists(t *testing.T) {
	repository, _ := openSQLite(t)
	job := domain.BackgroundJob{
		ID: "vacuum-1", Category: "vacuum", Type: "vacuum", LibraryName: "Database maintenance",
		Status: "done", StartedAt: time.Now().UTC(),
	}
	if err := repository.SaveJob(context.Background(), job); err != nil {
		t.Fatalf("vacuum job category must be allowed by the schema: %v", err)
	}
	jobs, err := repository.Jobs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Category != "vacuum" {
		t.Fatalf("vacuum job not persisted: %#v", jobs)
	}
}

func TestSQLiteTHMExtensionMapsToImageJPEG(t *testing.T) {
	repository, _ := openSQLite(t)
	mimeType, err := repository.MIMETypeForExtension(context.Background(), ".thm")
	if err != nil {
		t.Fatal(err)
	}
	if mimeType != "image/jpeg" {
		t.Fatalf("THM mime = %q, want image/jpeg", mimeType)
	}
}

// TestSQLiteSeparateHandlesOnSameFile verifies the dual-connection layout the
// server uses: one interactive handle and one dedicated job handle opened on
// the same file can operate concurrently, and a write committed by one handle
// is immediately visible from the other. This guards against regressions that
// would let a background job wedge the single connection a UI is reading.
func TestSQLiteSeparateHandlesOnSameFile(t *testing.T) {
	dbFile := filepath.Join(t.TempDir(), "media-library.db")
	repository, err := store.NewSQLite(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	jobStore, err := store.NewSQLite(dbFile)
	if err != nil {
		t.Fatal(err)
	}
	defer jobStore.Close()

	ctx := context.Background()
	if _, err := repository.CreateInitialAdmin(ctx, domain.User{
		ID: domain.InvalidID, Login: "owner",
	}, "a-secure-password"); err != nil {
		t.Fatal(err)
	}

	// Writes through the job handle are visible to the interactive handle...
	if err := jobStore.SaveUserSettings(ctx, 1, domain.UserSettings{Theme: "dark", StreamChunkSize: 40}); err != nil {
		t.Fatal(err)
	}
	readBack, err := repository.UserSettings(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if readBack.StreamChunkSize != 40 {
		t.Fatalf("interactive handle did not see job-handle write: %+v", readBack)
	}

	// ...and both handles stay usable under concurrent reads/writes.
	done := make(chan error, 2)
	go func() {
		var err error
		for i := 0; i < 20 && err == nil; i++ {
			_, err = jobStore.ServerSettings(ctx)
		}
		done <- err
	}()
	go func() {
		var err error
		for i := 0; i < 20 && err == nil; i++ {
			if _, err = repository.ServerSettings(ctx); err == nil {
				_, err = repository.Authenticate(ctx, "owner", "a-secure-password")
			}
		}
		done <- err
	}()
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatalf("concurrent handle access failed: %v", err)
		}
	}
}

func TestSQLiteFolderScopedMapEnrichesTrajectoryFromOwnFolder(t *testing.T) {
	repository, _ := openSQLite(t)
	ctx := context.Background()
	root := t.TempDir()
	library, err := repository.CreateLibrary(ctx, domain.Library{ID: domain.InvalidID, Name: "Trails", Roots: []domain.LibraryRoot{{ID: domain.InvalidID, Path: root}}})
	if err != nil {
		t.Fatal(err)
	}
	sub, err := repository.UpsertFolder(ctx, domain.MediaFolder{ID: domain.InvalidID, ParentID: library.Roots[0].ID, Path: filepath.Join(root, "day1"), RelativePath: "day1"})
	if err != nil {
		t.Fatal(err)
	}
	media, err := repository.UpsertMedia(ctx, domain.Media{ID: domain.InvalidID, FolderID: sub.ID, Path: filepath.Join(root, "day1", "a.jpg"), Name: "a.jpg", Kind: domain.KindImage, MIMEType: "image/jpeg", Size: 10, Metadata: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	gps := "50.45,30.52"
	if _, err := repository.UpdateMediaDetails(ctx, media.ID, domain.MediaDetailsPatch{GPS: &gps}); err != nil {
		t.Fatal(err)
	}
	// The media card attaches trajectory markers to the media's own folder; the
	// map view may be scoped to an ancestor folder that must still surface them.
	if err := repository.SetTrajectoryStart(ctx, sub.ID, media.ID, true); err != nil {
		t.Fatal(err)
	}
	name := "Evening loop"
	if err := repository.SetTrajectoryName(ctx, sub.ID, media.ID, name); err != nil {
		t.Fatal(err)
	}

	items, err := repository.GeotaggedMedia(ctx, 1, true, library.ID, library.Roots[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != media.ID {
		t.Fatalf("expected one map item, got %#v", items)
	}
	if !items[0].TrajectoryStart || items[0].TrajectoryName != name {
		t.Fatalf("expected start flag + name on folder map, got start=%v name=%q", items[0].TrajectoryStart, items[0].TrajectoryName)
	}
	area, err := repository.MediaInArea(ctx, 1, true, library.ID, library.Roots[0].ID, domain.Bounds{North: 51, South: 50, East: 31, West: 30})
	if err != nil {
		t.Fatal(err)
	}
	if len(area) != 1 || !area[0].TrajectoryStart || area[0].TrajectoryName != name {
		t.Fatalf("expected area item with start flag, got %#v", area)
	}
}
