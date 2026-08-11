package store

import (
	"crypto/sha1"
	"fmt"
	"path/filepath"
	"strings"

	"media-library/backend/internal/domain"
)

func temporaryPassword(parts ...string) string {
	sum := sha1.Sum([]byte("emby-import/" + strings.Join(parts, "/")))
	return "emby-" + fmt.Sprintf("%x", sum)[:18]
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
