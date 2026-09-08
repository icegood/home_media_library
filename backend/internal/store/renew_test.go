package store_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"media-library/backend/internal/domain"
	"media-library/backend/internal/store"
)

// verifyMetadataRenewOptions asserts the metadata-renew write policy shared by
// both stores: each MetadataWriteOptions flag makes its field overwrite an
// existing value; unchecked it only fills still-empty fields; and an absent
// extracted value never wipes stored data.
func verifyMetadataRenewOptions(t *testing.T, repository store.Store) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "photos")
	library := domain.Library{ID: domain.InvalidID, Name: "Photos", Roots: []domain.LibraryRoot{{ID: domain.InvalidID, Path: root}}}
	var err error
	library, err = repository.CreateLibrary(context.Background(), library)
	if err != nil {
		t.Fatal(err)
	}
	folderID := library.Roots[0].ID
	full, err := repository.UpsertMedia(context.Background(), domain.Media{ID: domain.InvalidID, FolderID: folderID,
		Path: filepath.Join(root, "full.jpg"), Name: "full.jpg", Kind: domain.KindImage, MIMEType: "image/jpeg",
		Metadata: map[string]any{"stored": true}, GPS: "10.5,20.25", TakenAt: "2020-01-01T00:00:00Z", MetadataError: "old error"})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := repository.UpsertMedia(context.Background(), domain.Media{ID: domain.InvalidID, FolderID: folderID,
		Path: filepath.Join(root, "empty.jpg"), Name: "empty.jpg", Kind: domain.KindImage, MIMEType: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}

	apply := func(id int, opts domain.MetadataWriteOptions) {
		t.Helper()
		if err := repository.UpdateMediaMetadata(context.Background(), id, map[string]any{"fresh": true}, "3,4", "2021-02-03T04:05:06Z", "new error", opts); err != nil {
			t.Fatal(err)
		}
	}
	load := func(id int) domain.Media {
		t.Helper()
		list, err := repository.MediaBatch(context.Background(), []int{id})
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 {
			t.Fatalf("media %d not found: %#v", id, list)
		}
		return list[0]
	}
	metadataJSON := func(item domain.Media) string {
		raw, err := json.Marshal(item.Metadata)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}

	// No options: populated fields survive, empty fields get filled.
	apply(full.ID, domain.MetadataWriteOptions{})
	if item := load(full.ID); metadataJSON(item) != `{"stored":true}` || item.MetadataError != "old error" || item.GPS != "10.5,20.25" || item.TakenAt != "2020-01-01T00:00:00Z" {
		t.Fatalf("unchecked renew must keep populated fields, got %#v", item)
	}
	apply(empty.ID, domain.MetadataWriteOptions{})
	if item := load(empty.ID); metadataJSON(item) != `{"fresh":true}` || item.MetadataError != "new error" || item.GPS != "3,4" || item.TakenAt != "2021-02-03T04:05:06Z" {
		t.Fatalf("unchecked renew must fill empty fields, got %#v", item)
	}

	// recreateExisting: metadata and its error are rewritten, nothing else.
	apply(full.ID, domain.MetadataWriteOptions{RecreateExisting: true})
	if item := load(full.ID); metadataJSON(item) != `{"fresh":true}` || item.MetadataError != "new error" || item.GPS != "10.5,20.25" || item.TakenAt != "2020-01-01T00:00:00Z" {
		t.Fatalf("recreateExisting must rewrite metadata/error only, got %#v", item)
	}

	// updateGps: coordinates replaced, everything else untouched.
	apply(full.ID, domain.MetadataWriteOptions{UpdateGPS: true})
	if item := load(full.ID); item.GPS != "3,4" || item.TakenAt != "2020-01-01T00:00:00Z" {
		t.Fatalf("updateGps must overwrite gps only, got %#v", item)
	}

	// updateTakenAt: timestamp replaced.
	apply(full.ID, domain.MetadataWriteOptions{UpdateTakenAt: true})
	if item := load(full.ID); item.TakenAt != "2021-02-03T04:05:06Z" {
		t.Fatalf("updateTakenAt must overwrite taken_at, got %#v", item)
	}

	// Empty extraction results must not wipe stored values even when the
	// option is on.
	if err := repository.UpdateMediaMetadata(context.Background(), full.ID, map[string]any{"fresh": true}, "", "", "new error", domain.MetadataWriteOptions{UpdateGPS: true, UpdateTakenAt: true}); err != nil {
		t.Fatal(err)
	}
	if item := load(full.ID); item.GPS != "3,4" || item.TakenAt != "2021-02-03T04:05:06Z" {
		t.Fatalf("absent extracted gps/taken_at must not wipe stored values, got %#v", item)
	}
}

func TestSQLiteUpdateMediaMetadataOptionPolicy(t *testing.T) {
	repository, _ := openSQLite(t)
	verifyMetadataRenewOptions(t, repository)
}

func TestPostgresUpdateMediaMetadataOptionPolicy(t *testing.T) {
	repository := openPostgres(t, true)
	verifyMetadataRenewOptions(t, repository)
}
