package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"media-library/backend/internal/domain"
)

// temporaryPassword returns a random one-time password for imported users
// without an importable password. It must be unpredictable: it is a real
// credential until the user changes it.
func temporaryPassword() (string, error) {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "emby-" + hex.EncodeToString(raw), nil
}

func verifyEmbySHA1(storedHash string, password string) bool {
	const prefix = "emby-sha1:"
	if !strings.HasPrefix(storedHash, prefix) {
		return false
	}
	sum := sha1.Sum([]byte(password))
	return strings.EqualFold(strings.TrimPrefix(storedHash, prefix), fmt.Sprintf("%x", sum))
}

func pathLabel(value string) string {
	value = filepath.Clean(value)
	if value == "." || value == string(filepath.Separator) {
		return "root"
	}
	return filepath.Base(value)
}

func folderLabel(folder domain.MediaFolder) string {
	if strings.TrimSpace(folder.Path) != "" {
		return pathLabel(folder.Path)
	}
	return pathLabel(folder.RelativePath)
}

// dedupeInts returns the input in input order with duplicates removed.
func dedupeInts(ids []int) []int {
	out := make([]int, 0, len(ids))
	seen := make(map[int]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// hiddenFoldersSQL lists folders hidden from their parent (show_as_child =
// false) with the library whose root contains them; plain SQL valid for both
// SQLite and Postgres.
const hiddenFoldersSQL = `SELECT h.id, h.path, lr.library_id, l.name, r.path
	FROM media_folders h
	JOIN library_roots lr ON 1 = 1
	JOIN media_folders r ON r.id = lr.folder_id
	JOIN libraries l ON l.id = lr.library_id
	WHERE NOT h.show_as_child AND (h.path = r.path OR h.path LIKE r.path || '/%')
	ORDER BY l.name, h.path`

func hiddenFolders(ctx context.Context, db *sql.DB) ([]domain.HiddenFolder, error) {
	rows, err := db.QueryContext(ctx, hiddenFoldersSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.HiddenFolder{}
	for rows.Next() {
		var item domain.HiddenFolder
		var folderPath, rootPath string
		if err := rows.Scan(&item.ID, &folderPath, &item.LibraryID, &item.LibraryName, &rootPath); err != nil {
			return nil, err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(folderPath, rootPath), "/")
		item.RelativePath = pathLabel(rootPath)
		if rel != "" {
			item.RelativePath += "/" + rel
		}
		item.Name = pathLabel(folderPath)
		out = append(out, item)
	}
	return out, rows.Err()
}

// hiddenFolderIDs is the set of folders hidden from their parent.
func hiddenFolderIDs(ctx context.Context, db *sql.DB) (map[int]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT id FROM media_folders WHERE NOT show_as_child`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]bool{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}
