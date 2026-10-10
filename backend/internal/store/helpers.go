package store

import (
	"crypto/rand"
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
