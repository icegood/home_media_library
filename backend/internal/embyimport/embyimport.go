package embyimport

import (
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"

	"media-library/backend/internal/domain"
)

type PathMapping struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type Options struct {
	ConfigRoot   string        `json:"configRoot"`
	PathMappings []PathMapping `json:"pathMappings"`
}

func Read(ctx context.Context, options Options) (domain.ImportSnapshot, error) {
	root := strings.TrimSpace(options.ConfigRoot)
	if root == "" {
		return domain.ImportSnapshot{}, fmt.Errorf("configRoot is required")
	}
	paths := embyPaths(root)
	usersDB, err := sql.Open("sqlite", paths.usersDB+"?mode=ro&immutable=1")
	if err != nil {
		return domain.ImportSnapshot{}, err
	}
	defer usersDB.Close()
	libraryDB, err := sql.Open("sqlite", paths.libraryDB+"?mode=ro&immutable=1")
	if err != nil {
		return domain.ImportSnapshot{}, err
	}
	defer libraryDB.Close()
	users, err := readUsers(ctx, usersDB, paths.userConfigDir)
	if err != nil {
		return domain.ImportSnapshot{}, err
	}
	libraries, embyToLibraryID, err := readLibraries(ctx, libraryDB, options.PathMappings)
	if err != nil {
		return domain.ImportSnapshot{}, err
	}
	access, err := readAccess(ctx, libraryDB, embyToLibraryID)
	if err != nil {
		return domain.ImportSnapshot{}, err
	}
	return domain.ImportSnapshot{Users: users, Libraries: libraries, Access: access}, nil
}

type resolvedEmbyPaths struct {
	libraryDB     string
	usersDB       string
	userConfigDir string
}

func embyPaths(configRoot string) resolvedEmbyPaths {
	root := filepath.Clean(configRoot)
	return resolvedEmbyPaths{
		libraryDB:     filepath.Join(root, "data", "library.db"),
		usersDB:       filepath.Join(root, "data", "users.db"),
		userConfigDir: filepath.Join(root, "config", "users"),
	}
}

func readUsers(ctx context.Context, db *sql.DB, configDir string) ([]domain.User, error) {
	rows, err := db.QueryContext(ctx, `SELECT Id, json_extract(data,'$.Name'), json_extract(data,'$.IdString'), json_extract(data,'$.Policy.IsAdministrator'), json_extract(data,'$.Password') FROM LocalUsersv2 ORDER BY Id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []domain.User
	for rows.Next() {
		var id int
		var login string
		var idString sql.NullString
		var admin sql.NullBool
		var password sql.NullString
		if err := rows.Scan(&id, &login, &idString, &admin, &password); err != nil {
			return nil, err
		}
		role := domain.RoleRegular
		if admin.Valid && admin.Bool {
			role = domain.RoleAdmin
		}
		if role != domain.RoleAdmin && idString.Valid {
			if policy, err := readUserPolicy(configDir, idString.String); err != nil {
				return nil, err
			} else if policy.IsAdministrator {
				role = domain.RoleAdmin
			}
		}
		user := domain.User{ID: id, Login: login, Role: role}
		if password.Valid && validEmbySHA1(password.String) {
			user.PasswordHash = "emby-sha1:" + strings.ToLower(password.String)
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

type userPolicy struct {
	IsAdministrator bool `xml:"IsAdministrator"`
}

func readUserPolicy(configDir, idString string) (userPolicy, error) {
	if strings.TrimSpace(configDir) == "" || strings.TrimSpace(idString) == "" {
		return userPolicy{}, nil
	}
	path := filepath.Join(configDir, idString, "policy.xml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return userPolicy{}, nil
		}
		return userPolicy{}, fmt.Errorf("read Emby user policy %q: %w", path, err)
	}
	var policy userPolicy
	if err := xml.Unmarshal(data, &policy); err != nil {
		return userPolicy{}, fmt.Errorf("decode Emby user policy %q: %w", path, err)
	}
	return policy, nil
}

func validEmbySHA1(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, r := range value {
		if r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F' {
			continue
		}
		return false
	}
	return true
}

func readLibraries(ctx context.Context, db *sql.DB, mappings []PathMapping) ([]domain.Library, map[int]int, error) {
	rows, err := db.QueryContext(ctx, `
SELECT l.Id, l.Name, e.Value
FROM MediaItems l
LEFT JOIN ItemExtradata e ON e.ItemId=l.Id AND e.ExtradataTypeId=(
  SELECT ExtradataTypeId FROM ItemExtradataTypes WHERE Name='LibraryOptions'
)
WHERE l.Type=4 AND l.ParentId=2
ORDER BY l.Id`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	libraries := []domain.Library{}
	ids := map[int]int{}
	for rows.Next() {
		var embyID int
		var name string
		var options sql.NullString
		if err := rows.Scan(&embyID, &name, &options); err != nil {
			return nil, nil, err
		}
		id := embyID
		library := domain.Library{ID: id, Name: name}
		if options.Valid {
			paths, err := libraryOptionPaths(options.String)
			if err != nil {
				return nil, nil, fmt.Errorf("read library %q PathInfos: %w", name, err)
			}
			seen := map[string]bool{}
			for _, rootPath := range paths {
				rootPath = rewritePath(rootPath, mappings)
				if rootPath == "" || seen[rootPath] {
					continue
				}
				seen[rootPath] = true
				library.Roots = append(library.Roots, domain.LibraryRoot{Path: rootPath})
			}
		}
		libraries = append(libraries, library)
		ids[embyID] = id
	}
	return libraries, ids, rows.Err()
}

func libraryOptionPaths(value string) ([]string, error) {
	var options struct {
		PathInfos []struct {
			Path string `json:"Path"`
		} `json:"PathInfos"`
	}
	if err := json.Unmarshal([]byte(value), &options); err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(options.PathInfos))
	for _, info := range options.PathInfos {
		if strings.TrimSpace(info.Path) != "" {
			paths = append(paths, strings.TrimSpace(info.Path))
		}
	}
	return paths, nil
}

func readAccess(ctx context.Context, db *sql.DB, libraryIDs map[int]int) ([]domain.ImportAccess, error) {
	rows, err := db.QueryContext(ctx, `
SELECT UserId, ItemId
FROM UserItemShares
WHERE ShareLevel >= 100 AND ItemId IN (SELECT Id FROM MediaItems WHERE Type=4)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var access []domain.ImportAccess
	for rows.Next() {
		var embyUserID int
		var embyLibraryID int
		if err := rows.Scan(&embyUserID, &embyLibraryID); err != nil {
			return nil, err
		}
		libraryID, ok := libraryIDs[embyLibraryID]
		if !ok {
			continue
		}
		access = append(access, domain.ImportAccess{
			LibraryID: libraryID,
			UserID:    embyUserID,
		})
	}
	return access, rows.Err()
}

func rewritePath(value string, mappings []PathMapping) string {
	cleaned := filepath.Clean(value)
	best := PathMapping{}
	for _, mapping := range mappings {
		from := filepath.Clean(mapping.From)
		if from == "." || mapping.To == "" {
			continue
		}
		if cleaned == from || strings.HasPrefix(cleaned, from+string(filepath.Separator)) {
			if len(from) > len(best.From) {
				best = PathMapping{From: from, To: filepath.Clean(mapping.To)}
			}
		}
	}
	if best.From == "" {
		return cleaned
	}
	relative, err := filepath.Rel(best.From, cleaned)
	if err != nil || relative == "." {
		return best.To
	}
	return filepath.Join(best.To, relative)
}
