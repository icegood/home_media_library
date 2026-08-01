package store_test

import (
	"context"
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
	settings.TranscodeCodec = "vp9"
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
	if err != nil || settings.TranscodeCodec != "vp9" {
		t.Fatalf("persisted codec = %q, err=%v", settings.TranscodeCodec, err)
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

func TestSQLiteLibraryListingIncludesStatistics(t *testing.T) {
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
	libraries, err := repository.LibrariesForUser(context.Background(), 0, true)
	if err != nil {
		t.Fatal(err)
	}
	stats := libraries[0].Stats
	if stats.Folders != 1 || stats.Files != 2 || stats.Images != 1 || stats.Videos != 1 {
		t.Fatalf("unexpected stats: %#v", stats)
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
