package api_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"media-library/backend/internal/api"
	"media-library/backend/internal/domain"
	"media-library/backend/internal/scanner"
	"media-library/backend/internal/store"
)

const secret = "test-secret-with-at-least-thirty-two-characters"

type fixture struct {
	handler      http.Handler
	store        store.Store
	mediaRoot    string
	thumbnailDir string
	libraryID    int
	folderID     int
	photoID      int
	aliceID      int
}

func openSQLite(t *testing.T) *store.SQLite {
	t.Helper()
	repository, err := store.NewSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repository.Close() })
	return repository
}

func seededPassword(t *testing.T) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	return string(hash)
}

func setup(t *testing.T) fixture {
	t.Helper()
	repository := openSQLite(t)
	if _, err := repository.CreateInitialAdmin(context.Background(), domain.User{ID: domain.InvalidID, Login: "admin"}, "password"); err != nil {
		t.Fatal(err)
	}
	imported, err := repository.ImportSnapshot(context.Background(), domain.ImportSnapshot{Users: []domain.User{
		{ID: 2, Login: "alice", Role: domain.RoleRegular, PasswordHash: seededPassword(t)},
		{ID: 3, Login: "bob", Role: domain.RoleRegular, PasswordHash: seededPassword(t)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	aliceID := imported.Users[0].User.ID
	mediaRoot := t.TempDir()
	_ = os.MkdirAll(filepath.Join(mediaRoot, "new"), 0o755)
	_ = os.MkdirAll(filepath.Join(mediaRoot, "archive"), 0o755)
	familyPath := filepath.Join(mediaRoot, "family")
	_ = os.MkdirAll(filepath.Join(familyPath, "2025"), 0o755)
	library, _ := repository.CreateLibrary(context.Background(), domain.Library{ID: domain.InvalidID, Name: "Family",
		Roots: []domain.LibraryRoot{{ID: domain.InvalidID, Path: familyPath}}})
	_ = repository.SetAccess(context.Background(), library.ID, aliceID, true)
	folder, _ := repository.UpsertFolder(context.Background(), domain.MediaFolder{ID: domain.InvalidID, ParentID: library.Roots[0].ID, Path: filepath.Join(familyPath, "2025"), RelativePath: "2025"})
	_ = os.WriteFile(filepath.Join(familyPath, "2025", "trip.jpg"), []byte("image"), 0o644)
	photo, _ := repository.UpsertMedia(context.Background(), domain.Media{ID: domain.InvalidID, FolderID: folder.ID, Path: filepath.Join(familyPath, "2025", "trip.jpg"), RelativePath: "2025/trip.jpg", Name: "trip.jpg", Kind: domain.KindImage, MIMEType: "image/jpeg", TakenAt: "2025-05-06T07:08:09Z"})
	thumbnailDir := t.TempDir()
	return fixture{handler: (&api.API{
		Store: repository, Scanner: scanner.Scanner{Store: repository},
		JWTSecret: []byte(secret), ThumbnailDir: thumbnailDir,
	}).Handler(), store: repository, mediaRoot: mediaRoot, thumbnailDir: thumbnailDir, libraryID: library.ID, folderID: folder.ID, photoID: photo.ID, aliceID: aliceID}
}

func login(t *testing.T, h http.Handler, login string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"login": login, "password": "password"})
	response := httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body)))
	if response.Code != http.StatusOK {
		t.Fatalf("login status = %d: %s", response.Code, response.Body)
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == "media_session" {
			return cookie.Value
		}
	}
	t.Fatalf("login did not set media_session cookie: %#v", response.Result().Cookies())
	return ""
}

func request(h http.Handler, method, path, session string, body []byte) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if session != "" {
		req.AddCookie(&http.Cookie{Name: "media_session", Value: session})
	}
	h.ServeHTTP(response, req)
	return response
}

func TestLibraryAccessIsPerUser(t *testing.T) {
	f := setup(t)
	alice := login(t, f.handler, "alice")
	bob := login(t, f.handler, "bob")
	if got := request(f.handler, http.MethodGet, fmt.Sprintf("/api/v1/libraries/%d/entries", f.libraryID), alice, nil).Code; got != http.StatusOK {
		t.Fatalf("alice status = %d", got)
	}
	if got := request(f.handler, http.MethodGet, fmt.Sprintf("/api/v1/libraries/%d/entries", f.libraryID), bob, nil).Code; got != http.StatusForbidden {
		t.Fatalf("bob status = %d", got)
	}
}

func TestDeleteLibraryRemovesOrphanThumbnailFiles(t *testing.T) {
	f := setup(t)
	admin := login(t, f.handler, "admin")
	thumbDir := filepath.Join(f.thumbnailDir, "media", "0")
	if err := os.MkdirAll(thumbDir, 0o770); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(thumbDir, fmt.Sprintf("%d_0.jpg", f.photoID)), []byte("thumb"), 0o660); err != nil {
		t.Fatal(err)
	}
	if err := f.store.UpsertThumbnail(context.Background(), domain.Thumbnail{MediaID: f.photoID, Index: 0, MIMEType: "image/jpeg"}); err != nil {
		t.Fatal(err)
	}
	response := request(f.handler, http.MethodDelete, fmt.Sprintf("/api/v1/admin/libraries/%d", f.libraryID), admin, nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d: %s", response.Code, response.Body)
	}
	if _, err := os.Stat(thumbDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("thumbnail dir should be removed, stat err=%v", err)
	}
}

func TestScheduledTasksLifecycle(t *testing.T) {
	f := setup(t)
	admin := login(t, f.handler, "admin")

	createBody := fmt.Sprintf(`{"name":"Nightly scan","taskType":"scan","libraryId":%d,"cron":"0 3 * * *","enabled":true}`, f.libraryID)
	create := request(f.handler, http.MethodPost, "/api/v1/admin/scheduled-tasks", admin, []byte(createBody))
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d: %s", create.Code, create.Body)
	}
	var task domain.ScheduledTask
	if err := json.Unmarshal(create.Body.Bytes(), &task); err != nil {
		t.Fatal(err)
	}
	if task.ID == 0 || task.NextRunAt.IsZero() {
		t.Fatalf("created task missing id or next run: %#v", task)
	}

	list := request(f.handler, http.MethodGet, "/api/v1/admin/scheduled-tasks", admin, nil)
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", list.Code, list.Body)
	}

	lastRun := time.Now().UTC().Add(-time.Hour)
	if err := f.store.MarkScheduledTaskRun(context.Background(), task.ID, lastRun, task.NextRunAt); err != nil {
		t.Fatal(err)
	}
	updateBody := fmt.Sprintf(`{"name":"Nightly scan","taskType":"scan","libraryId":%d,"cron":"0 4 * * *","enabled":true}`, f.libraryID)
	update := request(f.handler, http.MethodPut, fmt.Sprintf("/api/v1/admin/scheduled-tasks/%d", task.ID), admin, []byte(updateBody))
	if update.Code != http.StatusOK {
		t.Fatalf("update status = %d: %s", update.Code, update.Body)
	}
	var updated domain.ScheduledTask
	if err := json.Unmarshal(update.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Cron != "0 4 * * *" {
		t.Fatalf("cron not updated: %#v", updated.Cron)
	}
	if updated.LastRunAt == nil || !updated.LastRunAt.UTC().Equal(lastRun.UTC()) {
		t.Fatalf("last run was not preserved on edit: %#v", updated.LastRunAt)
	}

	if bad := request(f.handler, http.MethodPost, "/api/v1/admin/scheduled-tasks", admin, []byte(`{"name":"bad","taskType":"vacuum","cron":"not a cron"}`)); bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid cron status = %d: %s", bad.Code, bad.Body)
	}
	if noLib := request(f.handler, http.MethodPost, "/api/v1/admin/scheduled-tasks", admin, []byte(`{"name":"n","taskType":"scan","cron":"0 3 * * *"}`)); noLib.Code != http.StatusBadRequest {
		t.Fatalf("scan without library status = %d: %s", noLib.Code, noLib.Body)
	}
	if del := request(f.handler, http.MethodDelete, fmt.Sprintf("/api/v1/admin/scheduled-tasks/%d", task.ID), admin, nil); del.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d: %s", del.Code, del.Body)
	}
}

func TestFavoritesArePerUserAndRespectReadAccess(t *testing.T) {
	f := setup(t)
	alice := login(t, f.handler, "alice")
	bob := login(t, f.handler, "bob")
	create := request(f.handler, http.MethodPost, "/api/v1/favorite-views", alice, []byte(`{"name":"Best"}`))
	if create.Code != http.StatusCreated {
		t.Fatalf("create favorite view status = %d: %s", create.Code, create.Body)
	}
	var view domain.FavoriteView
	if err := json.NewDecoder(create.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if response := request(f.handler, http.MethodPut, fmt.Sprintf("/api/v1/favorite-views/%d/media/%d", view.ID, f.photoID), alice, nil); response.Code != http.StatusOK {
		t.Fatalf("alice favorite status = %d: %s", response.Code, response.Body)
	}
	if response := request(f.handler, http.MethodPut, fmt.Sprintf("/api/v1/favorite-views/%d/media/%d", view.ID, f.photoID), bob, nil); response.Code != http.StatusForbidden {
		t.Fatalf("bob favorite status = %d: %s", response.Code, response.Body)
	}
	response := request(f.handler, http.MethodGet, fmt.Sprintf("/api/v1/favorite-views/%d/media", view.ID), alice, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("favorites status = %d: %s", response.Code, response.Body)
	}
	if !bytes.Contains(response.Body.Bytes(), []byte(fmt.Sprintf(`"id":%d`, f.photoID))) || !bytes.Contains(response.Body.Bytes(), []byte(`"favorite":true`)) {
		t.Fatalf("favorite media missing from response: %s", response.Body)
	}
	response = request(f.handler, http.MethodGet, fmt.Sprintf("/api/v1/favorite-views/%d/media", view.ID), bob, nil)
	if response.Code != http.StatusNotFound || bytes.Contains(response.Body.Bytes(), []byte(fmt.Sprintf(`"id":%d`, f.photoID))) {
		t.Fatalf("bob favorites should not include photo: status=%d body=%s", response.Code, response.Body)
	}
}

func TestOnlyAdminCanManageLibraries(t *testing.T) {
	f := setup(t)
	alice := login(t, f.handler, "alice")
	admin := login(t, f.handler, "admin")
	payload := []byte(fmt.Sprintf(`{"name":"New","roots":[{"name":"new","path":%q},{"name":"archive","path":%q}]}`,
		filepath.Join(f.mediaRoot, "new"), filepath.Join(f.mediaRoot, "archive")))
	if got := request(f.handler, http.MethodPost, "/api/v1/admin/libraries", alice, payload).Code; got != http.StatusForbidden {
		t.Fatalf("regular user status = %d", got)
	}
	if got := request(f.handler, http.MethodPost, "/api/v1/admin/libraries", admin, payload).Code; got != http.StatusCreated {
		t.Fatalf("admin status = %d", got)
	}
	if got := request(f.handler, http.MethodDelete, fmt.Sprintf("/api/v1/admin/libraries/%d", f.libraryID), alice, nil).Code; got != http.StatusForbidden {
		t.Fatalf("regular delete status = %d", got)
	}
	if got := request(f.handler, http.MethodDelete, fmt.Sprintf("/api/v1/admin/libraries/%d", f.libraryID), admin, nil).Code; got != http.StatusNoContent {
		t.Fatalf("admin delete status = %d", got)
	}
}

func TestAdminCanBrowseFilesystem(t *testing.T) {
	f := setup(t)
	admin := login(t, f.handler, "admin")
	response := request(f.handler, http.MethodGet, "/api/v1/admin/filesystem?path="+url.QueryEscape(f.mediaRoot), admin, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("browse status = %d: %s", response.Code, response.Body)
	}
	if !bytes.Contains(response.Body.Bytes(), []byte(`"name":"archive"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"name":"new"`)) {
		t.Fatalf("expected media directories in response: %s", response.Body)
	}
	root := request(f.handler, http.MethodGet, "/api/v1/admin/filesystem?path=/", admin, nil)
	if root.Code != http.StatusOK {
		t.Fatalf("browse root status = %d: %s", root.Code, root.Body)
	}
}

func TestVisibleUserCanEditLocation(t *testing.T) {
	f := setup(t)
	alice := login(t, f.handler, "alice")
	response := request(f.handler, http.MethodPatch, fmt.Sprintf("/api/v1/media/%d/gps", f.photoID), alice, []byte(`{"gps":"50.45, 30.52"}`))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body)
	}
	item, _ := f.store.Media(context.Background(), f.photoID)
	if item.GPS != "50.45,30.52" {
		t.Fatalf("GPS not persisted canonically: %#v", item)
	}
}

func TestMapShowsOnlyReadableGeotaggedMediaWithLibraryContext(t *testing.T) {
	f := setup(t)
	item, _ := f.store.Media(context.Background(), f.photoID)
	item.GPS = "50.45,30.52"
	if _, err := f.store.UpsertMedia(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	alice := login(t, f.handler, "alice")
	response := request(f.handler, http.MethodGet, "/api/v1/map", alice, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body)
	}
	var items []domain.MapMedia
	if err := json.Unmarshal(response.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != f.photoID || items[0].LibraryID != f.libraryID {
		t.Fatalf("unexpected map payload: %#v", items)
	}
	bob := login(t, f.handler, "bob")
	response = request(f.handler, http.MethodGet, "/api/v1/map", bob, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("bob status = %d: %s", response.Code, response.Body)
	}
	if err := json.Unmarshal(response.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("bob should not see unreadable media: %#v", items)
	}
}

func TestMapScopesToLibraryAndFolder(t *testing.T) {
	f := setup(t)
	photo, _ := f.store.Media(context.Background(), f.photoID)
	photo.GPS = "50.45,30.52"
	if _, err := f.store.UpsertMedia(context.Background(), photo); err != nil {
		t.Fatal(err)
	}
	library, err := f.store.Library(context.Background(), f.libraryID)
	if err != nil {
		t.Fatal(err)
	}
	secondFolder, err := f.store.UpsertFolder(context.Background(), domain.MediaFolder{ID: domain.InvalidID, ParentID: library.Roots[0].ID, Path: filepath.Join(f.mediaRoot, "family", "2026"), RelativePath: "2026"})
	if err != nil {
		t.Fatal(err)
	}
	secondPhoto, err := f.store.UpsertMedia(context.Background(), domain.Media{ID: domain.InvalidID, FolderID: secondFolder.ID, Path: filepath.Join(f.mediaRoot, "family", "2026", "trip2.jpg"), RelativePath: "2026/trip2.jpg", Name: "trip2.jpg", Kind: domain.KindImage, MIMEType: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	secondPhoto.GPS = "50.46,30.53"
	if _, err := f.store.UpsertMedia(context.Background(), secondPhoto); err != nil {
		t.Fatal(err)
	}
	alice := login(t, f.handler, "alice")
	var items []domain.MapMedia
	response := request(f.handler, http.MethodGet, fmt.Sprintf("/api/v1/map?library=%d", f.libraryID), alice, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("library status = %d: %s", response.Code, response.Body)
	}
	if err := json.Unmarshal(response.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("library-scoped map payload = %d items, want 2: %#v", len(items), items)
	}
	response = request(f.handler, http.MethodGet, fmt.Sprintf("/api/v1/map?library=%d&folder=%d", f.libraryID, f.folderID), alice, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("folder status = %d: %s", response.Code, response.Body)
	}
	if err := json.Unmarshal(response.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != f.photoID {
		t.Fatalf("folder-scoped map payload = %#v, want only %d", items, f.photoID)
	}
	if got := request(f.handler, http.MethodGet, fmt.Sprintf("/api/v1/map?folder=%d", f.folderID), alice, nil).Code; got != http.StatusBadRequest {
		t.Fatalf("folder without library status = %d, want 400", got)
	}
	if got := request(f.handler, http.MethodGet, fmt.Sprintf("/api/v1/map?library=%d&folder=999999", f.libraryID), alice, nil).Code; got != http.StatusNotFound {
		t.Fatalf("missing folder status = %d, want 404", got)
	}
	bob := login(t, f.handler, "bob")
	if got := request(f.handler, http.MethodGet, fmt.Sprintf("/api/v1/map?library=%d", f.libraryID), bob, nil).Code; got != http.StatusForbidden {
		t.Fatalf("bob library map status = %d, want 403", got)
	}
	if got := request(f.handler, http.MethodGet, fmt.Sprintf("/api/v1/map?library=%d&folder=%d", f.libraryID, f.folderID), bob, nil).Code; got != http.StatusForbidden {
		t.Fatalf("bob folder map status = %d, want 403", got)
	}
}

func TestMapBBoxFiltersGeotaggedMedia(t *testing.T) {
	f := setup(t)
	item, _ := f.store.Media(context.Background(), f.photoID)
	item.GPS = "50.45,30.52"
	if _, err := f.store.UpsertMedia(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	alice := login(t, f.handler, "alice")
	var items []domain.MapMedia
	response := request(f.handler, http.MethodGet, "/api/v1/map?bbox=30.0,50.0,31.0,51.0", alice, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body)
	}
	if err := json.Unmarshal(response.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != f.photoID {
		t.Fatalf("bbox payload = %#v, want only %d", items, f.photoID)
	}
	response = request(f.handler, http.MethodGet, "/api/v1/map?bbox=-50.0,-50.0,-40.0,-40.0", alice, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("empty bbox status = %d: %s", response.Code, response.Body)
	}
	if err := json.Unmarshal(response.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("empty bbox payload = %#v, want none", items)
	}
	for _, query := range []string{
		"/api/v1/map?bbox=30,50,31",
		"/api/v1/map?bbox=a,50,31,51",
		"/api/v1/map?bbox=31,50,30,51",
		"/api/v1/map?bbox=30,51,31,50",
		"/api/v1/map?bbox=30,50,200,51",
	} {
		if got := request(f.handler, http.MethodGet, query, alice, nil).Code; got != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want 400", query, got)
		}
	}
}

func TestLibraryStatsEndpoint(t *testing.T) {
	f := setup(t)
	alice := login(t, f.handler, "alice")
	bob := login(t, f.handler, "bob")
	var libraries []domain.Library
	response := request(f.handler, http.MethodGet, "/api/v1/libraries", alice, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body)
	}
	if err := json.Unmarshal(response.Body.Bytes(), &libraries); err != nil {
		t.Fatal(err)
	}
	if len(libraries) != 1 || libraries[0].Stats.Folders != 0 {
		t.Fatalf("default listing should omit statistics: %#v", libraries)
	}
	response = request(f.handler, http.MethodGet, fmt.Sprintf("/api/v1/libraries/%d/stats", f.libraryID), alice, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("alice stats status = %d: %s", response.Code, response.Body)
	}
	var stats domain.LibraryStats
	if err := json.Unmarshal(response.Body.Bytes(), &stats); err != nil {
		t.Fatal(err)
	}
	if stats.Folders != 1 || stats.Files != 1 || stats.Images != 1 || stats.Videos != 0 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
	if got := request(f.handler, http.MethodGet, fmt.Sprintf("/api/v1/libraries/%d/stats", f.libraryID), bob, nil).Code; got != http.StatusForbidden {
		t.Fatalf("bob stats status = %d, want 403", got)
	}
	if got := request(f.handler, http.MethodGet, "/api/v1/libraries/999999/stats", alice, nil).Code; got != http.StatusNotFound {
		t.Fatalf("missing library stats status = %d, want 404", got)
	}
}

func TestVisibleUserCanEditMediaDetails(t *testing.T) {
	f := setup(t)
	alice := login(t, f.handler, "alice")
	response := request(f.handler, http.MethodPatch, fmt.Sprintf("/api/v1/media/%d/details", f.photoID), alice, []byte(`{"name":"renamed.jpg","gps":"50.45, 30.52","takenAt":"2020-08-21T12:34"}`))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body)
	}
	item, _ := f.store.Media(context.Background(), f.photoID)
	if item.Name != "renamed.jpg" || item.GPS != "50.45,30.52" || item.TakenAt != "2020-08-21T12:34:00Z" {
		t.Fatalf("details not persisted: %#v", item)
	}
}

func TestVisibleUserCanReadLibraryTimelineMedia(t *testing.T) {
	f := setup(t)
	alice := login(t, f.handler, "alice")
	response := request(f.handler, http.MethodGet, fmt.Sprintf("/api/v1/libraries/%d/media", f.libraryID), alice, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body)
	}
	var items []domain.Media
	if err := json.Unmarshal(response.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != f.photoID || items[0].TakenAt != "2025-05-06T07:08:09Z" {
		t.Fatalf("unexpected timeline media: %#v", items)
	}
	bob := login(t, f.handler, "bob")
	forbidden := request(f.handler, http.MethodGet, fmt.Sprintf("/api/v1/libraries/%d/media", f.libraryID), bob, nil)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("forbidden status = %d: %s", forbidden.Code, forbidden.Body)
	}
}

func TestMediaContentServes(t *testing.T) {
	f := setup(t)
	alice := login(t, f.handler, "alice")
	response := request(f.handler, http.MethodGet, fmt.Sprintf("/api/v1/media/%d/content", f.photoID), alice, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("content status = %d: %s", response.Code, response.Body)
	}
}

func TestInvalidCoordinatesRejected(t *testing.T) {
	f := setup(t)
	alice := login(t, f.handler, "alice")
	response := request(f.handler, http.MethodPatch, fmt.Sprintf("/api/v1/media/%d/gps", f.photoID), alice, []byte(`{"gps":"91,30"}`))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestVideoThumbnailCountRespectsMaxAndMinimumInterval(t *testing.T) {
	f := setup(t)
	alice := login(t, f.handler, "alice")
	item, _ := f.store.Media(context.Background(), f.photoID)
	item.Name = "clip.mp4"
	item.Kind = domain.KindVideo
	item.MIMEType = "video/mp4"
	item.Metadata = map[string]any{"ffprobe": map[string]any{"format": map[string]any{"duration": "180"}}}
	if _, err := f.store.UpsertMedia(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	settings, _ := f.store.ServerSettings(context.Background())
	settings.VideoThumbnailFirstSeconds = 5
	settings.VideoThumbnailMaxCount = 10
	settings.VideoThumbnailMinIntervalSeconds = 120
	_ = f.store.SaveServerSettings(context.Background(), settings)
	response := request(f.handler, http.MethodGet, fmt.Sprintf("/api/v1/media/%d/thumbnails", f.photoID), alice, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body)
	}
	var thumbnails []struct {
		Index       int `json:"index"`
		TimeSeconds int `json:"timeSeconds"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &thumbnails); err != nil {
		t.Fatal(err)
	}
	if len(thumbnails) != 2 || thumbnails[0].TimeSeconds != 5 || thumbnails[1].TimeSeconds != 125 {
		t.Fatalf("unexpected thumbnails: %#v", thumbnails)
	}
}

func TestThumbnailJobMarksBrokenMediaAndContinues(t *testing.T) {
	f := setup(t)
	admin := login(t, f.handler, "admin")
	item, err := f.store.Media(context.Background(), f.photoID)
	if err != nil {
		t.Fatal(err)
	}
	item.ID = domain.InvalidID
	item.RelativePath = "2025/broken.jpg"
	item.Name = "broken.jpg"
	item.Path = filepath.Join(t.TempDir(), "missing.jpg")
	item.MIMEType = "image/jpeg"
	item.Kind = domain.KindImage
	if item, err = f.store.UpsertMedia(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	items, err := f.store.MediaForLibrary(context.Background(), f.libraryID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, media := range items {
		if media.ID == item.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("broken media is not in library items: %#v", items)
	}
	response := request(f.handler, http.MethodPost, fmt.Sprintf("/api/v1/admin/libraries/%d/thumbnails", f.libraryID), admin, nil)
	if response.Code != http.StatusAccepted {
		t.Fatalf("thumbnail job status = %d: %s", response.Code, response.Body)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		jobs := request(f.handler, http.MethodGet, "/api/v1/admin/jobs", admin, nil)
		if jobs.Code != http.StatusOK {
			t.Fatalf("jobs status = %d: %s", jobs.Code, jobs.Body)
		}
		var statuses []api.JobStatus
		if err := json.Unmarshal(jobs.Body.Bytes(), &statuses); err != nil {
			t.Fatal(err)
		}
		if len(statuses) != 0 && statuses[0].Status == "done" {
			if statuses[0].Processed != statuses[0].Total || statuses[0].Error == "" {
				t.Fatalf("unexpected completed job: %#v", statuses[0])
			}
			updated, err := f.store.Media(context.Background(), item.ID)
			if err != nil {
				t.Fatal(err)
			}
			if updated.ThumbnailError == "" {
				allItems, _ := f.store.MediaForLibrary(context.Background(), f.libraryID)
				t.Fatalf("media not marked non-convertible: job=%#v media=%#v all=%#v", statuses[0], updated, allItems)
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("thumbnail job did not finish")
}

func TestLoginCookieIsSecureBehindHTTPSProxy(t *testing.T) {
	f := setup(t)
	body := []byte(`{"login":"alice","password":"password"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("X-Forwarded-Proto", "https")
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, req)
	result := response.Result()
	if len(result.Cookies()) != 1 || !result.Cookies()[0].Secure {
		t.Fatalf("expected secure session cookie, got %#v", result.Cookies())
	}
}

func TestLoginCookieMaxAgeComesFromSettings(t *testing.T) {
	f := setup(t)
	settings, err := f.store.ServerSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.SessionMaxAgeHours = 48
	if err := f.store.SaveServerSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"login":"alice","password":"password"}`)
	response := request(f.handler, http.MethodPost, "/api/v1/auth/login", "", body)
	result := response.Result()
	if len(result.Cookies()) != 1 || result.Cookies()[0].MaxAge != 48*60*60 {
		t.Fatalf("expected 48 hour cookie, got %#v", result.Cookies())
	}
}

func TestChangePasswordRequiresCurrentPassword(t *testing.T) {
	f := setup(t)
	alice := login(t, f.handler, "alice")
	if got := request(f.handler, http.MethodPut, "/api/v1/me/password", alice, []byte(`{"currentPassword":"wrong","newPassword":"a-brand-new-password"}`)).Code; got != http.StatusForbidden {
		t.Fatalf("wrong current password status = %d", got)
	}
	if got := request(f.handler, http.MethodPut, "/api/v1/me/password", alice, []byte(`{"currentPassword":"password","newPassword":"a-brand-new-password"}`)).Code; got != http.StatusOK {
		t.Fatalf("valid change status = %d", got)
	}
	if _, err := f.store.Authenticate(context.Background(), "alice", "a-brand-new-password"); err != nil {
		t.Fatalf("login with new password failed: %v", err)
	}
	if _, err := f.store.Authenticate(context.Background(), "alice", "password"); err == nil {
		t.Fatal("old password still works")
	}
}

func TestSetEmailAndConflict(t *testing.T) {
	f := setup(t)
	alice := login(t, f.handler, "alice")
	if got := request(f.handler, http.MethodPut, "/api/v1/me/email", alice, []byte(`{"email":"ALICE@example.com"}`)).Code; got != http.StatusOK {
		t.Fatalf("set email status = %d", got)
	}
	user, err := f.store.User(context.Background(), f.aliceID)
	if err != nil || user.Email != "alice@example.com" {
		t.Fatalf("stored email = %q, err=%v", user.Email, err)
	}
	bob := login(t, f.handler, "bob")
	if got := request(f.handler, http.MethodPut, "/api/v1/me/email", bob, []byte(`{"email":"alice@example.com"}`)).Code; got != http.StatusConflict {
		t.Fatalf("duplicate email status = %d", got)
	}
	if got := request(f.handler, http.MethodPut, "/api/v1/me/email", alice, []byte(`{"email":"not-an-email"}`)).Code; got != http.StatusBadRequest {
		t.Fatalf("invalid email status = %d", got)
	}
}

func TestForgotPasswordWithoutSMTP(t *testing.T) {
	f := setup(t)
	response := request(f.handler, http.MethodPost, "/api/v1/auth/forgot-password", "", []byte(`{"email":"alice@example.com"}`))
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"smtpNotConfigured"`)) {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body)
	}
}

func TestForgotPasswordWithSMTPDoesNotRevealUnknownEmail(t *testing.T) {
	f := setup(t)
	settings, err := f.store.ServerSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.SMTPHost = "smtp.invalid"
	settings.SMTPFrom = "media@example.com"
	if err := f.store.SaveServerSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	response := request(f.handler, http.MethodPost, "/api/v1/auth/forgot-password", "", []byte(`{"email":"nobody@example.com"}`))
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"sent":true`)) {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body)
	}
}

func TestResetPasswordFlow(t *testing.T) {
	f := setup(t)
	if err := f.store.SetUserEmail(context.Background(), f.aliceID, "alice@example.com"); err != nil {
		t.Fatal(err)
	}
	token := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	sum := sha256.Sum256([]byte(token))
	expires := time.Now().UTC().Add(time.Hour)
	if err := f.store.CreatePasswordResetToken(context.Background(), f.aliceID, hex.EncodeToString(sum[:]), expires); err != nil {
		t.Fatal(err)
	}
	response := request(f.handler, http.MethodPost, "/api/v1/auth/reset-password", "", []byte(`{"token":"`+token+`","password":"a-brand-new-password"}`))
	if response.Code != http.StatusOK {
		t.Fatalf("reset status = %d: %s", response.Code, response.Body)
	}
	if _, err := f.store.Authenticate(context.Background(), "alice", "a-brand-new-password"); err != nil {
		t.Fatalf("login with reset password failed: %v", err)
	}
	replay := request(f.handler, http.MethodPost, "/api/v1/auth/reset-password", "", []byte(`{"token":"`+token+`","password":"another-new-password"}`))
	if replay.Code != http.StatusBadRequest {
		t.Fatalf("reused token status = %d", replay.Code)
	}
	if got := request(f.handler, http.MethodPost, "/api/v1/auth/reset-password", "", []byte(`{"token":"deadbeef","password":"a-brand-new-password"}`)).Code; got != http.StatusBadRequest {
		t.Fatalf("invalid token status = %d", got)
	}
}

func TestResetPasswordRejectsExpiredToken(t *testing.T) {
	f := setup(t)
	if err := f.store.SetUserEmail(context.Background(), f.aliceID, "alice@example.com"); err != nil {
		t.Fatal(err)
	}
	token := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	sum := sha256.Sum256([]byte(token))
	if err := f.store.CreatePasswordResetToken(context.Background(), f.aliceID, hex.EncodeToString(sum[:]), time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	response := request(f.handler, http.MethodPost, "/api/v1/auth/reset-password", "", []byte(`{"token":"`+token+`","password":"a-brand-new-password"}`))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expired token status = %d: %s", response.Code, response.Body)
	}
}

func TestFirstRunCreatesExactlyOneAdministrator(t *testing.T) {
	repository := openSQLite(t)
	handler := (&api.API{Store: repository, JWTSecret: []byte(secret)}).Handler()

	status := request(handler, http.MethodGet, "/api/v1/setup", "", nil)
	if status.Code != http.StatusOK || !bytes.Contains(status.Body.Bytes(), []byte(`"required":true`)) {
		t.Fatalf("unexpected setup status: %d %s", status.Code, status.Body)
	}

	payload := []byte(`{"login":"owner","password":"a-secure-password"}`)
	created := request(handler, http.MethodPost, "/api/v1/setup", "", payload)
	if created.Code != http.StatusCreated {
		t.Fatalf("setup status = %d: %s", created.Code, created.Body)
	}
	var user domain.User
	_ = json.Unmarshal(created.Body.Bytes(), &user)
	if user.Role != domain.RoleAdmin {
		t.Fatalf("initial user role = %q", user.Role)
	}

	repeated := request(handler, http.MethodPost, "/api/v1/setup", "", payload)
	if repeated.Code != http.StatusConflict {
		t.Fatalf("repeated setup status = %d", repeated.Code)
	}
	loginBody := []byte(`{"login":"owner","password":"a-secure-password"}`)
	if response := request(handler, http.MethodPost, "/api/v1/auth/login", "", loginBody); response.Code != http.StatusOK {
		t.Fatalf("initial administrator login = %d: %s", response.Code, response.Body)
	}
}

func TestFirstRunRejectsWeakPassword(t *testing.T) {
	repository := openSQLite(t)
	handler := (&api.API{Store: repository, JWTSecret: []byte(secret)}).Handler()
	response := request(handler, http.MethodPost, "/api/v1/setup", "", []byte(`{"login":"owner","password":"short"}`))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("weak password status = %d", response.Code)
	}
	required, _ := repository.SetupRequired(context.Background())
	if !required {
		t.Fatal("failed setup must not create a user")
	}
}

func TestCodecIsUserSetting(t *testing.T) {
	f := setup(t)
	alice := login(t, f.handler, "alice")
	response := request(f.handler, http.MethodPut, "/api/v1/settings", alice, []byte(`{"theme":"dark","codec":"vp9"}`))
	if response.Code != http.StatusOK {
		t.Fatalf("user codec status = %d", response.Code)
	}
	settings, _ := f.store.UserSettings(context.Background(), f.aliceID)
	if settings.Codec != "vp9" {
		t.Fatalf("stored codec = %q", settings.Codec)
	}
	if settings.Zoom != 100 {
		t.Fatalf("missing zoom must default to 100, got %d", settings.Zoom)
	}
	adminSettings, _ := f.store.UserSettings(context.Background(), 1)
	if adminSettings.Codec != "h264" {
		t.Fatalf("default admin codec = %q", adminSettings.Codec)
	}
	if adminSettings.Zoom != 100 {
		t.Fatalf("default admin zoom = %d", adminSettings.Zoom)
	}
}

func TestUserZoomIsStoredPerUser(t *testing.T) {
	f := setup(t)
	alice := login(t, f.handler, "alice")
	response := request(f.handler, http.MethodPut, "/api/v1/settings", alice, []byte(`{"theme":"dark","codec":"h264","zoom":125}`))
	if response.Code != http.StatusOK {
		t.Fatalf("user zoom status = %d", response.Code)
	}
	settings, _ := f.store.UserSettings(context.Background(), f.aliceID)
	if settings.Zoom != 125 {
		t.Fatalf("stored zoom = %d", settings.Zoom)
	}
	adminSettings, _ := f.store.UserSettings(context.Background(), 1)
	if adminSettings.Zoom != 100 {
		t.Fatalf("zoom must not leak between users, admin = %d", adminSettings.Zoom)
	}
}

func TestSettingsRejectInvalidZoom(t *testing.T) {
	f := setup(t)
	alice := login(t, f.handler, "alice")
	response := request(f.handler, http.MethodPut, "/api/v1/settings", alice, []byte(`{"theme":"dark","codec":"h264","zoom":200}`))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid zoom status = %d", response.Code)
	}
}

func TestSettingsAcceptSystemTheme(t *testing.T) {
	f := setup(t)
	alice := login(t, f.handler, "alice")
	response := request(f.handler, http.MethodPut, "/api/v1/settings", alice, []byte(`{"theme":"system","codec":"h264","zoom":100}`))
	if response.Code != http.StatusOK {
		t.Fatalf("system theme status = %d", response.Code)
	}
	settings, _ := f.store.UserSettings(context.Background(), f.aliceID)
	if settings.Theme != "system" {
		t.Fatalf("stored theme = %q", settings.Theme)
	}
}

func TestSettingsRejectCodecOutsideAllowList(t *testing.T) {
	f := setup(t)
	alice := login(t, f.handler, "alice")
	response := request(f.handler, http.MethodPut, "/api/v1/settings", alice, []byte(`{"theme":"dark","codec":"av1"}`))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid codec status = %d", response.Code)
	}
}

func TestSettingsRejectInvalidLogLevel(t *testing.T) {
	f := setup(t)
	admin := login(t, f.handler, "admin")
	response := request(f.handler, http.MethodPut, "/api/v1/admin/settings", admin, []byte(`{"transcodeCodec":"h264","httpEnabled":true,"httpsEnabled":false,"logLevel":"verbose"}`))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid log level status = %d", response.Code)
	}
}

func TestAdminCanReadRecentLogs(t *testing.T) {
	repository := openSQLite(t)
	if _, err := repository.CreateInitialAdmin(context.Background(), domain.User{ID: domain.InvalidID, Login: "admin"}, "password"); err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(logFile, []byte("D hidden detail\nI server started\nE thumbnail failed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := (&api.API{
		Store: repository, Scanner: scanner.Scanner{Store: repository},
		JWTSecret: []byte(secret), ThumbnailDir: t.TempDir(), LogFile: logFile,
	}).Handler()
	admin := login(t, handler, "admin")
	response := request(handler, http.MethodGet, "/api/v1/admin/logs?limit=2", admin, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body)
	}
	var payload struct {
		Path  string   `json:"path"`
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Path != logFile || len(payload.Lines) != 2 || payload.Lines[0] != "I server started" || payload.Lines[1] != "E thumbnail failed" {
		t.Fatalf("unexpected logs payload: %#v", payload)
	}
}
