package embyimport

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestReadTransfersLibrariesUsersAndAccess(t *testing.T) {
	dir := t.TempDir()
	usersDB := filepath.Join(dir, "data", "users.db")
	libraryDB := filepath.Join(dir, "data", "library.db")
	configDir := filepath.Join(dir, "config", "users")
	if err := os.MkdirAll(filepath.Join(configDir, "ice-guid"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "ice-guid", "policy.xml"), []byte(`<?xml version="1.0"?><UserPolicy><IsAdministrator>true</IsAdministrator></UserPolicy>`), 0o644); err != nil {
		t.Fatal(err)
	}
	mustExec(t, usersDB, `
CREATE TABLE LocalUsersv2 (Id INTEGER PRIMARY KEY AUTOINCREMENT, guid BLOB NOT NULL, data TEXT NOT NULL) STRICT;
INSERT INTO LocalUsersv2 (guid,data) VALUES
  (x'01', '{"Name":"ice","IdString":"ice-guid","Password":"5BAA61E4C9B93F3F0682250B6CF8331B7EE68FD8"}'),
  (x'02', '{"Name":"sveta","IdString":"sveta-guid","Password":"not-a-valid-hash"}');`)
	mustExec(t, libraryDB, `
CREATE TABLE MediaItems (Id INTEGER PRIMARY KEY AUTOINCREMENT, Type INT, ParentId INT, Name TEXT, Path TEXT) STRICT;
CREATE TABLE ItemExtradataTypes (ExtradataTypeId INTEGER PRIMARY KEY AUTOINCREMENT, Name TEXT NOT NULL COLLATE NOCASE) STRICT;
CREATE TABLE ItemExtradata (ItemId INT NOT NULL, ExtradataTypeId INT NOT NULL, Value TEXT NOT NULL, PRIMARY KEY (ItemId, ExtradataTypeId)) STRICT, WITHOUT ROWID;
CREATE TABLE UserItemShares (UserId INT NOT NULL, ItemId INT NOT NULL, ShareLevel INT NOT NULL, PRIMARY KEY (UserId, ItemId)) STRICT, WITHOUT ROWID;
INSERT INTO ItemExtradataTypes (ExtradataTypeId,Name) VALUES (2,'LibraryOptions');
INSERT INTO MediaItems (Id,Type,ParentId,Name,Path) VALUES
  (1,1,NULL,'root',NULL),
  (2,2,NULL,'Media Folders',NULL),
  (3,4,2,'Family',NULL),
  (4,3,1,'family','/data1/family'),
  (5,4,2,'Private',NULL),
  (6,3,1,'private','/data2/private');
INSERT INTO ItemExtradata VALUES
  (3,2,'{"PathInfos":[{"Path":"/data1/family"},{"Path":"/data1/family"}]}'),
  (5,2,'{"PathInfos":[{"Path":"/data2/private"}]}');
INSERT INTO UserItemShares VALUES
  (1,3,100),
  (2,3,100),
  (2,5,0);`)

	snapshot, err := Read(context.Background(), Options{
		ConfigRoot:   dir,
		PathMappings: []PathMapping{{From: "/data1", To: "/media/data1"}, {From: "/data2", To: "/media/data2"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Users) != 2 || snapshot.Users[0].ID != 1 || snapshot.Users[0].Role != "admin" {
		t.Fatalf("unexpected users: %#v", snapshot.Users)
	}
	if snapshot.Users[0].PasswordHash != "emby-sha1:5baa61e4c9b93f3f0682250b6cf8331b7ee68fd8" || snapshot.Users[1].PasswordHash != "" {
		t.Fatalf("unexpected imported password formats")
	}
	if len(snapshot.Libraries) != 2 || snapshot.Libraries[0].ID != 3 || snapshot.Libraries[0].Roots[0].Path != "/media/data1/family" {
		t.Fatalf("unexpected libraries: %#v", snapshot.Libraries)
	}
	if len(snapshot.Access) != 2 || snapshot.Access[1].LibraryID != 3 || snapshot.Access[1].UserID != 2 {
		t.Fatalf("unexpected access: %#v", snapshot.Access)
	}
}

func mustExec(t *testing.T, path string, ddl string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(ddl); err != nil {
		t.Fatal(err)
	}
}
