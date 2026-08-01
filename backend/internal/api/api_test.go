package api_test

import (
	"bytes"
	"context"
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
	}).Handler(), store: repository, mediaRoot: mediaRoot, thumbnailDir: thumbnailDir, libraryID: library.ID, folderID: folder.ID, photoID: photo.ID}
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
	if err := os.MkdirAll(thumbDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(thumbDir, fmt.Sprintf("%d_0.jpg", f.photoID)), []byte("thumb"), 0o640); err != nil {
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

func TestOnlyAdminCanChangeTranscodeCodec(t *testing.T) {
	f := setup(t)
	alice := login(t, f.handler, "alice")
	admin := login(t, f.handler, "admin")
	payload := []byte(`{"transcodeCodec":"vp9","httpEnabled":true,"httpsEnabled":false,"finishedJobRetentionMinutes":15}`)
	if got := request(f.handler, http.MethodPut, "/api/v1/admin/settings", alice, payload).Code; got != http.StatusForbidden {
		t.Fatalf("regular user status = %d", got)
	}
	if got := request(f.handler, http.MethodPut, "/api/v1/admin/settings", admin, payload).Code; got != http.StatusOK {
		t.Fatalf("admin status = %d", got)
	}
	settings, _ := f.store.ServerSettings(context.Background())
	if settings.TranscodeCodec != "vp9" {
		t.Fatalf("stored codec = %q", settings.TranscodeCodec)
	}
	if settings.FinishedJobRetentionMinutes != 15 {
		t.Fatalf("stored finished job retention = %d", settings.FinishedJobRetentionMinutes)
	}
}

func TestSettingsRejectCodecOutsideAllowList(t *testing.T) {
	f := setup(t)
	admin := login(t, f.handler, "admin")
	response := request(f.handler, http.MethodPut, "/api/v1/admin/settings", admin, []byte(`{"transcodeCodec":"av1"}`))
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
