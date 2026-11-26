package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"

	"media-library/backend/internal/domain"
)

func idsOf[T any](t *testing.T, body []byte, id func(T) int) map[int]bool {
	t.Helper()
	var items []T
	if err := json.Unmarshal(body, &items); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	out := map[int]bool{}
	for _, item := range items {
		out[id(item)] = true
	}
	return out
}

// A folder hidden from its parent (show_as_child = false) is left out of the
// parent's folder list, timelines and maps for everyone, is unreadable for
// regular users even by direct URL, and stays openable for admins, who find
// it in the admin list of hidden folders.
func TestHiddenFolderIsLeftOutOfItsAncestorsAndAdminOnly(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	photo, _ := f.store.Media(ctx, f.photoID)
	photo.GPS = "50.45,30.52"
	if _, err := f.store.UpsertMedia(ctx, photo); err != nil {
		t.Fatal(err)
	}
	hiddenPath := filepath.Join(f.mediaRoot, "family", "2025", "private")
	hidden, err := f.store.UpsertFolder(ctx, domain.MediaFolder{ID: domain.InvalidID, ParentID: f.folderID, Path: hiddenPath, RelativePath: "2025/private"})
	if err != nil {
		t.Fatal(err)
	}
	nested, err := f.store.UpsertFolder(ctx, domain.MediaFolder{ID: domain.InvalidID, ParentID: hidden.ID, Path: filepath.Join(hiddenPath, "deep"), RelativePath: "2025/private/deep"})
	if err != nil {
		t.Fatal(err)
	}
	secret, err := f.store.UpsertMedia(ctx, domain.Media{ID: domain.InvalidID, FolderID: nested.ID, Path: filepath.Join(hiddenPath, "deep", "secret.jpg"), RelativePath: "2025/private/deep/secret.jpg", Name: "secret.jpg", Kind: domain.KindImage, MIMEType: "image/jpeg", TakenAt: "2025-06-01T00:00:00Z", GPS: "50.46,30.53"})
	if err != nil {
		t.Fatal(err)
	}
	admin := login(t, f.handler, "admin")
	alice := login(t, f.handler, "alice")
	setShow := func(session string, show bool) int {
		body, _ := json.Marshal(map[string]bool{"showAsChild": show})
		return request(f.handler, http.MethodPut, fmt.Sprintf("/api/v1/admin/folders/%d/show-as-child", hidden.ID), session, body).Code
	}
	if got := setShow(alice, false); got != http.StatusForbidden {
		t.Fatalf("regular user hiding a folder = %d, want 403", got)
	}
	if got := request(f.handler, http.MethodGet, "/api/v1/admin/hidden-folders", alice, nil).Code; got != http.StatusForbidden {
		t.Fatalf("regular user listing hidden folders = %d, want 403", got)
	}
	if got := request(f.handler, http.MethodPut, "/api/v1/admin/folders/999999/show-as-child", admin, []byte(`{"showAsChild":false}`)).Code; got != http.StatusNotFound {
		t.Fatalf("hiding a missing folder = %d, want 404", got)
	}
	if got := request(f.handler, http.MethodPut, fmt.Sprintf("/api/v1/admin/folders/%d/show-as-child", hidden.ID), admin, []byte(`{}`)).Code; got != http.StatusBadRequest {
		t.Fatalf("missing showAsChild = %d, want 400", got)
	}
	media := func(item domain.Media) int { return item.ID }
	mapMedia := func(item domain.MapMedia) int { return item.ID }
	entry := func(item domain.Entry) int { return item.ID }
	folderEntries := func(session string) map[int]bool {
		response := request(f.handler, http.MethodGet, fmt.Sprintf("/api/v1/libraries/%d/folders/%d/entries", f.libraryID, f.folderID), session, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("entries = %d: %s", response.Code, response.Body)
		}
		var payload struct{ Entries json.RawMessage }
		_ = json.Unmarshal(response.Body.Bytes(), &payload)
		return idsOf(t, payload.Entries, entry)
	}
	get := func(session, path string) []byte {
		response := request(f.handler, http.MethodGet, path, session, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s = %d: %s", path, response.Code, response.Body)
		}
		return response.Body.Bytes()
	}
	libraryTimeline := fmt.Sprintf("/api/v1/libraries/%d/media", f.libraryID)
	folderTimeline := fmt.Sprintf("/api/v1/libraries/%d/folders/%d/media", f.libraryID, f.folderID)
	libraryMap := fmt.Sprintf("/api/v1/map?library=%d", f.libraryID)

	// Visible by default.
	if !folderEntries(alice)[hidden.ID] || !idsOf(t, get(alice, folderTimeline), media)[secret.ID] || !idsOf(t, get(alice, libraryMap), mapMedia)[secret.ID] {
		t.Fatal("a folder must be shown in its parent by default")
	}

	if got := setShow(admin, false); got != http.StatusNoContent {
		t.Fatalf("hide = %d, want 204", got)
	}
	var list []domain.HiddenFolder
	if err := json.Unmarshal(get(admin, "/api/v1/admin/hidden-folders"), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != hidden.ID || list[0].LibraryID != f.libraryID || list[0].LibraryName != "Family" || list[0].RelativePath != "family/2025/private" {
		t.Fatalf("hidden folders = %#v", list)
	}
	for _, session := range []string{alice, admin} {
		if folderEntries(session)[hidden.ID] {
			t.Fatal("hidden folder listed in its parent")
		}
		for _, path := range []string{libraryTimeline, folderTimeline} {
			got := idsOf(t, get(session, path), media)
			if got[secret.ID] || !got[f.photoID] {
				t.Fatalf("GET %s = %v, want the parent's photo without the hidden one", path, got)
			}
		}
		got := idsOf(t, get(session, libraryMap), mapMedia)
		if got[secret.ID] || !got[f.photoID] {
			t.Fatalf("map = %v, want the hidden photo left out", got)
		}
	}
	// Regular users cannot open it, nor anything inside it, even directly.
	for _, path := range []string{
		fmt.Sprintf("/api/v1/libraries/%d/folders/%d/media", f.libraryID, hidden.ID),
		fmt.Sprintf("/api/v1/libraries/%d/folders/%d/entries", f.libraryID, nested.ID),
		fmt.Sprintf("/api/v1/media/%d", secret.ID),
	} {
		if got := request(f.handler, http.MethodGet, path, alice, nil).Code; got != http.StatusForbidden && got != http.StatusNotFound {
			t.Fatalf("regular user GET %s = %d, want 403/404", path, got)
		}
	}
	// Admins open it directly: its own timeline still shows its content.
	if !idsOf(t, get(admin, fmt.Sprintf("/api/v1/libraries/%d/folders/%d/media", f.libraryID, hidden.ID)), media)[secret.ID] {
		t.Fatal("admin must see a hidden folder's media when opening it directly")
	}
	if got := request(f.handler, http.MethodGet, fmt.Sprintf("/api/v1/media/%d", secret.ID), admin, nil).Code; got != http.StatusOK {
		t.Fatalf("admin GET hidden media = %d", got)
	}

	// Shown again.
	if got := setShow(admin, true); got != http.StatusNoContent {
		t.Fatalf("unhide = %d", got)
	}
	if !folderEntries(alice)[hidden.ID] || !idsOf(t, get(alice, libraryTimeline), media)[secret.ID] {
		t.Fatal("an unhidden folder must be shown again")
	}
	if len(idsOf(t, get(admin, "/api/v1/admin/hidden-folders"), func(item domain.HiddenFolder) int { return item.ID })) != 0 {
		t.Fatal("hidden list must be empty after unhiding")
	}
}
