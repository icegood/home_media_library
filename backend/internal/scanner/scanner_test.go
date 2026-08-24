package scanner_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"media-library/backend/internal/domain"
	"media-library/backend/internal/metadata"
	"media-library/backend/internal/scanner"
	"media-library/backend/internal/store"
)

func openSQLite(t *testing.T) *store.SQLite {
	t.Helper()
	repository, err := store.NewSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repository.Close() })
	return repository
}

func TestScanPreservesRelativeFoldersAndFiltersFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "family", "2025"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "family", "empty", "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "family", "2025", "photo.JPG"), []byte("image"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "archive", "old.mp4"), []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "archive", "camera.MPG"), []byte("mpeg video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "archive", "old_libvpx-vp9_libmp3lame.mkv"), []byte("generated"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "family", "notes.txt"), []byte("ignore"), 0o644); err != nil {
		t.Fatal(err)
	}
	repository := openSQLite(t)
	library := scanner.NewLibrary("Family", []domain.LibraryRoot{
		{ID: domain.InvalidID, Path: filepath.Join(root, "family")},
		{ID: domain.InvalidID, Path: filepath.Join(root, "archive")},
	})
	var err error
	library, err = repository.CreateLibrary(context.Background(), library)
	if err != nil {
		t.Fatal(err)
	}
	subject := scanner.Scanner{Store: repository}
	if err := subject.Scan(context.Background(), library); err != nil {
		t.Fatal(err)
	}
	entries, _ := repository.Entries(context.Background(), 0, library.ID, "")
	if len(entries) != 2 || entries[0].Type != "folder" || entries[0].RelativePath != "archive" ||
		entries[1].RelativePath != "family" {
		t.Fatalf("unexpected root entries: %#v", entries)
	}
	entries, _ = repository.Entries(context.Background(), 0, library.ID, "family")
	if len(entries) != 2 || entries[0].RelativePath != "family/2025" || entries[1].RelativePath != "family/empty" {
		t.Fatalf("unexpected current entries: %#v", entries)
	}
	entries, _ = repository.Entries(context.Background(), 0, library.ID, "family/empty")
	if len(entries) != 1 || entries[0].Type != "folder" || entries[0].RelativePath != "family/empty/child" {
		t.Fatalf("unexpected empty-folder entries: %#v", entries)
	}
	entries, _ = repository.Entries(context.Background(), 0, library.ID, "family/2025")
	if len(entries) != 1 || entries[0].RelativePath != "family/2025/photo.JPG" ||
		entries[0].Media.RelativePath != "family/2025/photo.JPG" ||
		entries[0].Media.MIMEType != "image/jpeg" {
		t.Fatalf("unexpected media entries: %#v", entries)
	}
	entries, _ = repository.Entries(context.Background(), 0, library.ID, "archive")
	mimeTypes := map[string]string{}
	for _, entry := range entries {
		mimeTypes[entry.Name] = entry.Media.MIMEType
	}
	if len(entries) != 3 || mimeTypes["camera.MPG"] != "video/mpeg" || mimeTypes["old_libvpx-vp9_libmp3lame.mkv"] != "video/x-matroska" || mimeTypes["old.mp4"] != "video/mp4" {
		t.Fatalf("unexpected archive entries: %#v", entries)
	}
}

func TestMIMETypeForPathIsResolvedFromDatabaseCaseInsensitively(t *testing.T) {
	repository := openSQLite(t)
	subject := scanner.Scanner{Store: repository}
	cases := map[string]string{
		"photo.JPG":  "image/jpeg",
		"photo.JpEg": "image/jpeg",
		"clip.MOV":   "video/quicktime",
		"clip.MPG":   "video/mpeg",
		"movie.MpEg": "video/mpeg",
	}
	for name, expected := range cases {
		mimeType, ok := subject.MIMETypeForPath(context.Background(), name)
		if !ok || mimeType != expected {
			t.Fatalf("%s => %q, %v; want %q", name, mimeType, ok, expected)
		}
	}
	if _, ok := subject.MIMETypeForPath(context.Background(), "notes.TXT"); ok {
		t.Fatal("txt should not be treated as media")
	}
}

func TestScanReportsPerRootTotalsForOverlappingRoots(t *testing.T) {
	root := t.TempDir()
	album := filepath.Join(root, "album")
	if err := os.MkdirAll(filepath.Join(album, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		filepath.Join(album, "one.JPG"):         []byte("image"),
		filepath.Join(album, "two.MPG"):         []byte("video"),
		filepath.Join(album, "nested", "x.mov"): []byte("video"),
		filepath.Join(album, "notes.txt"):       []byte("ignore"),
	}
	for path, data := range files {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(album, "fake.JPG"), 0o755); err != nil {
		t.Fatal(err)
	}
	repository := openSQLite(t)
	subject := scanner.Scanner{Store: repository}
	total := 0
	walkedRoots := 0
	subject = subject.WithTotalReady(func(n int) { total += n; walkedRoots++ })
	library := domain.Library{Roots: []domain.LibraryRoot{
		{Path: album},
		{Path: filepath.Join(album, "nested")},
	}}
	if err := subject.Scan(context.Background(), library); err != nil {
		t.Fatal(err)
	}
	// Totals are reported per root, so the overlapping nested root reports
	// x.mov twice; imports stay unique because media rows are keyed by path.
	if total != 4 || walkedRoots != 2 {
		t.Fatalf("total = %d across %d roots, want 4 across 2", total, walkedRoots)
	}
	for path := range files {
		if path == filepath.Join(album, "notes.txt") {
			continue
		}
		if _, err := repository.MediaByPath(context.Background(), path); err != nil {
			t.Fatalf("%s not imported: %v", path, err)
		}
	}
}

func TestScanImportsGeneratedDerivativeMedia(t *testing.T) {
	root := t.TempDir()
	photos := filepath.Join(root, "photos")
	if err := os.MkdirAll(photos, 0o755); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(photos, "clip.MOV")
	generated := filepath.Join(photos, "clip_libvpx-vp9_libmp3lame.mkv")
	if err := os.WriteFile(original, []byte("mov"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(generated, []byte("generated"), 0o644); err != nil {
		t.Fatal(err)
	}
	repository := openSQLite(t)
	library := scanner.NewLibrary("Photos", []domain.LibraryRoot{{ID: domain.InvalidID, Path: photos}})
	var err error
	library, err = repository.CreateLibrary(context.Background(), library)
	if err != nil {
		t.Fatal(err)
	}
	subject := scanner.Scanner{Store: repository}
	if err := subject.Scan(context.Background(), library); err != nil {
		t.Fatal(err)
	}
	entries, _ := repository.Entries(context.Background(), 0, library.ID, "photos")
	if len(entries) != 2 || entries[0].Name != "clip.MOV" || entries[1].Name != "clip_libvpx-vp9_libmp3lame.mkv" || entries[1].Media.MIMEType != "video/x-matroska" {
		t.Fatalf("generated derivative should be imported, got %#v", entries)
	}
	if err := os.Remove(original); err != nil {
		t.Fatal(err)
	}
	if err := subject.Scan(context.Background(), library); err != nil {
		t.Fatal(err)
	}
	entries, _ = repository.Entries(context.Background(), 0, library.ID, "photos")
	if len(entries) != 1 || entries[0].Name != "clip_libvpx-vp9_libmp3lame.mkv" {
		t.Fatalf("removed original should be pruned without removing derivative, got %#v", entries)
	}
}

func TestSingleRootLibraryOpensAtRootChildren(t *testing.T) {
	root := t.TempDir()
	trip := filepath.Join(root, "20100821_karpaty_lakes_pip_ivan")
	if err := os.MkdirAll(filepath.Join(trip, "DSC-H3"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(trip, "NIKON_D50"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(trip, "DSC-H3", "one.JPG"), []byte("image"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(trip, "NIKON_D50", "two.jpg"), []byte("image"), 0o644); err != nil {
		t.Fatal(err)
	}
	repository := openSQLite(t)
	library := scanner.NewLibrary("Trip", []domain.LibraryRoot{{ID: domain.InvalidID, Path: trip}})
	var err error
	library, err = repository.CreateLibrary(context.Background(), library)
	if err != nil {
		t.Fatal(err)
	}
	subject := scanner.Scanner{Store: repository}
	if err := subject.Scan(context.Background(), library); err != nil {
		t.Fatal(err)
	}
	entries, _ := repository.Entries(context.Background(), 0, library.ID, "")
	if len(entries) != 1 || entries[0].ID != library.Roots[0].ID || entries[0].RelativePath != "20100821_karpaty_lakes_pip_ivan" {
		t.Fatalf("library root should show the picked root folder, got %#v", entries)
	}
	folderEntries, _ := repository.EntriesForFolder(context.Background(), 0, library.ID, library.Roots[0].ID)
	if len(folderEntries.Entries) != 2 || folderEntries.Entries[0].RelativePath != "DSC-H3" || folderEntries.Entries[1].RelativePath != "NIKON_D50" {
		t.Fatalf("root folder should open at disk subfolders, got %#v", folderEntries.Entries)
	}
}

func TestScanRejectsNonDirectoryRoot(t *testing.T) {
	subject := scanner.Scanner{Store: openSQLite(t)}
	if err := subject.Scan(context.Background(), domain.Library{ID: domain.InvalidID, Roots: []domain.LibraryRoot{
		{ID: domain.InvalidID, Path: filepath.Join(t.TempDir(), "missing")},
	}}); err == nil {
		t.Fatal("expected error for missing root directory")
	}
}

func TestSharedPhysicalRootUsesOneMediaIdentity(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "shared"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "shared", "same.jpg"), []byte("image"), 0o644); err != nil {
		t.Fatal(err)
	}
	repository := openSQLite(t)
	shared := filepath.Join(root, "shared")
	first := scanner.NewLibrary("First", []domain.LibraryRoot{{ID: domain.InvalidID, Path: shared}})
	second := scanner.NewLibrary("Second", []domain.LibraryRoot{{ID: domain.InvalidID, Path: shared}})
	var err error
	first, err = repository.CreateLibrary(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	second, err = repository.CreateLibrary(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	subject := scanner.Scanner{Store: repository}
	if err := subject.Scan(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := subject.Scan(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	firstEntries, _ := repository.Entries(context.Background(), 0, first.ID, "shared")
	secondEntries, _ := repository.Entries(context.Background(), 0, second.ID, "shared")
	if len(firstEntries) != 1 || len(secondEntries) != 1 {
		t.Fatalf("unexpected entries: first=%#v second=%#v", firstEntries, secondEntries)
	}
	if firstEntries[0].Media.ID != secondEntries[0].Media.ID {
		t.Fatalf("shared file was duplicated: %d != %d", firstEntries[0].Media.ID, secondEntries[0].Media.ID)
	}
}

func TestRefreshSkipsAlreadyIndexedMedia(t *testing.T) {
	root := t.TempDir()
	countFile := filepath.Join(root, "count")
	if err := os.WriteFile(countFile, []byte("0"), 0o644); err != nil {
		t.Fatal(err)
	}
	countingTool := filepath.Join(root, "count-exif")
	if err := os.WriteFile(countingTool, []byte("#!/bin/sh\nn=$(cat \""+countFile+"\")\nn=$((n+1))\necho \"$n\" > \""+countFile+"\"\necho '{\"SourceFile\":\"x.jpg\"}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	photos := filepath.Join(root, "photos")
	if err := os.MkdirAll(photos, 0o755); err != nil {
		t.Fatal(err)
	}
	photoPath := filepath.Join(photos, "one.jpg")
	if err := os.WriteFile(photoPath, []byte("image"), 0o644); err != nil {
		t.Fatal(err)
	}
	repository := openSQLite(t)
	library := scanner.NewLibrary("Photos", []domain.LibraryRoot{{ID: domain.InvalidID, Path: photos}})
	var err error
	library, err = repository.CreateLibrary(context.Background(), library)
	if err != nil {
		t.Fatal(err)
	}
	subject := scanner.Scanner{
		Store:    repository,
		Metadata: metadata.Extractor{ExifTool: countingTool, FFProbe: filepath.Join(root, "missing-ffprobe"), Timeout: time.Second},
	}
	if err := subject.Scan(context.Background(), library); err != nil {
		t.Fatal(err)
	}
	count, err := os.ReadFile(countFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(count) != "1\n" {
		t.Fatalf("first scan should extract metadata once, count=%q", count)
	}
	if err := subject.Scan(context.Background(), library); err != nil {
		t.Fatal(err)
	}
	count, err = os.ReadFile(countFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(count) != "1\n" {
		t.Fatalf("refresh should not re-extract metadata for existing media, count=%q", count)
	}
}

func TestMetadataErrorIsStoredAndPreventsRetry(t *testing.T) {
	root := t.TempDir()
	failingTool := filepath.Join(root, "fail-exif")
	countFile := filepath.Join(root, "count")
	if err := os.WriteFile(failingTool, []byte("#!/bin/sh\nn=0\n[ -f \""+countFile+"\" ] && n=$(cat \""+countFile+"\")\nn=$((n+1))\necho \"$n\" > \""+countFile+"\"\necho broken metadata >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	photos := filepath.Join(root, "photos")
	if err := os.MkdirAll(photos, 0o755); err != nil {
		t.Fatal(err)
	}
	photoPath := filepath.Join(photos, "broken.jpg")
	if err := os.WriteFile(photoPath, []byte("bad jpg"), 0o644); err != nil {
		t.Fatal(err)
	}
	repository := openSQLite(t)
	library := scanner.NewLibrary("Photos", []domain.LibraryRoot{{ID: domain.InvalidID, Path: photos}})
	var err error
	library, err = repository.CreateLibrary(context.Background(), library)
	if err != nil {
		t.Fatal(err)
	}
	subject := scanner.Scanner{
		Store:    repository,
		Metadata: metadata.Extractor{ExifTool: failingTool, FFProbe: filepath.Join(root, "missing-ffprobe"), Timeout: time.Second},
	}
	if err := subject.Scan(context.Background(), library); err != nil {
		t.Fatal(err)
	}
	entries, _ := repository.Entries(context.Background(), 0, library.ID, "photos")
	if len(entries) != 1 || entries[0].Media == nil || entries[0].Media.MetadataError == "" {
		t.Fatalf("metadata error was not stored: %#v", entries)
	}
	if err := subject.Scan(context.Background(), library); err != nil {
		t.Fatal(err)
	}
	count, err := os.ReadFile(countFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(count) != "1\n" {
		t.Fatalf("metadata extractor retried despite stored error, count=%q", count)
	}
}
