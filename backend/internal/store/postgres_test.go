package store_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"media-library/backend/internal/domain"
	"media-library/backend/internal/store"
)

func postgresDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN not set; skipping postgres store tests")
	}
	return dsn
}

func resetPostgresSchema(t *testing.T, dsn string) {
	t.Helper()
	raw, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatal(err)
	}
}

// openPostgres opens a store against POSTGRES_TEST_DSN, resetting the schema
// first when reset is true.
func openPostgres(t *testing.T, reset bool) *store.Postgres {
	t.Helper()
	dsn := postgresDSN(t)
	if reset {
		resetPostgresSchema(t, dsn)
	}
	repository, err := store.NewPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repository.Close() })
	return repository
}

func TestPostgresInitialAdminAndRestart(t *testing.T) {
	repository := openPostgres(t, true)
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

	restarted := openPostgres(t, false)
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

func TestPostgresLibraryStatePersistsInternalPathsAcrossRestart(t *testing.T) {
	repository := openPostgres(t, true)
	mediaRoot := t.TempDir()
	root := filepath.Join(mediaRoot, "photos")
	folderPath := filepath.Join(root, "Camera")
	mediaPath := filepath.Join(folderPath, "one.jpg")
	library := domain.Library{ID: domain.InvalidID, Name: "Photos", Roots: []domain.LibraryRoot{{ID: domain.InvalidID, Path: root}}}
	var err error
	library, err = repository.CreateLibrary(context.Background(), library)
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
	restarted := openPostgres(t, false)
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
	if loadedMedia.RelativePath != "Camera/one.jpg" {
		t.Fatalf("relative path should be computed on the fly: %q", loadedMedia.RelativePath)
	}
}

func TestPostgresFolderChain(t *testing.T) {
	repository := openPostgres(t, true)
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
	if chain[0].RelativePath != "" || chain[1].RelativePath != "Camera" || chain[2].RelativePath != "Camera/Day1" {
		t.Fatalf("FolderChain relative paths = [%q %q %q], want [\"\" Camera Camera/Day1]", chain[0].RelativePath, chain[1].RelativePath, chain[2].RelativePath)
	}
	if _, err := repository.FolderChain(ctx, library.ID, 999999); err != store.ErrNotFound {
		t.Fatalf("FolderChain unknown folder err = %v, want ErrNotFound", err)
	}
}

func TestPostgresCreateLibraryRejectsNestedRoots(t *testing.T) {
	repository := openPostgres(t, true)
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

func TestPostgresFolderChainNestedAcrossLibraries(t *testing.T) {
	repository := openPostgres(t, true)
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

	// Cross-library nesting is not rejected at create time, but a folder is
	// reachable only through the library whose root is the tree top, so the
	// inner library finds nothing under it.
	if _, err := repository.FolderChain(ctx, narrow.ID, day1.ID); err != store.ErrNotFound {
		t.Fatalf("narrow chain err = %v, want ErrNotFound", err)
	}
	if _, err := repository.FolderChain(ctx, narrow.ID, trips.ID); err != store.ErrNotFound {
		t.Fatalf("narrow root chain err = %v, want ErrNotFound", err)
	}
}

func TestPostgresMediaForLibraryComputesRelativePaths(t *testing.T) {
	repository := openPostgres(t, true)
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
	list, err := repository.MediaForLibrary(context.Background(), 0, library.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].RelativePath != "Camera/one.jpg" {
		t.Fatalf("media for library relative paths: %#v", list)
	}
}

func TestPostgresImportedEmbySHA1PasswordAuthenticatesAndUpgradesToBcrypt(t *testing.T) {
	repository := openPostgres(t, true)
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
	restarted := openPostgres(t, false)
	if _, err := restarted.Authenticate(context.Background(), "ice", "password"); err != nil {
		t.Fatalf("upgraded bcrypt login failed after restart: %v", err)
	}
}

func TestPostgresCreateLibraryRegistersRootFolderFirst(t *testing.T) {
	repository := openPostgres(t, true)
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

func TestPostgresLibraryNamesAreUniqueCaseInsensitive(t *testing.T) {
	repository := openPostgres(t, true)
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

func TestPostgresRegularLibraryListingDoesNotEraseStoredRootPaths(t *testing.T) {
	repository := openPostgres(t, true)
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

func TestPostgresLibraryStats(t *testing.T) {
	repository := openPostgres(t, true)
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
	stats, err := repository.LibraryStats(context.Background(), library.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Folders != 1 || stats.Files != 2 || stats.Images != 1 || stats.Videos != 1 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
}

func TestPostgresDeleteLibraryRemovesOnlyUnsharedMedia(t *testing.T) {
	repository := openPostgres(t, true)
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
	var sharedRootID, privateRootID = domain.InvalidID, domain.InvalidID
	for _, root := range first.Roots {
		if root.Path == shared {
			sharedRootID = root.ID
		}
		if root.Path == private {
			privateRootID = root.ID
		}
	}
	if sharedRootID == domain.InvalidID || privateRootID == domain.InvalidID {
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

func TestPostgresFavoriteViewsAndRelativePaths(t *testing.T) {
	repository := openPostgres(t, true)
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
	if _, err := repository.CreateFavoriteView(context.Background(), 1, "My Favorites"); err == nil {
		t.Fatal("duplicate favorite view name should be rejected by UNIQUE(user_id, name)")
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
	if len(list) != 1 || list[0].ID != media.ID || list[0].RelativePath != "one.jpg" {
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

func TestPostgresMediaBatchAndFoldersByIDs(t *testing.T) {
	repository := openPostgres(t, true)
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

func TestPostgresThumbnailUpsertAndRead(t *testing.T) {
	repository := openPostgres(t, true)
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

func TestPostgresAccessControlForRegularUser(t *testing.T) {
	repository := openPostgres(t, true)
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

func TestPostgresPruneFolderKeepsRoot(t *testing.T) {
	repository := openPostgres(t, true)
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
