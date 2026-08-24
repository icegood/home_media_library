package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"golang.org/x/crypto/bcrypt"

	"media-library/backend/internal/domain"
)

//go:embed migrations/sqlite/*.sql
var sqliteMigrations embed.FS

const mediaColumns = `m.id, m.folder_id, m.path, m.name, m.mime_type, m.size, m.metadata_json, m.gps, m.taken_at, m.metadata_error, m.thumbnail_error`

// favoriteExpr is a correlated EXISTS over the media alias m yielding whether
// the row belongs to any favorite view owned by a user id bound as a query
// parameter. It must be selected with a *bool scan destination.
const favoriteExpr = `EXISTS(SELECT 1 FROM favorite_view_items fvi
	JOIN favorite_views fv ON fv.id = fvi.favorite_view_id
	WHERE fv.user_id = ? AND fvi.media_id = m.id)`

// folderEntriesSQL returns child folders and media rows of a folder in a single
// result set, discriminated by a leading entry_kind column ('folder'/'media').
// Column order: entry_kind, id, parent/folder_id, path, name, mime_type, size,
// metadata_json, gps, taken_at, metadata_error, thumbnail_error, favorite.
// Query arguments: parentID, userID, parentID.
const folderEntriesSQL = `SELECT 'folder' AS entry_kind, f.id, f.parent_id, f.path, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, 0
	FROM media_folders f WHERE f.parent_id = ?
	UNION ALL
	SELECT 'media' AS entry_kind, ` + mediaColumns + `, ` + favoriteExpr + `
	FROM media m WHERE m.folder_id = ?`

// SQLite is a full database-backed implementation of Store using modernc.org/sqlite.
type SQLite struct {
	db *sql.DB
}

func NewSQLite(dsn string) (*SQLite, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("sqlite dsn is empty")
	}
	pathOnly := strings.SplitN(dsn, "?", 2)[0]
	if dir := filepath.Dir(pathOnly); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create sqlite directory: %w", err)
		}
	}
	if !strings.Contains(dsn, "?") {
		dsn += "?"
	} else if !strings.HasSuffix(dsn, "?") && !strings.HasSuffix(dsn, "&") {
		dsn += "&"
	}
	dsn += "_pragma=foreign_keys(1)&_pragma=busy_timeout(10000)"
	db, err := openLogged("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// Single connection: SQLite writes serialize on one writer and this keeps
	// transaction semantics predictable for the small self-hosted workload.
	db.SetMaxOpenConns(1)
	store := &SQLite{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLite) Close() error {
	return s.db.Close()
}

// Vacuum compacts the SQLite database file. It is a no-op on a closed or
// transaction-bound connection; callers must not be inside a transaction.
func (s *SQLite) Vacuum(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `VACUUM`)
	return err
}

func (s *SQLite) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return err
	}
	entries, err := fs.Glob(sqliteMigrations, "migrations/sqlite/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(entries)
	for _, name := range entries {
		version := filepath.Base(name)
		var applied string
		err := s.db.QueryRow(`SELECT version FROM schema_migrations WHERE version = ?`, version).Scan(&applied)
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		data, err := sqliteMigrations.ReadFile(name)
		if err != nil {
			return err
		}
		if _, err := s.db.Exec(string(data)); err != nil {
			return fmt.Errorf("apply migration %s: %w", version, err)
		}
		if _, err := s.db.Exec(`INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)`,
			version, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return err
		}
	}
	return nil
}

func translateErr(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func normalizePath(value string) string {
	return filepath.ToSlash(filepath.Clean(value))
}

// gpsCoords splits a "lat,lng" value into its coordinates. Values that are not
// two finite numbers produce (nil, nil) so absent GPS is stored as NULL rather
// than a fake 0,0 point; the media_geo R*Tree only ever holds real coordinates.
func gpsCoords(value string) (any, any) {
	if canonical, ok := domain.CanonicalGPS(value); ok {
		parts := strings.Split(canonical, ",")
		lat, _ := strconv.ParseFloat(parts[0], 64)
		lng, _ := strconv.ParseFloat(parts[1], 64)
		return lat, lng
	}
	return nil, nil
}

// parentIDOrNull converts only the sentinel InvalidID to SQL NULL. Postgres
// identity sequences start at 0, so 0 is a legitimate folder id and must pass
// through untouched; SQLite rowids start at 1, so the conversion is a no-op there.
func parentIDOrNull(id int) any {
	if id == domain.InvalidID {
		return nil
	}
	return id
}

// scanMedia scans one media row. extras are additional trailing scan
// destinations (e.g. *int or *string) for computed columns such as the
// library id, the on-the-fly relative path, or the per-user favorite flag.
func scanMedia(sc interface{ Scan(...any) error }, extras ...any) (domain.Media, error) {
	var item domain.Media
	var meta, mime string
	args := []any{&item.ID, &item.FolderID, &item.Path, &item.Name, &mime, &item.Size,
		&meta, &item.GPS, &item.TakenAt, &item.MetadataError, &item.ThumbnailError}
	args = append(args, extras...)
	if err := sc.Scan(args...); err != nil {
		return item, err
	}
	item.Kind = domain.KindFromMIME(mime)
	item.MIMEType = mime
	_ = json.Unmarshal([]byte(meta), &item.Metadata)
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	return item, nil
}

// scanEntryRows scans rows of the merged folderEntriesSQL / folderEntriesPGSQL
// query and builds folder and media entries. folderRel and mediaRel compute the
// relative path for folder and media rows respectively.
func scanEntryRows(rows *sql.Rows, folderRel func(domain.MediaFolder) string, mediaRel func(domain.Media) string) ([]domain.Entry, error) {
	out := []domain.Entry{}
	for rows.Next() {
		var kind string
		var id int
		var secondID sql.NullInt64
		var path, name, mime, metadataJSON, gps, takenAt, metadataError, thumbnailError sql.NullString
		var size sql.NullInt64
		var favorite bool
		if err := rows.Scan(&kind, &id, &secondID, &path, &name, &mime, &size, &metadataJSON, &gps, &takenAt, &metadataError, &thumbnailError, &favorite); err != nil {
			rows.Close()
			return nil, err
		}
		if kind == "folder" {
			folder := domain.MediaFolder{ID: id, Path: path.String}
			if secondID.Valid {
				folder.ParentID = int(secondID.Int64)
			} else {
				folder.ParentID = domain.InvalidID
			}
			copyFolder := folder
			copyFolder.RelativePath = folderRel(folder)
			out = append(out, domain.Entry{ID: folder.ID, Name: folderLabel(folder),
				RelativePath: copyFolder.RelativePath, Type: "folder", Folder: &copyFolder,
				FolderThumbnail: folder.ID})
		} else {
			item := domain.Media{ID: id, FolderID: int(secondID.Int64), Path: path.String, Name: name.String,
				Kind: domain.KindFromMIME(mime.String), MIMEType: mime.String, Size: size.Int64,
				GPS: gps.String, TakenAt: takenAt.String, MetadataError: metadataError.String,
				ThumbnailError: thumbnailError.String, Favorite: favorite}
			_ = json.Unmarshal([]byte(metadataJSON.String), &item.Metadata)
			if item.Metadata == nil {
				item.Metadata = map[string]any{}
			}
			copy := item
			copy.RelativePath = mediaRel(item)
			out = append(out, domain.Entry{ID: item.ID, Name: item.Name,
				RelativePath: copy.RelativePath, Type: "media", Media: &copy})
		}
	}
	rows.Close()
	return out, nil
}

// relativePathExpr returns the on-the-fly relative path of a folder/media path
// beneath a library root: the absolute path with the root prefix stripped.
func relativePathExpr(pathExpr, rootPathExpr string) string {
	return `substr(` + pathExpr + `, length(` + rootPathExpr + `) + 2)`
}

// rootPathForFolder returns the path of the nearest library-root ancestor of a
// folder, or "" when the folder is not beneath any library root. Stored folder
// paths are canonical absolute paths built from the resolved root, so the
// nearest root is simply the library root whose path is the longest prefix of
// the folder's own path; no ancestor walk is needed.
func (s *SQLite) rootPathForFolder(ctx context.Context, folderID int) string {
	query := `SELECT root.path
		FROM library_roots lr
		JOIN media_folders root ON root.id = lr.folder_id
		JOIN media_folders folder ON folder.id = ?
		WHERE root.path = folder.path
		   OR substr(folder.path, 1, length(root.path) + 1) = root.path || '/'
		ORDER BY length(root.path) DESC LIMIT 1`
	var root string
	if err := s.db.QueryRowContext(ctx, query, folderID).Scan(&root); err != nil {
		return ""
	}
	return root
}

func pathBelowRoot(root, value string) string {
	return strings.TrimPrefix(strings.TrimPrefix(value, root), "/")
}

// nestedPath reports whether child is a subfolder of parent. Paths are
// canonical absolute paths with forward slashes, so the boundary check is a
// simple prefix test.
func nestedPath(child, parent string) bool {
	return strings.HasPrefix(child, parent+"/")
}

// attachRelativePaths fills in the computed relative path for each media item,
// memoizing the nearest root lookup per folder.
func (s *SQLite) attachRelativePaths(ctx context.Context, items []domain.Media) {
	roots := map[int]string{}
	for index := range items {
		root, ok := roots[items[index].FolderID]
		if !ok {
			root = s.rootPathForFolder(ctx, items[index].FolderID)
			roots[items[index].FolderID] = root
		}
		if root != "" {
			items[index].RelativePath = strings.TrimPrefix(strings.TrimPrefix(items[index].Path, root), "/")
		}
	}
}

// accessibleSubtree returns the seed SELECT for a recursive CTE of every folder
// beneath the roots a user may read (all roots for admins). Callers wrap it in
// `WITH RECURSIVE sub(id) AS (` ... ` UNION ALL ...)`.
func accessibleSubtree(userID int, admin bool) (string, []any) {
	if admin {
		return `SELECT lr.folder_id FROM library_roots lr`, nil
	}
	return `SELECT lr.folder_id FROM library_access la JOIN library_roots lr ON lr.library_id = la.library_id WHERE la.user_id = ?`,
		[]any{userID}
}

func (s *SQLite) SetupRequired(ctx context.Context) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return false, err
	}
	return count == 0, nil
}

func (s *SQLite) CreateInitialAdmin(ctx context.Context, user domain.User, password string) (domain.User, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return domain.User{}, err
	}
	if count != 0 {
		return domain.User{}, ErrConflict
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return domain.User{}, err
	}
	user.Login = strings.ToLower(strings.TrimSpace(user.Login))
	user.Role = domain.RoleAdmin
	user.PasswordHash = string(hash)
	res, err := s.db.ExecContext(ctx, `INSERT INTO users(login, password_hash, role) VALUES(?,?,?)`,
		user.Login, user.PasswordHash, user.Role)
	if err != nil {
		return domain.User{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.User{}, err
	}
	user.ID = int(id)
	return user, nil
}

func (s *SQLite) ServerSettings(ctx context.Context) (domain.ServerSettings, error) {
	settings := domain.DefaultServerSettings()
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value_json FROM server_settings WHERE id = 0`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return settings, nil
	}
	if err != nil {
		return settings, err
	}
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &settings); err != nil {
			return settings, err
		}
	}
	return settings, nil
}

func (s *SQLite) SaveServerSettings(ctx context.Context, settings domain.ServerSettings) error {
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO server_settings(id, value_json) VALUES(0,?)
		ON CONFLICT(id) DO UPDATE SET value_json = excluded.value_json`, string(data))
	return err
}

func (s *SQLite) MIMETypeForExtension(ctx context.Context, extension string) (string, error) {
	var mimeType string
	err := s.db.QueryRowContext(ctx, `SELECT mime_type FROM media_mime_extensions WHERE extension = ?`, strings.ToLower(strings.TrimSpace(extension))).Scan(&mimeType)
	if err != nil {
		return "", translateErr(err)
	}
	return mimeType, nil
}

func (s *SQLite) UserSettings(ctx context.Context, userID int) (domain.UserSettings, error) {
	settings := domain.DefaultUserSettings()
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value_json FROM user_settings WHERE user_id = ?`, userID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return settings, nil
	}
	if err != nil {
		return settings, err
	}
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &settings); err != nil {
			return settings, err
		}
	}
	return settings, nil
}

func (s *SQLite) SaveUserSettings(ctx context.Context, userID int, settings domain.UserSettings) error {
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO user_settings(user_id, value_json) VALUES(?,?)
		ON CONFLICT(user_id) DO UPDATE SET value_json = excluded.value_json`, userID, string(data))
	return err
}

func (s *SQLite) Authenticate(ctx context.Context, login, password string) (domain.User, error) {
	login = strings.ToLower(strings.TrimSpace(login))
	var user domain.User
	err := s.db.QueryRowContext(ctx, `SELECT id, login, password_hash, role, COALESCE(email, '') FROM users WHERE login = ?`, login).
		Scan(&user.ID, &user.Login, &user.PasswordHash, &user.Role, &user.Email)
	if err != nil {
		return user, translateErr(err)
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) == nil {
		return user, nil
	}
	if verifyEmbySHA1(user.PasswordHash, password) {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return user, err
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, string(hash), user.ID); err != nil {
			return user, err
		}
		user.PasswordHash = string(hash)
		return user, nil
	}
	return domain.User{}, ErrNotFound
}

func (s *SQLite) User(ctx context.Context, id int) (domain.User, error) {
	var user domain.User
	err := s.db.QueryRowContext(ctx, `SELECT id, login, password_hash, role, COALESCE(email, '') FROM users WHERE id = ?`, id).
		Scan(&user.ID, &user.Login, &user.PasswordHash, &user.Role, &user.Email)
	if err != nil {
		return user, translateErr(err)
	}
	return user, nil
}

func normalizeUserForSave(user domain.User) (domain.User, error) {
	user.Login = strings.ToLower(strings.TrimSpace(user.Login))
	if user.Login == "" {
		return user, ErrNotFound
	}
	if user.Role != domain.RoleAdmin && user.Role != domain.RoleRegular {
		return user, ErrNotFound
	}
	return user, nil
}

func (s *SQLite) Users(ctx context.Context) ([]domain.User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, login, password_hash, role, COALESCE(email, '') FROM users ORDER BY role, login`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []domain.User
	for rows.Next() {
		var user domain.User
		if err := rows.Scan(&user.ID, &user.Login, &user.PasswordHash, &user.Role, &user.Email); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *SQLite) CreateUser(ctx context.Context, user domain.User, password string) (domain.User, error) {
	var err error
	user, err = normalizeUserForSave(user)
	if err != nil {
		return domain.User{}, err
	}
	if len(password) < 12 {
		return domain.User{}, ErrNotFound
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return domain.User{}, err
	}
	user.PasswordHash = string(hash)
	res, err := s.db.ExecContext(ctx, `INSERT INTO users(login, password_hash, role) VALUES(?,?,?)`, user.Login, user.PasswordHash, user.Role)
	if err != nil {
		return domain.User{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.User{}, err
	}
	user.ID = int(id)
	return user, nil
}

func (s *SQLite) UpdateUser(ctx context.Context, user domain.User, password string) (domain.User, error) {
	var err error
	user, err = normalizeUserForSave(user)
	if err != nil {
		return domain.User{}, err
	}
	var res sql.Result
	if strings.TrimSpace(password) != "" {
		if len(password) < 12 {
			return domain.User{}, ErrNotFound
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return domain.User{}, err
		}
		res, err = s.db.ExecContext(ctx, `UPDATE users SET login = ?, role = ?, password_hash = ? WHERE id = ?`, user.Login, user.Role, string(hash), user.ID)
	} else {
		res, err = s.db.ExecContext(ctx, `UPDATE users SET login = ?, role = ? WHERE id = ?`, user.Login, user.Role, user.ID)
	}
	if err != nil {
		return domain.User{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return domain.User{}, err
	}
	if affected == 0 {
		return domain.User{}, ErrNotFound
	}
	return s.User(ctx, user.ID)
}

func (s *SQLite) SetUserEmail(ctx context.Context, userID int, email string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE users SET email = ? WHERE id = ?`, email, userID)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	}
	return rowsAffectedErr(res)
}

func (s *SQLite) UserByEmail(ctx context.Context, email string) (domain.User, error) {
	var user domain.User
	err := s.db.QueryRowContext(ctx, `SELECT id, login, password_hash, role, COALESCE(email, '') FROM users WHERE email = ? COLLATE NOCASE`, email).
		Scan(&user.ID, &user.Login, &user.PasswordHash, &user.Role, &user.Email)
	if err != nil {
		return user, translateErr(err)
	}
	return user, nil
}

func (s *SQLite) UpdatePassword(ctx context.Context, userID int, password string) error {
	if len(password) < 12 {
		return ErrNotFound
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, string(hash), userID)
	if err != nil {
		return err
	}
	return rowsAffectedErr(res)
}

func (s *SQLite) CreatePasswordResetToken(ctx context.Context, userID int, tokenHash string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO password_reset_tokens(token_hash, user_id, created_at, expires_at) VALUES(?,?,?,?)`,
		tokenHash, userID, time.Now().UTC().Format(time.RFC3339), expiresAt.UTC().Format(time.RFC3339))
	return err
}

func (s *SQLite) ConsumePasswordResetToken(ctx context.Context, tokenHash string) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var userID int
	var expiresAt string
	err = tx.QueryRowContext(ctx, `SELECT user_id, expires_at FROM password_reset_tokens WHERE token_hash = ?`, tokenHash).
		Scan(&userID, &expiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	if expires, parseErr := time.Parse(time.RFC3339, expiresAt); parseErr != nil || time.Now().After(expires) {
		_, _ = tx.ExecContext(ctx, `DELETE FROM password_reset_tokens WHERE token_hash = ?`, tokenHash)
		_ = tx.Commit()
		return 0, ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM password_reset_tokens WHERE token_hash = ?`, tokenHash); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return userID, nil
}

func rowsAffectedErr(res sql.Result) error {
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func isUniqueViolation(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint failed") ||
		strings.Contains(message, "constraint failed: unique") ||
		strings.Contains(message, "duplicate key value violates unique constraint") ||
		strings.Contains(message, "sqlstate 23505")
}

func (s *SQLite) ImportSnapshot(ctx context.Context, snapshot domain.ImportSnapshot) (domain.ImportResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.ImportResult{}, err
	}
	defer tx.Rollback()
	result := domain.ImportResult{}
	userIDs := map[int]int{}
	for _, user := range snapshot.Users {
		user.Login = strings.ToLower(strings.TrimSpace(user.Login))
		if user.Login == "" {
			continue
		}
		originalID := user.ID
		id := user.ID
		if id == domain.InvalidID {
			id = 0
		}
		if id != 0 {
			var existing domain.User
			err := tx.QueryRowContext(ctx, `SELECT id, login, password_hash, role FROM users WHERE id = ?`, id).
				Scan(&existing.ID, &existing.Login, &existing.PasswordHash, &existing.Role)
			if err == nil {
				userIDs[originalID] = existing.ID
				result.Users = append(result.Users, domain.ImportedUser{User: existing, Existed: true})
				continue
			}
		}
		if user.Role == "" {
			user.Role = domain.RoleRegular
		}
		imported := domain.ImportedUser{User: user}
		if id == 0 {
			res, err := tx.ExecContext(ctx, `INSERT INTO users(login, password_hash, role) VALUES(?,?,?)`,
				user.Login, user.PasswordHash, user.Role)
			if err != nil {
				continue
			}
			newID, _ := res.LastInsertId()
			id = int(newID)
			if user.PasswordHash == "" {
				password := temporaryPassword(strconv.Itoa(id), user.Login)
				hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
				if err != nil {
					return result, err
				}
				if _, err := tx.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, string(hash), id); err != nil {
					return result, err
				}
				imported.TemporaryPassword = password
			}
		} else {
			if user.PasswordHash == "" {
				password := temporaryPassword(strconv.Itoa(id), user.Login)
				hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
				if err != nil {
					return result, err
				}
				user.PasswordHash = string(hash)
				imported.TemporaryPassword = password
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO users(id, login, password_hash, role) VALUES(?,?,?,?)`,
				id, user.Login, user.PasswordHash, user.Role); err != nil {
				continue
			}
		}
		user.ID = id
		user.PasswordHash = ""
		userIDs[originalID] = id
		imported.User = user
		result.Users = append(result.Users, imported)
	}
	libraryIDs := map[int]int{}
	for _, library := range snapshot.Libraries {
		name := strings.TrimSpace(library.Name)
		if name == "" {
			continue
		}
		originalID := library.ID
		id := library.ID
		roots := []domain.LibraryRoot{}
		for _, root := range library.Roots {
			if strings.TrimSpace(root.Path) == "" {
				continue
			}
			folderID, err := s.ensureFolderByPathTx(ctx, tx, normalizePath(root.Path))
			if err != nil {
				return result, err
			}
			roots = append(roots, domain.LibraryRoot{ID: folderID, Path: root.Path})
		}
		if id == domain.InvalidID {
			res, err := tx.ExecContext(ctx, `INSERT INTO libraries(name) VALUES(?)`, name)
			if err != nil {
				return result, err
			}
			newID, _ := res.LastInsertId()
			id = int(newID)
		} else if _, err := tx.ExecContext(ctx, `INSERT INTO libraries(id, name) VALUES(?,?)`, id, name); err != nil {
			continue
		}
		for _, root := range roots {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO library_roots(library_id, folder_id) VALUES(?,?)`,
				id, root.ID); err != nil {
				return result, err
			}
		}
		library.ID = id
		library.Name = name
		library.Roots = roots
		libraryIDs[originalID] = id
		result.Libraries = append(result.Libraries, library)
	}
	for _, link := range snapshot.Access {
		userID, ok := userIDs[link.UserID]
		if !ok {
			continue
		}
		libID, ok := libraryIDs[link.LibraryID]
		if !ok {
			continue
		}
		var role string
		if err := tx.QueryRowContext(ctx, `SELECT role FROM users WHERE id = ?`, userID).Scan(&role); err != nil {
			continue
		}
		if role == string(domain.RoleAdmin) {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO library_access(library_id, user_id) VALUES(?,?)`,
			libID, userID); err != nil {
			return result, err
		}
		result.Access = append(result.Access, domain.ImportAccess{LibraryID: libID, UserID: userID})
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func (s *SQLite) loadRoots(ctx context.Context, libraryID int) []domain.LibraryRoot {
	rows, err := s.db.QueryContext(ctx, `SELECT lr.folder_id, f.path, COALESCE(lr.watch, 0) FROM library_roots lr
		JOIN media_folders f ON f.id = lr.folder_id
		WHERE lr.library_id = ? ORDER BY f.path`, libraryID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	roots := []domain.LibraryRoot{}
	for rows.Next() {
		var root domain.LibraryRoot
		var watch bool
		if err := rows.Scan(&root.ID, &root.Path, &watch); err != nil {
			continue
		}
		root.Watch = watch
		roots = append(roots, root)
	}
	return roots
}

// WatchedRoots returns every library root flagged for filesystem watching.
func (s *SQLite) WatchedRoots(ctx context.Context) ([]domain.WatchedRoot, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT lr.library_id, f.path FROM library_roots lr
		JOIN media_folders f ON f.id = lr.folder_id
		WHERE COALESCE(lr.watch, 0) = 1 ORDER BY lr.library_id, f.path`)
	if err != nil {
		return nil, translateErr(err)
	}
	defer rows.Close()
	out := []domain.WatchedRoot{}
	for rows.Next() {
		var root domain.WatchedRoot
		if err := rows.Scan(&root.LibraryID, &root.Path); err != nil {
			continue
		}
		out = append(out, root)
	}
	return out, rows.Err()
}

func (s *SQLite) LibraryStats(ctx context.Context, libraryID int) (domain.KindStats, error) {
	var stats domain.KindStats
	query := `WITH RECURSIVE sub(id) AS (
		SELECT lr.folder_id FROM library_roots lr WHERE lr.library_id = ?
		UNION ALL
		SELECT f.id FROM media_folders f JOIN sub ON f.parent_id = sub.id
	)
	SELECT (SELECT COUNT(*) FROM media m JOIN media_mime_types mmt ON mmt.value = m.mime_type JOIN sub ON m.folder_id = sub.id WHERE mmt.media_type = 'image'),
		(SELECT COUNT(*) FROM media m JOIN media_mime_types mmt ON mmt.value = m.mime_type JOIN sub ON m.folder_id = sub.id WHERE mmt.media_type = 'video')`
	err := s.db.QueryRowContext(ctx, query, libraryID).
		Scan(&stats.Images, &stats.Videos)
	if err != nil {
		return domain.KindStats{}, translateErr(err)
	}
	return stats, nil
}

// FolderStats aggregates media kinds over the folder and every subfolder in
// one recursive query. FolderChain validates existence and read access.
func (s *SQLite) FolderStats(ctx context.Context, userID, libraryID, folderID int) (domain.KindStats, error) {
	if _, err := s.FolderChain(ctx, libraryID, folderID); err != nil {
		return domain.KindStats{}, err
	}
	var stats domain.KindStats
	query := `WITH RECURSIVE sub(id) AS (
		SELECT f.id FROM media_folders f WHERE f.id = ?
		UNION ALL
		SELECT f.id FROM media_folders f JOIN sub ON f.parent_id = sub.id
	)
	SELECT COALESCE(SUM(CASE WHEN mmt.media_type = 'image' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN mmt.media_type = 'video' THEN 1 ELSE 0 END), 0)
	FROM media m JOIN sub ON m.folder_id = sub.id
	JOIN media_mime_types mmt ON mmt.value = m.mime_type`
	if err := s.db.QueryRowContext(ctx, query, folderID).Scan(&stats.Images, &stats.Videos); err != nil {
		return domain.KindStats{}, translateErr(err)
	}
	return stats, nil
}

// FavoriteViewStats aggregates media kinds over direct mentions plus the full
// contents of favorite folders, mirroring FavoriteMediaExpanded's scope.
func (s *SQLite) FavoriteViewStats(ctx context.Context, userID, viewID int, admin bool) (domain.KindStats, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM favorite_views WHERE id = ? AND user_id = ?`, viewID, userID).Scan(&exists); err != nil {
		return domain.KindStats{}, translateErr(err)
	}
	var stats domain.KindStats
	sub, subArgs := accessibleSubtree(userID, admin)
	query := `WITH RECURSIVE sub(id) AS (` + sub + `
		UNION ALL
		SELECT f.id FROM media_folders f JOIN sub ON f.parent_id = sub.id),
		fav_folders(id) AS (SELECT ff.folder_id FROM favorite_folders ff WHERE ff.favorite_view_id = ?),
		folder_sub(id) AS (SELECT id FROM fav_folders UNION ALL SELECT f.id FROM media_folders f JOIN folder_sub ON f.parent_id = folder_sub.id),
		mentions(media_id) AS (
			SELECT fvi.media_id FROM favorite_view_items fvi WHERE fvi.favorite_view_id = ?
			UNION ALL
			SELECT m2.id FROM media m2 JOIN folder_sub fs ON m2.folder_id = fs.id
		)
	SELECT COALESCE(SUM(CASE WHEN mmt.media_type = 'image' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN mmt.media_type = 'video' THEN 1 ELSE 0 END), 0)
	FROM mentions JOIN media m ON m.id = mentions.media_id
	JOIN media_mime_types mmt ON mmt.value = m.mime_type
	WHERE (? = 1 OR m.folder_id IN (SELECT id FROM sub))`
	args := append([]any{}, subArgs...)
	if admin {
		args = append(args, viewID, viewID, 1)
	} else {
		args = append(args, viewID, viewID, 0)
	}
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&stats.Images, &stats.Videos); err != nil {
		return domain.KindStats{}, translateErr(err)
	}
	return stats, nil
}

func (s *SQLite) loadLibrary(ctx context.Context, id int) (domain.Library, error) {
	var library domain.Library
	err := s.db.QueryRowContext(ctx, `SELECT id, name FROM libraries WHERE id = ?`, id).
		Scan(&library.ID, &library.Name)
	if err != nil {
		return library, translateErr(err)
	}
	library.Roots = s.loadRoots(ctx, id)
	return library, nil
}

func (s *SQLite) LibrariesForUser(ctx context.Context, userID int, admin bool) ([]domain.Library, error) {
	var rows *sql.Rows
	var err error
	if admin {
		rows, err = s.db.QueryContext(ctx, `SELECT l.id, l.name, lr.folder_id, f.path, COALESCE(lr.watch, 0)
			FROM libraries l
			LEFT JOIN library_roots lr ON lr.library_id = l.id
			LEFT JOIN media_folders f ON f.id = lr.folder_id
			ORDER BY l.name, f.path`)
	} else {
		rows, err = s.db.QueryContext(ctx, `SELECT l.id, l.name, lr.folder_id, f.path, COALESCE(lr.watch, 0)
			FROM libraries l
			JOIN library_access la ON la.library_id = l.id AND la.user_id = ?
			LEFT JOIN library_roots lr ON lr.library_id = l.id
			LEFT JOIN media_folders f ON f.id = lr.folder_id
			ORDER BY l.name, f.path`, userID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	libraries := map[int]*domain.Library{}
	var order []int
	for rows.Next() {
		var id int
		var name string
		var rootID sql.NullInt64
		var rootPath sql.NullString
		var rootWatch sql.NullBool
		if err := rows.Scan(&id, &name, &rootID, &rootPath, &rootWatch); err != nil {
			return nil, err
		}
		library, ok := libraries[id]
		if !ok {
			library = &domain.Library{ID: id, Name: name}
			libraries[id] = library
			order = append(order, id)
		}
		if rootID.Valid && rootPath.Valid {
			path := rootPath.String
			if !admin {
				path = ""
			}
			library.Roots = append(library.Roots, domain.LibraryRoot{ID: int(rootID.Int64), Path: path, Watch: rootWatch.Bool})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.fillLibraryStats(ctx, libraries, admin, userID); err != nil {
		return nil, err
	}
	out := make([]domain.Library, 0, len(order))
	for _, id := range order {
		out = append(out, *libraries[id])
	}
	return out, nil
}

// fillLibraryStats computes images/videos for every listed library in one
// grouped recursive pass instead of one query per library.
func (s *SQLite) fillLibraryStats(ctx context.Context, libraries map[int]*domain.Library, admin bool, userID int) error {
	if len(libraries) == 0 {
		return nil
	}
	accessFilter := ""
	args := []any{}
	if !admin {
		accessFilter = `JOIN library_access la ON la.library_id = lr.library_id AND la.user_id = ?`
		args = append(args, userID)
	}
	query := `WITH RECURSIVE tree(library_id, folder_id) AS (
		SELECT lr.library_id, lr.folder_id FROM library_roots lr ` + accessFilter + `
		UNION ALL
		SELECT tree.library_id, f.id FROM media_folders f JOIN tree ON f.parent_id = tree.folder_id
	)
	SELECT tree.library_id,
		COALESCE(SUM(CASE WHEN mmt.media_type = 'image' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN mmt.media_type = 'video' THEN 1 ELSE 0 END), 0)
	FROM tree JOIN media m ON m.folder_id = tree.folder_id
	JOIN media_mime_types mmt ON mmt.value = m.mime_type
	GROUP BY tree.library_id`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return translateErr(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var stats domain.KindStats
		if err := rows.Scan(&id, &stats.Images, &stats.Videos); err != nil {
			return translateErr(err)
		}
		if library, ok := libraries[id]; ok {
			library.Stats = stats
		}
	}
	return rows.Err()
}

func (s *SQLite) Library(ctx context.Context, id int) (domain.Library, error) {
	return s.loadLibrary(ctx, id)
}

func (s *SQLite) Folder(ctx context.Context, id int) (domain.MediaFolder, error) {
	var folder domain.MediaFolder
	var parentID sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT id, parent_id, path FROM media_folders WHERE id = ?`, id).
		Scan(&folder.ID, &parentID, &folder.Path)
	if err != nil {
		return folder, translateErr(err)
	}
	if parentID.Valid {
		folder.ParentID = int(parentID.Int64)
	} else {
		folder.ParentID = domain.InvalidID
	}
	folder.Name = folderLabel(folder)
	return folder, nil
}

// FolderChain returns the ancestor chain of folderID within libraryID, ordered
// from the library root down to the folder itself. The recursive walk climbs
// to the top of the folder tree; the chain is then cut at the nearest ancestor
// that is a root of this library (a library root may itself be nested beneath
// another library's root). ErrNotFound is returned when the folder is not
// beneath any root of this library.
func (s *SQLite) FolderChain(ctx context.Context, libraryID, folderID int) ([]domain.MediaFolder, error) {
	query := `WITH RECURSIVE chain(id, parent_id, path, depth) AS (
			SELECT id, parent_id, path, 0 FROM media_folders WHERE id = ?
			UNION ALL
			SELECT f.id, f.parent_id, f.path, c.depth + 1
			FROM media_folders f JOIN chain c ON f.id = c.parent_id)
		SELECT c.id, c.parent_id, c.path
		FROM chain c
		WHERE c.depth <= (SELECT MIN(depth) FROM chain
			WHERE id IN (SELECT folder_id FROM library_roots WHERE library_id = ?))
		ORDER BY c.depth DESC`
	rows, err := s.db.QueryContext(ctx, query, folderID, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.MediaFolder{}
	for rows.Next() {
		var folder domain.MediaFolder
		var parentID sql.NullInt64
		if err := rows.Scan(&folder.ID, &parentID, &folder.Path); err != nil {
			return nil, err
		}
		if parentID.Valid {
			folder.ParentID = int(parentID.Int64)
		} else {
			folder.ParentID = domain.InvalidID
		}
		out = append(out, folder)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, ErrNotFound
	}
	root := out[0].Path
	for index := range out {
		out[index].Name = folderLabel(out[index])
		out[index].RelativePath = pathBelowRoot(root, out[index].Path)
	}
	return out, nil
}

func (s *SQLite) FoldersByIDs(ctx context.Context, ids []int) (map[int]domain.MediaFolder, error) {
	out := map[int]domain.MediaFolder{}
	if len(ids) == 0 {
		return out, nil
	}
	unique := dedupeInts(ids)
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(unique)), ",")
	args := make([]any, len(unique))
	for index, id := range unique {
		args[index] = id
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, parent_id, path FROM media_folders WHERE id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var folder domain.MediaFolder
		var parentID sql.NullInt64
		if err := rows.Scan(&folder.ID, &parentID, &folder.Path); err != nil {
			return nil, err
		}
		if parentID.Valid {
			folder.ParentID = int(parentID.Int64)
		} else {
			folder.ParentID = domain.InvalidID
		}
		folder.Name = folderLabel(folder)
		out[folder.ID] = folder
	}
	return out, rows.Err()
}

func (s *SQLite) CanRead(ctx context.Context, userID, libraryID int, admin bool) (bool, error) {
	if admin {
		return true, nil
	}
	var ok bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM library_access WHERE library_id = ? AND user_id = ?)`,
		libraryID, userID).Scan(&ok)
	return ok, err
}

func (s *SQLite) CanReadMedia(ctx context.Context, userID, mediaID int, admin bool) (bool, error) {
	var folderID int
	if err := s.db.QueryRowContext(ctx, `SELECT folder_id FROM media WHERE id = ?`, mediaID).Scan(&folderID); err != nil {
		return false, translateErr(err)
	}
	if admin {
		return true, nil
	}
	sub, args := accessibleSubtree(userID, false)
	query := `WITH RECURSIVE sub(id) AS (` + sub + `
		UNION ALL
		SELECT f.id FROM media_folders f JOIN sub ON f.parent_id = sub.id)
	SELECT EXISTS(SELECT 1 FROM sub WHERE id = ?)`
	args = append(args, folderID)
	var ok bool
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&ok)
	return ok, err
}

func (s *SQLite) Entries(ctx context.Context, userID, libraryID int, dir string) ([]domain.Entry, error) {
	dir = strings.Trim(path.Clean("/"+dir), "/")
	library, err := s.loadLibrary(ctx, libraryID)
	if err != nil {
		return nil, err
	}
	out := []domain.Entry{}
	showRootWrappers := true
	for _, mapping := range library.Roots {
		mappingName := pathLabel(mapping.Path)
		if showRootWrappers && dir == "" {
			out = append(out, domain.Entry{ID: mapping.ID, Name: mappingName,
				RelativePath: mappingName, Type: "folder", FolderThumbnail: mapping.ID})
			continue
		}
		folderDir := dir
		if showRootWrappers {
			if dir != mappingName && !strings.HasPrefix(dir, mappingName+"/") {
				continue
			}
			folderDir = strings.TrimPrefix(strings.TrimPrefix(dir, mappingName), "/")
		} else if dir == mappingName || strings.HasPrefix(dir, mappingName+"/") {
			folderDir = strings.TrimPrefix(strings.TrimPrefix(dir, mappingName), "/")
		}
		parentID := mapping.ID
		if folderDir != "" {
			parentID = s.descendantFolderID(ctx, mapping.ID, folderDir)
			if parentID == domain.InvalidID {
				continue
			}
		}
		rows, err := s.db.QueryContext(ctx, folderEntriesSQL, parentID, userID, parentID)
		if err != nil {
			return nil, err
		}
		folderEntries, err := scanEntryRows(rows,
			func(f domain.MediaFolder) string { return path.Join(dir, folderLabel(f)) },
			func(m domain.Media) string { return path.Join(dir, m.Name) })
		if err != nil {
			return nil, err
		}
		out = append(out, folderEntries...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type == "folder"
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (s *SQLite) EntriesForFolder(ctx context.Context, userID, libraryID, folderID int) (domain.FolderEntries, error) {
	chain, err := s.FolderChain(ctx, libraryID, folderID)
	if err != nil {
		return domain.FolderEntries{}, err
	}
	entries, err := s.entriesForParent(ctx, userID, folderID, chain[0].Path)
	if err != nil {
		return domain.FolderEntries{}, err
	}
	return domain.FolderEntries{Entries: entries, Chain: chain}, nil
}

func (s *SQLite) entriesForParent(ctx context.Context, userID, parentID int, root string) ([]domain.Entry, error) {
	rows, err := s.db.QueryContext(ctx, folderEntriesSQL, parentID, userID, parentID)
	if err != nil {
		return nil, err
	}
	out, err := scanEntryRows(rows,
		func(f domain.MediaFolder) string { return pathBelowRoot(root, f.Path) },
		func(m domain.Media) string { return pathBelowRoot(root, m.Path) })
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type == "folder"
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (s *SQLite) descendantFolderID(ctx context.Context, rootID int, relativePath string) int {
	current := rootID
	for _, name := range strings.Split(relativePath, "/") {
		next := s.childFolderIDByLabel(ctx, current, name)
		if next == domain.InvalidID {
			return domain.InvalidID
		}
		current = next
	}
	return current
}

func (s *SQLite) childFolderIDByLabel(ctx context.Context, parentID int, name string) int {
	rows, err := s.db.QueryContext(ctx, `SELECT id, path FROM media_folders WHERE parent_id = ?`, parentID)
	if err != nil {
		return domain.InvalidID
	}
	defer rows.Close()
	for rows.Next() {
		var folder domain.MediaFolder
		if err := rows.Scan(&folder.ID, &folder.Path); err != nil {
			continue
		}
		if folderLabel(folder) == name {
			return folder.ID
		}
	}
	return domain.InvalidID
}

func (s *SQLite) folderThumbnails(ctx context.Context, folderID int, limit int) []domain.ThumbnailRef {
	query := `WITH RECURSIVE sub(id) AS (
		SELECT ?
		UNION ALL
		SELECT f.id FROM media_folders f JOIN sub ON f.parent_id = sub.id)
	SELECT m.id FROM media m JOIN sub ON m.folder_id = sub.id
	ORDER BY (m.gps <> '')*2 + (m.mime_type LIKE 'image/%') +
		(CASE WHEN m.metadata_json <> '' AND m.metadata_json <> '{}' THEN 1 ELSE 0 END) DESC,
		m.path ASC
	LIMIT ?`
	rows, err := s.db.QueryContext(ctx, query, folderID, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	refs := []domain.ThumbnailRef{}
	seen := map[int]bool{}
	for rows.Next() {
		var mediaID int
		if err := rows.Scan(&mediaID); err != nil || seen[mediaID] {
			continue
		}
		seen[mediaID] = true
		refs = append(refs, domain.ThumbnailRef{MediaID: mediaID, Index: 0})
	}
	return refs
}

func (s *SQLite) FolderThumbnailRefs(ctx context.Context, folderID int, limit int) ([]domain.ThumbnailRef, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM media_folders WHERE id = ?`, folderID).Scan(&exists); err != nil {
		return nil, translateErr(err)
	}
	return s.folderThumbnails(ctx, folderID, limit), nil
}

func (s *SQLite) Media(ctx context.Context, id int) (domain.Media, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+mediaColumns+` FROM media m WHERE m.id = ?`, id)
	item, err := scanMedia(row)
	if err != nil {
		return item, translateErr(err)
	}
	if root := s.rootPathForFolder(ctx, item.FolderID); root != "" {
		item.RelativePath = strings.TrimPrefix(strings.TrimPrefix(item.Path, root), "/")
	}
	return item, nil
}

func (s *SQLite) MediaBatch(ctx context.Context, ids []int) ([]domain.Media, error) {
	if len(ids) == 0 {
		return []domain.Media{}, nil
	}
	unique := dedupeInts(ids)
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(unique)), ",")
	args := make([]any, len(unique))
	for index, id := range unique {
		args[index] = id
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+mediaColumns+` FROM media m WHERE m.id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	out := []domain.Media{}
	for rows.Next() {
		item, err := scanMedia(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	s.attachRelativePaths(ctx, out)
	return out, nil
}
// MediaInFolders returns every media item inside the given folders,
// including all nested subfolders.
func (s *SQLite) MediaInFolders(ctx context.Context, folderIDs []int) ([]domain.Media, error) {
	if len(folderIDs) == 0 {
		return []domain.Media{}, nil
	}
	unique := dedupeInts(folderIDs)
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(unique)), ",")
	args := make([]any, 0, len(unique))
	for _, id := range unique {
		args = append(args, id)
	}
	// Relative paths are computed against the nearest library-root ancestor so
	// archives of nested folders keep their on-disk structure.
	rootExpr := `COALESCE((SELECT rf.path FROM media_folders rf JOIN library_roots lr ON lr.folder_id = rf.id WHERE f.path LIKE rf.path || '/' || '%' OR f.path = rf.path ORDER BY length(rf.path) DESC LIMIT 1), f.path)`
	rel := relativePathExpr("m.path", "sub.root_path")
	query := `WITH RECURSIVE sub(id, root_path) AS (
		SELECT f.id, ` + rootExpr + ` FROM media_folders f WHERE f.id IN (` + placeholders + `)
		UNION ALL
		SELECT child.id, sub.root_path FROM media_folders child JOIN sub ON child.parent_id = sub.id
	)
	SELECT ` + mediaColumns + `, ` + rel + ` FROM media m JOIN sub ON m.folder_id = sub.id ORDER BY ` + rel
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Media{}
	for rows.Next() {
		var relativePath string
		item, err := scanMedia(rows, &relativePath)
		if err != nil {
			return nil, err
		}
		item.RelativePath = relativePath
		out = append(out, item)
	}
	return out, rows.Err()
}


func (s *SQLite) MediaByPath(ctx context.Context, mediaPath string) (domain.Media, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+mediaColumns+` FROM media m WHERE m.path = ?`, normalizePath(mediaPath))
	item, err := scanMedia(row)
	if err != nil {
		return item, translateErr(err)
	}
	return item, nil
}

func (s *SQLite) FavoriteViews(ctx context.Context, userID int) ([]domain.FavoriteView, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT fv.id, fv.name, COUNT(fvi.media_id)
		FROM favorite_views fv LEFT JOIN favorite_view_items fvi ON fvi.favorite_view_id = fv.id
		WHERE fv.user_id = ? GROUP BY fv.id, fv.name ORDER BY fv.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	views := []domain.FavoriteView{}
	for rows.Next() {
		var view domain.FavoriteView
		if err := rows.Scan(&view.ID, &view.Name, &view.Count); err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, rows.Err()
}

func (s *SQLite) FavoriteViewsForMedia(ctx context.Context, userID, mediaID int) ([]domain.FavoriteViewMembership, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT fv.id, fv.name, COUNT(fvi.media_id),
		EXISTS(SELECT 1 FROM favorite_view_items fvi2 WHERE fvi2.favorite_view_id = fv.id AND fvi2.media_id = ?)
		FROM favorite_views fv LEFT JOIN favorite_view_items fvi ON fvi.favorite_view_id = fv.id
		WHERE fv.user_id = ? GROUP BY fv.id, fv.name ORDER BY fv.name`, mediaID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	views := []domain.FavoriteViewMembership{}
	for rows.Next() {
		var view domain.FavoriteViewMembership
		if err := rows.Scan(&view.ID, &view.Name, &view.Count, &view.Contains); err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, rows.Err()
}

func (s *SQLite) FavoriteViewsForFolder(ctx context.Context, userID, folderID int) ([]domain.FavoriteViewMembership, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT fv.id, fv.name, COUNT(ff.folder_id),
		EXISTS(SELECT 1 FROM favorite_folders ff2 WHERE ff2.favorite_view_id = fv.id AND ff2.folder_id = ?)
		FROM favorite_views fv LEFT JOIN favorite_folders ff ON ff.favorite_view_id = fv.id
		WHERE fv.user_id = ? GROUP BY fv.id, fv.name ORDER BY fv.name`, folderID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	views := []domain.FavoriteViewMembership{}
	for rows.Next() {
		var view domain.FavoriteViewMembership
		if err := rows.Scan(&view.ID, &view.Name, &view.Count, &view.Contains); err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, rows.Err()
}

func (s *SQLite) CreateFavoriteView(ctx context.Context, userID int, name string) (domain.FavoriteView, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.FavoriteView{}, ErrConflict
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO favorite_views(user_id, name) VALUES(?,?)`, userID, name)
	if err != nil {
		return domain.FavoriteView{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.FavoriteView{}, err
	}
	return domain.FavoriteView{ID: int(id), Name: name, Count: 0}, nil
}

func (s *SQLite) UpdateFavoriteView(ctx context.Context, userID, viewID int, name string) (domain.FavoriteView, error) {
	var existing domain.FavoriteView
	err := s.db.QueryRowContext(ctx, `SELECT id, name FROM favorite_views WHERE id = ? AND user_id = ?`, viewID, userID).
		Scan(&existing.ID, &existing.Name)
	if err != nil {
		return existing, translateErr(err)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return existing, ErrConflict
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE favorite_views SET name = ? WHERE id = ?`, name, viewID); err != nil {
		return existing, err
	}
	existing.Name = name
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM favorite_view_items WHERE favorite_view_id = ?`, viewID).Scan(&count); err != nil {
		return existing, err
	}
	existing.Count = count
	return existing, nil
}

func (s *SQLite) DeleteFavoriteView(ctx context.Context, userID, viewID int) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM favorite_views WHERE id = ? AND user_id = ?`, viewID, userID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLite) FavoriteMedia(ctx context.Context, userID, viewID int, admin bool) ([]domain.Media, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM favorite_views WHERE id = ? AND user_id = ?`, viewID, userID).Scan(&exists); err != nil {
		return nil, translateErr(err)
	}
	sub, subArgs := accessibleSubtree(userID, admin)
	query := `WITH RECURSIVE sub(id) AS (` + sub + `
		UNION ALL
		SELECT f.id FROM media_folders f JOIN sub ON f.parent_id = sub.id)
	SELECT ` + mediaColumns + ` FROM favorite_view_items fvi
	JOIN media m ON m.id = fvi.media_id
	WHERE fvi.favorite_view_id = ? AND (? = 1 OR m.folder_id IN (SELECT id FROM sub))
	ORDER BY m.name`
	args := append([]any{}, subArgs...)
	args = append(args, viewID)
	if admin {
		args = append(args, 1)
	} else {
		args = append(args, 0)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	out := []domain.Media{}
	for rows.Next() {
		item, err := scanMedia(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		item.Favorite = true
		out = append(out, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	s.attachRelativePaths(ctx, out)
	return out, nil
}

func (s *SQLite) FavoriteMediaExpanded(ctx context.Context, userID, viewID int, admin bool) ([]domain.Media, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM favorite_views WHERE id = ? AND user_id = ?`, viewID, userID).Scan(&exists); err != nil {
		return nil, translateErr(err)
	}
	sub, subArgs := accessibleSubtree(userID, admin)
	query := `WITH RECURSIVE sub(id) AS (` + sub + `
		UNION ALL
		SELECT f.id FROM media_folders f JOIN sub ON f.parent_id = sub.id),
		fav_folders(id) AS (SELECT ff.folder_id FROM favorite_folders ff WHERE ff.favorite_view_id = ?),
		folder_sub(id) AS (SELECT id FROM fav_folders UNION ALL SELECT f.id FROM media_folders f JOIN folder_sub ON f.parent_id = folder_sub.id),
		mentions(media_id) AS (
			SELECT fvi.media_id FROM favorite_view_items fvi WHERE fvi.favorite_view_id = ?
			UNION ALL
			SELECT m2.id FROM media m2 JOIN folder_sub fs ON m2.folder_id = fs.id
		)
	SELECT ` + mediaColumns + ` FROM mentions JOIN media m ON m.id = mentions.media_id
	WHERE (? = 1 OR m.folder_id IN (SELECT id FROM sub))
	ORDER BY m.name`
	args := append([]any{}, subArgs...)
	args = append(args, viewID, viewID)
	if admin {
		args = append(args, 1)
	} else {
		args = append(args, 0)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	out := []domain.Media{}
	for rows.Next() {
		item, err := scanMedia(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		item.Favorite = true
		out = append(out, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	s.attachRelativePaths(ctx, out)
	return out, nil
}

func (s *SQLite) SetFavorite(ctx context.Context, userID, viewID, mediaID int, favorite bool) (domain.Media, error) {
	if _, err := s.Media(ctx, mediaID); err != nil {
		return domain.Media{}, err
	}
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM favorite_views WHERE id = ? AND user_id = ?`, viewID, userID).Scan(&exists); err != nil {
		return domain.Media{}, translateErr(err)
	}
	if favorite {
		if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO favorite_view_items(favorite_view_id, media_id) VALUES(?,?)`,
			viewID, mediaID); err != nil {
			return domain.Media{}, err
		}
	} else {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM favorite_view_items WHERE favorite_view_id = ? AND media_id = ?`,
			viewID, mediaID); err != nil {
			return domain.Media{}, err
		}
	}
	item, err := s.Media(ctx, mediaID)
	if err != nil {
		return item, err
	}
	item.Favorite, err = s.IsFavorite(ctx, userID, mediaID)
	return item, err
}

func (s *SQLite) IsFavorite(ctx context.Context, userID, mediaID int) (bool, error) {
	var ok bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM favorite_view_items fvi
		JOIN favorite_views fv ON fv.id = fvi.favorite_view_id
		WHERE fv.user_id = ? AND fvi.media_id = ?)`, userID, mediaID).Scan(&ok)
	return ok, err
}

func (s *SQLite) FavoritesForUser(ctx context.Context, userID int, mediaIDs []int) (map[int]bool, error) {
	if len(mediaIDs) == 0 {
		return map[int]bool{}, nil
	}
	out := map[int]bool{}
	for _, id := range mediaIDs {
		out[id] = false
	}
	ph := make([]string, len(mediaIDs))
	args := make([]any, 0, len(mediaIDs)+1)
	args = append(args, userID)
	for i, id := range mediaIDs {
		ph[i] = "?"
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT fvi.media_id FROM favorite_view_items fvi
		JOIN favorite_views fv ON fv.id = fvi.favorite_view_id
		WHERE fv.user_id = ? AND fvi.media_id IN (`+strings.Join(ph, ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

func (s *SQLite) UpdateGPS(ctx context.Context, id int, patch domain.GPSPatch) (domain.Media, error) {
	return s.UpdateMediaDetails(ctx, id, domain.MediaDetailsPatch{GPS: patch.GPS})
}

func (s *SQLite) UpdateMediaDetails(ctx context.Context, id int, patch domain.MediaDetailsPatch) (domain.Media, error) {
	if _, err := s.Media(ctx, id); err != nil {
		return domain.Media{}, err
	}
	sets := []string{}
	args := []any{}
	if patch.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, strings.TrimSpace(*patch.Name))
	}
	if patch.GPS != nil {
		gps := strings.TrimSpace(*patch.GPS)
		lat, lng := gpsCoords(gps)
		sets = append(sets, "gps = ?", "gps_lat = ?", "gps_lng = ?")
		args = append(args, gps, lat, lng)
	}
	if patch.TakenAt != nil {
		sets = append(sets, "taken_at = ?")
		args = append(args, strings.TrimSpace(*patch.TakenAt))
	}
	if len(sets) > 0 {
		args = append(args, id)
		if _, err := s.db.ExecContext(ctx, `UPDATE media SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
			return domain.Media{}, err
		}
	}
	return s.Media(ctx, id)
}

func (s *SQLite) UpdateMediaMetadata(ctx context.Context, id int, metadata map[string]any, gps string, takenAt string, metadataError string, replaceTakenAt bool) error {
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	sets := []string{"metadata_json = ?", "metadata_error = ?"}
	args := []any{string(metadataJSON), metadataError}
	gps = strings.TrimSpace(gps)
	if gps != "" {
		gpsLat, gpsLng := gpsCoords(gps)
		sets = append(sets, "gps = ?", "gps_lat = ?", "gps_lng = ?")
		args = append(args, gps, gpsLat, gpsLng)
	}
	if takenAt != "" {
		if replaceTakenAt {
			sets = append(sets, "taken_at = ?")
		} else {
			sets = append(sets, "taken_at = CASE WHEN taken_at = '' THEN ? ELSE taken_at END")
		}
		args = append(args, takenAt)
	}
	args = append(args, id)
	if _, err := s.db.ExecContext(ctx, `UPDATE media SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
		return err
	}
	return nil
}

func (s *SQLite) bulkTargetSub(ctx context.Context, ids []int, folderIDs []int) (cte, targetSub string, args []any) {
	if len(folderIDs) > 0 {
		folderPh := make([]string, len(folderIDs))
		folderArgs := make([]any, len(folderIDs))
		for i, fid := range folderIDs {
			folderPh[i] = "?"
			folderArgs[i] = fid
		}
		cte = `WITH RECURSIVE descend(folder_id) AS (
			SELECT id FROM media_folders WHERE id IN (` + strings.Join(folderPh, ",") + `)
			UNION ALL
			SELECT f.id FROM media_folders f JOIN descend d ON f.parent_id = d.folder_id) `
		targetSub = `SELECT m.id FROM media m JOIN descend d ON m.folder_id = d.folder_id`
		args = append(args, folderArgs...)
	}
	if len(ids) > 0 {
		idPh := make([]string, len(ids))
		for i, id := range ids {
			idPh[i] = "?"
			args = append(args, id)
		}
		if targetSub != "" {
			targetSub += ` UNION `
		} else {
			targetSub = `SELECT id FROM media WHERE id IN (`
		}
		if targetSub == `SELECT id FROM media WHERE id IN (` {
			targetSub += strings.Join(idPh, ",") + `)`
		} else {
			targetSub += `SELECT id FROM media WHERE id IN (` + strings.Join(idPh, ",") + `)`
		}
	}
	return
}

func (s *SQLite) BulkUpdateMediaGPS(ctx context.Context, ids []int, folderIDs []int, gps string, lat, lng float64) ([]domain.BulkMediaResult, error) {
	cte, targetSub, args := s.bulkTargetSub(ctx, ids, folderIDs)
	if targetSub == "" {
		return []domain.BulkMediaResult{}, nil
	}
	setClause := "gps = ?, gps_lat = ?, gps_lng = ?"
	setArgs := []any{gps, lat, lng}
	fullArgs := append(setArgs, args...)
	if _, err := s.db.ExecContext(ctx, cte+`UPDATE media SET `+setClause+` WHERE id IN (`+targetSub+`)`, fullArgs...); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, cte+`SELECT m.id, m.gps FROM media m WHERE m.id IN (`+targetSub+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.BulkMediaResult
	for rows.Next() {
		var r domain.BulkMediaResult
		if err := rows.Scan(&r.ID, &r.GPS); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *SQLite) BulkUpdateMediaSetTime(ctx context.Context, ids []int, folderIDs []int, takenAt string) ([]domain.BulkMediaResult, error) {
	cte, targetSub, args := s.bulkTargetSub(ctx, ids, folderIDs)
	if targetSub == "" {
		return []domain.BulkMediaResult{}, nil
	}
	setClause := "taken_at = ?"
	fullArgs := append([]any{takenAt}, args...)
	if _, err := s.db.ExecContext(ctx, cte+`UPDATE media SET `+setClause+` WHERE id IN (`+targetSub+`)`, fullArgs...); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, cte+`SELECT m.id, m.taken_at FROM media m WHERE m.id IN (`+targetSub+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.BulkMediaResult
	for rows.Next() {
		var r domain.BulkMediaResult
		if err := rows.Scan(&r.ID, &r.TakenAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *SQLite) BulkUpdateMediaShiftTime(ctx context.Context, ids []int, folderIDs []int, shiftMinutes float64) ([]domain.BulkMediaResult, error) {
	cte, targetSub, args := s.bulkTargetSub(ctx, ids, folderIDs)
	if targetSub == "" {
		return []domain.BulkMediaResult{}, nil
	}
	setClause := "taken_at = CASE WHEN taken_at = '' THEN taken_at ELSE datetime(taken_at, ? || ' minutes') || 'Z' END"
	fullArgs := append(args, shiftMinutes)
	if _, err := s.db.ExecContext(ctx, cte+`UPDATE media SET `+setClause+` WHERE id IN (`+targetSub+`)`, fullArgs...); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, cte+`SELECT m.id, m.taken_at FROM media m WHERE m.id IN (`+targetSub+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.BulkMediaResult
	for rows.Next() {
		var r domain.BulkMediaResult
		if err := rows.Scan(&r.ID, &r.TakenAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *SQLite) queryMediaByIDs(ctx context.Context, idCond string, idArgs []any) ([]domain.Media, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+mediaColumns+` FROM media m WHERE m.id IN (`+idCond+`)`, idArgs...)
	if err != nil {
		return nil, err
	}
	var out []domain.Media
	for rows.Next() {
		item, err := scanMedia(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	s.attachRelativePaths(ctx, out)
	return out, nil
}

func (s *SQLite) GeotaggedMedia(ctx context.Context, userID int, admin bool, libraryID, folderID int) ([]domain.MapMedia, error) {
	var query string
	switch {
	case folderID > 0:
		query = `WITH RECURSIVE covers(folder_id, library_id) AS (
			SELECT ?, ?
			UNION ALL
			SELECT f.id, covers.library_id FROM media_folders f JOIN covers ON f.parent_id = covers.folder_id)
		SELECT ` + mediaColumns + `, MIN(covers.library_id)
		FROM media m JOIN covers ON covers.folder_id = m.folder_id
		WHERE m.gps <> '' AND (? = 1 OR EXISTS(SELECT 1 FROM library_access la WHERE la.library_id = covers.library_id AND la.user_id = ?))
		GROUP BY m.id`
	case libraryID > 0:
		query = `WITH RECURSIVE covers(folder_id, library_id) AS (
			SELECT lr.folder_id, lr.library_id FROM library_roots lr WHERE lr.library_id = ?
			UNION ALL
			SELECT f.id, covers.library_id FROM media_folders f JOIN covers ON f.parent_id = covers.folder_id)
		SELECT ` + mediaColumns + `, MIN(covers.library_id)
		FROM media m JOIN covers ON covers.folder_id = m.folder_id
		WHERE m.gps <> '' AND (? = 1 OR EXISTS(SELECT 1 FROM library_access la WHERE la.library_id = covers.library_id AND la.user_id = ?))
		GROUP BY m.id`
	default:
		query = `WITH RECURSIVE covers(folder_id, library_id) AS (
			SELECT lr.folder_id, lr.library_id FROM library_roots lr
			UNION ALL
			SELECT f.id, covers.library_id FROM media_folders f JOIN covers ON f.parent_id = covers.folder_id)
		SELECT ` + mediaColumns + `, MIN(covers.library_id)
		FROM media m JOIN covers ON covers.folder_id = m.folder_id
		WHERE m.gps <> '' AND (? = 1 OR EXISTS(SELECT 1 FROM library_access la WHERE la.library_id = covers.library_id AND la.user_id = ?))
		GROUP BY m.id`
	}
	args := []any{}
	switch {
	case folderID > 0:
		if admin {
			args = append(args, folderID, libraryID, 1, 0)
		} else {
			args = append(args, folderID, libraryID, 0, userID)
		}
	case libraryID > 0:
		if admin {
			args = append(args, libraryID, 1, 0)
		} else {
			args = append(args, libraryID, 0, userID)
		}
	default:
		if admin {
			args = append(args, 1, 0)
		} else {
			args = append(args, 0, userID)
		}
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	type geoTuple struct {
		item      domain.Media
		libraryID int
	}
	tuples := []geoTuple{}
	for rows.Next() {
		var libraryID int
		item, err := scanMedia(rows, &libraryID)
		if err != nil {
			rows.Close()
			return nil, err
		}
		tuples = append(tuples, geoTuple{item: item, libraryID: libraryID})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := []domain.MapMedia{}
	roots := map[int]string{}
	for _, tuple := range tuples {
		root, ok := roots[tuple.item.FolderID]
		if !ok {
			root = s.rootPathForFolder(ctx, tuple.item.FolderID)
			roots[tuple.item.FolderID] = root
		}
		if root != "" {
			tuple.item.RelativePath = strings.TrimPrefix(strings.TrimPrefix(tuple.item.Path, root), "/")
		}
		out = append(out, domain.MapMedia{Media: tuple.item, LibraryID: tuple.libraryID})
	}
	return out, nil
}

// MediaInArea returns geotagged media the user may read whose point falls inside
// bounds. The rectangle test is pushed into the R*Tree media_geo index (a point
// lies inside the box iff minLat <= north, maxLat >= south, minLng <= east and
// maxLng >= west), so only matching ids are materialized.
func (s *SQLite) MediaInArea(ctx context.Context, userID int, admin bool, libraryID, folderID int, bounds domain.Bounds) ([]domain.MapMedia, error) {
	var query string
	switch {
	case folderID > 0:
		query = `WITH RECURSIVE covers(folder_id, library_id) AS (
			SELECT ?, ?
			UNION ALL
			SELECT f.id, covers.library_id FROM media_folders f JOIN covers ON f.parent_id = covers.folder_id)
		SELECT ` + mediaColumns + `, MIN(covers.library_id)
		FROM media m JOIN media_geo g ON g.id = m.id JOIN covers ON covers.folder_id = m.folder_id
		WHERE g.minLat <= ? AND g.maxLat >= ? AND g.minLng <= ? AND g.maxLng >= ?
			AND (? = 1 OR EXISTS(SELECT 1 FROM library_access la WHERE la.library_id = covers.library_id AND la.user_id = ?))
		GROUP BY m.id`
	case libraryID > 0:
		query = `WITH RECURSIVE covers(folder_id, library_id) AS (
			SELECT lr.folder_id, lr.library_id FROM library_roots lr WHERE lr.library_id = ?
			UNION ALL
			SELECT f.id, covers.library_id FROM media_folders f JOIN covers ON f.parent_id = covers.folder_id)
		SELECT ` + mediaColumns + `, MIN(covers.library_id)
		FROM media m JOIN media_geo g ON g.id = m.id JOIN covers ON covers.folder_id = m.folder_id
		WHERE g.minLat <= ? AND g.maxLat >= ? AND g.minLng <= ? AND g.maxLng >= ?
			AND (? = 1 OR EXISTS(SELECT 1 FROM library_access la WHERE la.library_id = covers.library_id AND la.user_id = ?))
		GROUP BY m.id`
	default:
		query = `WITH RECURSIVE covers(folder_id, library_id) AS (
			SELECT lr.folder_id, lr.library_id FROM library_roots lr
			UNION ALL
			SELECT f.id, covers.library_id FROM media_folders f JOIN covers ON f.parent_id = covers.folder_id)
		SELECT ` + mediaColumns + `, MIN(covers.library_id)
		FROM media m JOIN media_geo g ON g.id = m.id JOIN covers ON covers.folder_id = m.folder_id
		WHERE g.minLat <= ? AND g.maxLat >= ? AND g.minLng <= ? AND g.maxLng >= ?
			AND (? = 1 OR EXISTS(SELECT 1 FROM library_access la WHERE la.library_id = covers.library_id AND la.user_id = ?))
		GROUP BY m.id`
	}
	args := []any{}
	switch {
	case folderID > 0:
		args = append(args, folderID, libraryID)
	case libraryID > 0:
		args = append(args, libraryID)
	}
	args = append(args, bounds.North, bounds.South, bounds.East, bounds.West)
	if admin {
		args = append(args, 1, 0)
	} else {
		args = append(args, 0, userID)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	type geoTuple struct {
		item      domain.Media
		libraryID int
	}
	tuples := []geoTuple{}
	for rows.Next() {
		var libraryID int
		item, err := scanMedia(rows, &libraryID)
		if err != nil {
			rows.Close()
			return nil, err
		}
		tuples = append(tuples, geoTuple{item: item, libraryID: libraryID})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := []domain.MapMedia{}
	roots := map[int]string{}
	for _, tuple := range tuples {
		root, ok := roots[tuple.item.FolderID]
		if !ok {
			root = s.rootPathForFolder(ctx, tuple.item.FolderID)
			roots[tuple.item.FolderID] = root
		}
		if root != "" {
			tuple.item.RelativePath = strings.TrimPrefix(strings.TrimPrefix(tuple.item.Path, root), "/")
		}
		out = append(out, domain.MapMedia{Media: tuple.item, LibraryID: tuple.libraryID})
	}
	return out, nil
}

func (s *SQLite) MediaForLibrary(ctx context.Context, userID, libraryID int) ([]domain.Media, error) {
	if _, err := s.loadLibrary(ctx, libraryID); err != nil {
		return nil, err
	}
	rel := relativePathExpr("m.path", "covers.root_path")
	query := `WITH RECURSIVE covers(folder_id, root_path) AS (
		SELECT lr.folder_id, f.path FROM library_roots lr JOIN media_folders f ON f.id = lr.folder_id WHERE lr.library_id = ?
		UNION ALL
		SELECT f.id, covers.root_path FROM media_folders f JOIN covers ON f.parent_id = covers.folder_id)
	SELECT ` + mediaColumns + `, ` + rel + `, ` + favoriteExpr + ` FROM media m JOIN covers ON covers.folder_id = m.folder_id
	ORDER BY ` + rel
	rows, err := s.db.QueryContext(ctx, query, libraryID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Media{}
	for rows.Next() {
		var relativePath string
		var favorite bool
		item, err := scanMedia(rows, &relativePath, &favorite)
		if err != nil {
			return nil, err
		}
		item.RelativePath = relativePath
		item.Favorite = favorite
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *SQLite) MediaForFolder(ctx context.Context, userID, libraryID, folderID int) ([]domain.Media, error) {
	if _, err := s.FolderChain(ctx, libraryID, folderID); err != nil {
		return nil, err
	}
	rel := relativePathExpr("m.path", "covers.root_path")
	query := `WITH RECURSIVE covers(folder_id, root_path) AS (
		SELECT f.id, f.path FROM media_folders f WHERE f.id = ?
		UNION ALL
		SELECT f.id, covers.root_path FROM media_folders f JOIN covers ON f.parent_id = covers.folder_id)
	SELECT ` + mediaColumns + `, ` + rel + `, ` + favoriteExpr + ` FROM media m JOIN covers ON covers.folder_id = m.folder_id
	ORDER BY ` + rel
	rows, err := s.db.QueryContext(ctx, query, folderID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Media{}
	for rows.Next() {
		var relativePath string
		var favorite bool
		item, err := scanMedia(rows, &relativePath, &favorite)
		if err != nil {
			return nil, err
		}
		item.RelativePath = relativePath
		item.Favorite = favorite
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *SQLite) FoldersForLibrary(ctx context.Context, libraryID int) ([]domain.MediaFolder, error) {
	if _, err := s.loadLibrary(ctx, libraryID); err != nil {
		return nil, err
	}
	rel := relativePathExpr("path", "root_path")
	rows, err := s.db.QueryContext(ctx, `WITH RECURSIVE covers(id, parent_id, path, root_path) AS (
		SELECT f.id, COALESCE(f.parent_id, -1), f.path, f.path FROM library_roots lr JOIN media_folders f ON f.id = lr.folder_id WHERE lr.library_id = ?
		UNION
		SELECT f.id, COALESCE(f.parent_id, -1), f.path, covers.root_path FROM media_folders f JOIN covers ON f.parent_id = covers.id)
		SELECT DISTINCT id, parent_id, path, `+rel+` FROM covers ORDER BY path`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.MediaFolder{}
	for rows.Next() {
		var folder domain.MediaFolder
		if err := rows.Scan(&folder.ID, &folder.ParentID, &folder.Path, &folder.RelativePath); err != nil {
			return nil, err
		}
		out = append(out, folder)
	}
	return out, rows.Err()
}

func (s *SQLite) ThumbnailCleanupRefsForLibrary(ctx context.Context, libraryID int) (domain.ThumbnailCleanupRefs, error) {
	if _, err := s.loadLibrary(ctx, libraryID); err != nil {
		return domain.ThumbnailCleanupRefs{}, err
	}
	covers := `WITH RECURSIVE covers(folder_id) AS (
		SELECT lr.folder_id FROM library_roots lr WHERE lr.library_id = ?
		UNION ALL
		SELECT f.id FROM media_folders f JOIN covers ON f.parent_id = covers.folder_id)`
	mediaRows, err := s.db.QueryContext(ctx, covers+`
		SELECT DISTINCT t.media_id, t.thumbnail_index
		FROM thumbnails t JOIN media m ON m.id = t.media_id JOIN covers ON covers.folder_id = m.folder_id
		ORDER BY t.media_id, t.thumbnail_index`, libraryID)
	if err != nil {
		return domain.ThumbnailCleanupRefs{}, err
	}
	refs := domain.ThumbnailCleanupRefs{}
	for mediaRows.Next() {
		var ref domain.ThumbnailRef
		if err := mediaRows.Scan(&ref.MediaID, &ref.Index); err != nil {
			mediaRows.Close()
			return domain.ThumbnailCleanupRefs{}, err
		}
		refs.Media = append(refs.Media, ref)
	}
	if err := mediaRows.Close(); err != nil {
		return domain.ThumbnailCleanupRefs{}, err
	}
	folderRows, err := s.db.QueryContext(ctx, covers+`
		SELECT DISTINCT ftf.folder_id
		FROM folder_thumbnail_files ftf JOIN covers ON covers.folder_id = ftf.folder_id
		ORDER BY ftf.folder_id`, libraryID)
	if err != nil {
		return domain.ThumbnailCleanupRefs{}, err
	}
	defer folderRows.Close()
	for folderRows.Next() {
		var folderID int
		if err := folderRows.Scan(&folderID); err != nil {
			return domain.ThumbnailCleanupRefs{}, err
		}
		refs.Folders = append(refs.Folders, folderID)
	}
	return refs, folderRows.Err()
}

func (s *SQLite) SetMediaActionError(ctx context.Context, id int, action, message string) error {
	column := ""
	switch action {
	case "metadata":
		column = "metadata_error"
	case "thumbnail":
		column = "thumbnail_error"
	default:
		return ErrNotFound
	}
	query := `UPDATE media SET ` + column + ` = ? WHERE id = ?`
	res, err := s.db.ExecContext(ctx, query, strings.TrimSpace(message), id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLite) PruneFolder(ctx context.Context, rootFolderID int, keepFolders, keepMedia map[int]bool) error {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM media_folders WHERE id = ?`, rootFolderID).Scan(&exists); err != nil {
		return translateErr(err)
	}
	keepMediaPlaceholders, mediaArgs := idsIN(keepMedia)
	keepFolderPlaceholders, folderArgs := idsIN(keepFolders)
	subtree := `WITH RECURSIVE sub(id) AS (
		SELECT ?
		UNION ALL
		SELECT f.id FROM media_folders f JOIN sub ON f.parent_id = sub.id)`
	if _, err := s.db.ExecContext(ctx, subtree+` DELETE FROM media
		WHERE folder_id IN (SELECT id FROM sub) AND id NOT IN (`+keepMediaPlaceholders+`)`,
		append([]any{rootFolderID}, mediaArgs...)...); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, subtree+` DELETE FROM media_folders
		WHERE id IN (SELECT id FROM sub) AND id <> ? AND id NOT IN (`+keepFolderPlaceholders+`)
		AND id NOT IN (SELECT DISTINCT folder_id FROM media)`,
		append([]any{rootFolderID, rootFolderID}, folderArgs...)...); err != nil {
		return err
	}
	return nil
}

func (s *SQLite) DeleteLibrary(ctx context.Context, id int) error {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM libraries WHERE id = ?`, id).Scan(&exists); err != nil {
		return translateErr(err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM libraries WHERE id = ?`, id); err != nil {
		return err
	}
	covers := `WITH RECURSIVE covers(folder_id, library_id) AS (
		SELECT lr.folder_id, lr.library_id FROM library_roots lr
		UNION ALL
		SELECT f.id, covers.library_id FROM media_folders f JOIN covers ON f.parent_id = covers.folder_id)`
	if _, err := s.db.ExecContext(ctx, covers+` DELETE FROM media
		WHERE folder_id NOT IN (SELECT covers.folder_id FROM covers)`); err != nil {
		return err
	}
	for {
		res, err := s.db.ExecContext(ctx, `DELETE FROM media_folders
			WHERE id NOT IN (SELECT folder_id FROM library_roots)
			  AND id NOT IN (SELECT DISTINCT folder_id FROM media)
			  AND id NOT IN (SELECT DISTINCT parent_id FROM media_folders WHERE parent_id IS NOT NULL)`)
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return nil
		}
	}
}

func (s *SQLite) CreateLibrary(ctx context.Context, library domain.Library) (domain.Library, error) {
	name := strings.TrimSpace(library.Name)
	if name == "" {
		return domain.Library{}, ErrConflict
	}
	if s.libraryNameExists(ctx, name, domain.InvalidID) {
		return domain.Library{}, ErrConflict
	}
	roots, err := s.ensureRoots(ctx, library.Roots)
	if err != nil {
		return domain.Library{}, err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO libraries(name) VALUES(?)`, name)
	if err != nil {
		return domain.Library{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.Library{}, err
	}
	for _, root := range roots {
		if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO library_roots(library_id, folder_id, watch) VALUES(?,?,?)`,
			id, root.ID, root.Watch); err != nil {
			return domain.Library{}, err
		}
	}
	library.ID = int(id)
	library.Name = name
	library.Roots = roots
	return library, nil
}

func (s *SQLite) UpdateLibrary(ctx context.Context, library domain.Library) error {
	name := strings.TrimSpace(library.Name)
	if name == "" {
		return ErrConflict
	}
	if _, err := s.loadLibrary(ctx, library.ID); err != nil {
		return err
	}
	if s.libraryNameExists(ctx, name, library.ID) {
		return ErrConflict
	}
	roots, err := s.ensureRoots(ctx, library.Roots)
	if err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE libraries SET name = ? WHERE id = ?`,
		name, library.ID); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM library_roots WHERE library_id = ?`, library.ID); err != nil {
		return err
	}
	for _, root := range roots {
		if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO library_roots(library_id, folder_id, watch) VALUES(?,?,?)`,
			library.ID, root.ID, root.Watch); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLite) libraryNameExists(ctx context.Context, name string, exceptID int) bool {
	var id int
	err := s.db.QueryRowContext(ctx, `SELECT id FROM libraries WHERE lower(name) = lower(?) AND id <> ? LIMIT 1`,
		name, exceptID).Scan(&id)
	return err == nil
}

func (s *SQLite) ensureRoots(ctx context.Context, roots []domain.LibraryRoot) ([]domain.LibraryRoot, error) {
	out := make([]domain.LibraryRoot, 0, len(roots))
	seen := map[int]bool{}
	for _, root := range roots {
		root.Path = normalizePath(root.Path)
		if root.Path == "" {
			continue
		}
		folderID, err := s.ensureFolderByPath(ctx, root.Path)
		if err != nil {
			return nil, err
		}
		if seen[folderID] {
			continue
		}
		seen[folderID] = true
		out = append(out, domain.LibraryRoot{ID: folderID, Path: root.Path, Watch: root.Watch})
	}
	for i := range out {
		for j := range out {
			if i != j && nestedPath(out[i].Path, out[j].Path) {
				return nil, ErrNestedRoot
			}
		}
	}
	return out, nil
}

func (s *SQLite) ensureFolderByPath(ctx context.Context, path string) (int, error) {
	var id int
	err := s.db.QueryRowContext(ctx, `SELECT id FROM media_folders WHERE path = ?`, path).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO media_folders(parent_id, path) VALUES(NULL, ?)`, path)
	if err != nil {
		return 0, err
	}
	newID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(newID), nil
}

func (s *SQLite) ensureFolderByPathTx(ctx context.Context, tx *sql.Tx, path string) (int, error) {
	var id int
	err := tx.QueryRowContext(ctx, `SELECT id FROM media_folders WHERE path = ?`, path).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO media_folders(parent_id, path) VALUES(NULL, ?)`, path)
	if err != nil {
		return 0, err
	}
	newID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(newID), nil
}

func (s *SQLite) SetAccess(ctx context.Context, libraryID, userID int, allowed bool) error {
	if _, err := s.Library(ctx, libraryID); err != nil {
		return err
	}
	if _, err := s.User(ctx, userID); err != nil {
		return err
	}
	if allowed {
		_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO library_access(library_id, user_id) VALUES(?,?)`,
			libraryID, userID)
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM library_access WHERE library_id = ? AND user_id = ?`, libraryID, userID)
	return err
}

func (s *SQLite) LibraryAccess(ctx context.Context, libraryID int) ([]domain.LibraryUserAccess, error) {
	if _, err := s.Library(ctx, libraryID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT u.id, u.login, u.password_hash, u.role,
		CASE WHEN u.role = 'admin' OR la.user_id IS NOT NULL THEN 1 ELSE 0 END
		FROM users u LEFT JOIN library_access la ON la.user_id = u.id AND la.library_id = ?
		ORDER BY u.role, u.login`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var access []domain.LibraryUserAccess
	for rows.Next() {
		var item domain.LibraryUserAccess
		if err := rows.Scan(&item.User.ID, &item.User.Login, &item.User.PasswordHash, &item.User.Role, &item.Allowed); err != nil {
			return nil, err
		}
		access = append(access, item)
	}
	return access, rows.Err()
}

func (s *SQLite) UpsertFolder(ctx context.Context, folder domain.MediaFolder) (domain.MediaFolder, error) {
	folder.Path = normalizePath(folder.Path)
	var id int
	if folder.Path != "" {
		err := s.db.QueryRowContext(ctx, `SELECT id FROM media_folders WHERE path = ?`, folder.Path).Scan(&id)
		if err == nil {
			folder.ID = id
		} else if !errors.Is(err, sql.ErrNoRows) {
			return folder, err
		}
	}
	parentID := folder.ParentID
	if folder.Path != "" {
		if dirID, err := s.folderIDByPath(ctx, filepath.Dir(folder.Path)); err == nil && dirID != folder.ID {
			parentID = dirID
		}
	}
	if folder.ID != domain.InvalidID && folder.ID != 0 {
		if _, err := s.db.ExecContext(ctx, `UPDATE media_folders SET parent_id = ? WHERE id = ?`,
			parentIDOrNull(parentID), folder.ID); err != nil {
			return folder, err
		}
		folder.ParentID = parentID
		return folder, nil
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO media_folders(parent_id, path) VALUES(?,?)`,
		parentIDOrNull(parentID), folder.Path)
	if err != nil {
		return folder, err
	}
	newID, err := res.LastInsertId()
	if err != nil {
		return folder, err
	}
	folder.ID = int(newID)
	folder.ParentID = parentID
	return folder, nil
}

func (s *SQLite) folderIDByPath(ctx context.Context, path string) (int, error) {
	var id int
	err := s.db.QueryRowContext(ctx, `SELECT id FROM media_folders WHERE path = ?`, normalizePath(path)).Scan(&id)
	if err != nil {
		return domain.InvalidID, err
	}
	return id, nil
}

func (s *SQLite) UpsertMedia(ctx context.Context, media domain.Media) (domain.Media, error) {
	media.Path = normalizePath(media.Path)
	if media.Path != "" {
		if id, err := s.mediaIDByPath(ctx, media.Path); err == nil {
			media.ID = id
		} else if !errors.Is(err, sql.ErrNoRows) {
			return media, err
		}
	}
	folderID := media.FolderID
	if media.Path != "" {
		if dirID, err := s.folderIDByPath(ctx, filepath.Dir(media.Path)); err == nil {
			folderID = dirID
		}
	}
	if folderID == domain.InvalidID || folderID == 0 {
		return media, ErrNotFound
	}
	if media.Path != "" && media.ThumbnailError == "" {
		if existing, err := s.mediaByPath(ctx, media.Path); err == nil {
			media.ThumbnailError = existing.ThumbnailError
		}
	}
	metadataJSON, err := json.Marshal(media.Metadata)
	if err != nil {
		return media, err
	}
	media.GPS = strings.TrimSpace(media.GPS)
	gpsLat, gpsLng := gpsCoords(media.GPS)
	if media.ID == domain.InvalidID || media.ID == 0 {
		res, err := s.db.ExecContext(ctx, `INSERT INTO media(folder_id, path, name, mime_type, size, metadata_json, gps, gps_lat, gps_lng, taken_at, metadata_error, thumbnail_error)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
			folderID, media.Path, media.Name, media.MIMEType, media.Size,
			string(metadataJSON), media.GPS, gpsLat, gpsLng, media.TakenAt, media.MetadataError, media.ThumbnailError)
		if err != nil {
			return media, err
		}
		newID, err := res.LastInsertId()
		if err != nil {
			return media, err
		}
		media.ID = int(newID)
	} else {
		if _, err := s.db.ExecContext(ctx, `UPDATE media SET folder_id = ?, name = ?, mime_type = ?, size = ?, metadata_json = ?, gps = ?, gps_lat = ?, gps_lng = ?, taken_at = ?, metadata_error = ?, thumbnail_error = ? WHERE id = ?`,
			folderID, media.Name, media.MIMEType, media.Size,
			string(metadataJSON), media.GPS, gpsLat, gpsLng, media.TakenAt, media.MetadataError, media.ThumbnailError, media.ID); err != nil {
			return media, err
		}
	}
	media.FolderID = folderID
	return media, nil
}

func (s *SQLite) mediaIDByPath(ctx context.Context, mediaPath string) (int, error) {
	var id int
	err := s.db.QueryRowContext(ctx, `SELECT id FROM media WHERE path = ?`, normalizePath(mediaPath)).Scan(&id)
	if err != nil {
		return domain.InvalidID, err
	}
	return id, nil
}

func (s *SQLite) mediaByPath(ctx context.Context, mediaPath string) (domain.Media, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+mediaColumns+` FROM media m WHERE m.path = ?`, normalizePath(mediaPath))
	return scanMedia(row)
}

func (s *SQLite) Thumbnail(ctx context.Context, mediaID int, index int) (domain.Thumbnail, error) {
	var thumbnail domain.Thumbnail
	err := s.db.QueryRowContext(ctx, `SELECT media_id, thumbnail_index, mime_type FROM thumbnails WHERE media_id = ? AND thumbnail_index = ?`,
		mediaID, index).Scan(&thumbnail.MediaID, &thumbnail.Index, &thumbnail.MIMEType)
	if err != nil {
		return thumbnail, translateErr(err)
	}
	return thumbnail, nil
}

func (s *SQLite) FolderThumbnailFile(ctx context.Context, folderID int) (domain.FolderThumbnail, error) {
	var thumbnail domain.FolderThumbnail
	err := s.db.QueryRowContext(ctx, `SELECT folder_id, mime_type FROM folder_thumbnail_files WHERE folder_id = ?`, folderID).Scan(&thumbnail.FolderID, &thumbnail.MIMEType)
	if err != nil {
		return thumbnail, translateErr(err)
	}
	return thumbnail, nil
}

func (s *SQLite) UpsertFolderThumbnail(ctx context.Context, thumbnail domain.FolderThumbnail) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO folder_thumbnail_files(folder_id, mime_type) VALUES(?,?)
		ON CONFLICT(folder_id) DO UPDATE SET mime_type = excluded.mime_type`, thumbnail.FolderID, thumbnail.MIMEType); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM folder_thumbnails WHERE folder_id = ?`, thumbnail.FolderID); err != nil {
		return err
	}
	for index, ref := range thumbnail.Sources {
		if _, err := tx.ExecContext(ctx, `INSERT INTO folder_thumbnails(folder_id, thumbnail_index, source_media_id) VALUES(?,?,?)`,
			thumbnail.FolderID, index, ref.MediaID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLite) UpsertThumbnail(ctx context.Context, thumbnail domain.Thumbnail) error {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM media WHERE id = ?`, thumbnail.MediaID).Scan(&exists); err != nil {
		return translateErr(err)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO thumbnails(media_id, thumbnail_index, mime_type) VALUES(?,?,?)
		ON CONFLICT(media_id, thumbnail_index) DO UPDATE SET mime_type = excluded.mime_type`,
		thumbnail.MediaID, thumbnail.Index, thumbnail.MIMEType)
	return err
}

func (s *SQLite) SaveJob(ctx context.Context, job domain.BackgroundJob) error {
	options, err := json.Marshal(job.Options)
	if err != nil {
		return err
	}
	var finished any
	if job.FinishedAt != nil {
		finished = job.FinishedAt.UTC().Format(time.RFC3339Nano)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO background_jobs(id, category, type, library_id, library_name, root_path, status, paused, cancelable, current_path, processed, total, error, started_at, finished_at, options_json)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET category = excluded.category, type = excluded.type, library_id = excluded.library_id, library_name = excluded.library_name,
			root_path = excluded.root_path, status = excluded.status, paused = excluded.paused, cancelable = excluded.cancelable,
			current_path = excluded.current_path, processed = excluded.processed, total = excluded.total, error = excluded.error,
			started_at = excluded.started_at, finished_at = excluded.finished_at, options_json = excluded.options_json`,
		job.ID, job.Category, job.Type, job.LibraryID, job.LibraryName, job.RootPath, job.Status, boolInt(job.Paused), boolInt(job.Cancelable),
		job.CurrentPath, job.Processed, job.Total, job.Error, job.StartedAt.UTC().Format(time.RFC3339Nano), finished, string(options))
	return err
}

func (s *SQLite) Jobs(ctx context.Context) ([]domain.BackgroundJob, error) {
	return s.jobsWhere(ctx, "")
}

func (s *SQLite) UnfinishedJobs(ctx context.Context) ([]domain.BackgroundJob, error) {
	return s.jobsWhere(ctx, `WHERE status IN ('running', 'paused', 'cancelling')`)
}

func (s *SQLite) DeleteFinishedJobsBefore(ctx context.Context, before time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM background_jobs WHERE finished_at IS NOT NULL AND finished_at < ?`, before.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *SQLite) ScheduledTasks(ctx context.Context) ([]domain.ScheduledTask, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, task_type, library_id, cron, enabled, last_run_at, next_run_at
		FROM scheduled_tasks ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []domain.ScheduledTask{}
	for rows.Next() {
		task, err := scanScheduledTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s *SQLite) ScheduledTask(ctx context.Context, id int) (domain.ScheduledTask, error) {
	task, err := scanScheduledTask(s.db.QueryRowContext(ctx, `SELECT id, name, task_type, library_id, cron, enabled, last_run_at, next_run_at
		FROM scheduled_tasks WHERE id = ?`, id))
	if err != nil {
		return domain.ScheduledTask{}, translateErr(err)
	}
	return task, nil
}

func (s *SQLite) CreateScheduledTask(ctx context.Context, task domain.ScheduledTask) (domain.ScheduledTask, error) {
	var lastRun any
	if task.LastRunAt != nil {
		lastRun = task.LastRunAt.UTC().Format(time.RFC3339Nano)
	}
	err := s.db.QueryRowContext(ctx, `INSERT INTO scheduled_tasks(name, task_type, library_id, cron, enabled, last_run_at, next_run_at)
		VALUES(?,?,?,?,?,?,?) RETURNING id`,
		task.Name, task.TaskType, task.LibraryID, task.Cron, boolInt(task.Enabled), lastRun, task.NextRunAt.UTC().Format(time.RFC3339Nano)).Scan(&task.ID)
	return task, err
}

func (s *SQLite) UpdateScheduledTask(ctx context.Context, task domain.ScheduledTask) error {
	var lastRun any
	if task.LastRunAt != nil {
		lastRun = task.LastRunAt.UTC().Format(time.RFC3339Nano)
	}
	result, err := s.db.ExecContext(ctx, `UPDATE scheduled_tasks SET name = ?, task_type = ?, library_id = ?, cron = ?, enabled = ?, last_run_at = ?, next_run_at = ?
		WHERE id = ?`,
		task.Name, task.TaskType, task.LibraryID, task.Cron, boolInt(task.Enabled), lastRun, task.NextRunAt.UTC().Format(time.RFC3339Nano), task.ID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLite) DeleteScheduledTask(ctx context.Context, id int) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM scheduled_tasks WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLite) DeleteScheduledTasksForLibrary(ctx context.Context, libraryID int) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM scheduled_tasks WHERE library_id = ?`, libraryID)
	return err
}

func (s *SQLite) DisableScheduledTask(ctx context.Context, id int) error {
	_, err := s.db.ExecContext(ctx, `UPDATE scheduled_tasks SET enabled = 0 WHERE id = ?`, id)
	return err
}

func (s *SQLite) DueScheduledTasks(ctx context.Context, now time.Time) ([]domain.ScheduledTask, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, task_type, library_id, cron, enabled, last_run_at, next_run_at
		FROM scheduled_tasks WHERE enabled = 1 AND next_run_at <= ? ORDER BY next_run_at`, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []domain.ScheduledTask{}
	for rows.Next() {
		task, err := scanScheduledTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s *SQLite) MarkScheduledTaskRun(ctx context.Context, id int, lastRunAt, nextRunAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE scheduled_tasks SET last_run_at = ?, next_run_at = ? WHERE id = ?`,
		lastRunAt.UTC().Format(time.RFC3339Nano), nextRunAt.UTC().Format(time.RFC3339Nano), id)
	return err
}

type scheduledTaskScanner interface {
	Scan(dest ...any) error
}

func scanScheduledTask(row scheduledTaskScanner) (domain.ScheduledTask, error) {
	var task domain.ScheduledTask
	var enabled int
	var lastRun, nextRun sql.NullString
	if err := row.Scan(&task.ID, &task.Name, &task.TaskType, &task.LibraryID, &task.Cron, &enabled, &lastRun, &nextRun); err != nil {
		return task, err
	}
	task.Enabled = enabled != 0
	if lastRun.Valid && lastRun.String != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, lastRun.String); err == nil {
			task.LastRunAt = &parsed
		}
	}
	if nextRun.Valid && nextRun.String != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, nextRun.String); err == nil {
			task.NextRunAt = parsed
		}
	}
	return task, nil
}

func (s *SQLite) jobsWhere(ctx context.Context, where string) ([]domain.BackgroundJob, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, category, type, library_id, library_name, root_path, status, paused, cancelable, current_path, processed, total, error, started_at, finished_at, options_json
		FROM background_jobs `+where+` ORDER BY started_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []domain.BackgroundJob{}
	for rows.Next() {
		var job domain.BackgroundJob
		var paused, cancelable int
		var started, finished sql.NullString
		var optionsRaw string
		if err := rows.Scan(&job.ID, &job.Category, &job.Type, &job.LibraryID, &job.LibraryName, &job.RootPath, &job.Status, &paused, &cancelable,
			&job.CurrentPath, &job.Processed, &job.Total, &job.Error, &started, &finished, &optionsRaw); err != nil {
			return nil, err
		}
		job.Paused = paused != 0
		job.Cancelable = cancelable != 0
		if started.Valid {
			if parsed, err := time.Parse(time.RFC3339Nano, started.String); err == nil {
				job.StartedAt = parsed
			}
		}
		if finished.Valid && finished.String != "" {
			if parsed, err := time.Parse(time.RFC3339Nano, finished.String); err == nil {
				job.FinishedAt = &parsed
			}
		}
		if strings.TrimSpace(optionsRaw) != "" {
			_ = json.Unmarshal([]byte(optionsRaw), &job.Options)
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// idsIN builds an `id IN (...)` predicate from a set of IDs.
func idsIN(ids map[int]bool) (string, []any) {
	values := make([]int, 0, len(ids))
	for id := range ids {
		values = append(values, id)
	}
	sort.Ints(values)
	if len(values) == 0 {
		return "NULL", nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(values)), ",")
	args := make([]any, len(values))
	for i, id := range values {
		args[i] = id
	}
	return placeholders, args
}

func (s *SQLite) FavoriteFolders(ctx context.Context, userID, viewID int) ([]domain.MediaFolder, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT mf.id, mf.parent_id, mf.path FROM favorite_folders ff JOIN media_folders mf ON mf.id = ff.folder_id WHERE ff.favorite_view_id = ?`, viewID)
	if err != nil {
		return nil, err
	}
	type rawFolder struct {
		id       int
		parentID sql.NullInt64
		path     string
	}
	var raw []rawFolder
	for rows.Next() {
		var r rawFolder
		if err := rows.Scan(&r.id, &r.parentID, &r.path); err != nil {
			rows.Close()
			return nil, err
		}
		raw = append(raw, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	roots := map[int]string{}
	out := make([]domain.MediaFolder, 0, len(raw))
	for _, r := range raw {
		f := domain.MediaFolder{ID: r.id, Path: r.path}
		if r.parentID.Valid {
			f.ParentID = int(r.parentID.Int64)
		}
		f.Name = filepath.Base(f.Path)
		root, ok := roots[f.ID]
		if !ok {
			root = s.rootPathForFolder(ctx, f.ID)
			roots[f.ID] = root
		}
		if root != "" {
			f.RelativePath = strings.TrimPrefix(strings.TrimPrefix(f.Path, root), "/")
		}
		out = append(out, f)
	}
	return out, nil
}

func (s *SQLite) SetFavoriteFolder(ctx context.Context, userID, viewID, folderID int, favorite bool) error {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM favorite_views WHERE id = ? AND user_id = ?`, viewID, userID).Scan(&exists); err != nil {
		return translateErr(err)
	}
	if favorite {
		_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO favorite_folders(favorite_view_id, folder_id) VALUES(?,?)`, viewID, folderID)
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM favorite_folders WHERE favorite_view_id = ? AND folder_id = ?`, viewID, folderID)
	return err
}
