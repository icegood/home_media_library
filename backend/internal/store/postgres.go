package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"golang.org/x/crypto/bcrypt"

	"media-library/backend/internal/domain"
)

//go:embed migrations/postgres/*.sql
var postgresMigrations embed.FS

// folderEntriesPGSQL mirrors folderEntriesSQL for Postgres. Same column order:
// entry_kind, id, parent/folder_id, path, name, mime_type, size, metadata_json,
// gps, taken_at, metadata_error, thumbnail_error, favorite.
// Query arguments: parentID, userID, parentID.
const folderEntriesPGSQL = `SELECT 'folder' AS entry_kind, f.id, f.parent_id, f.path, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, false
	FROM media_folders f WHERE f.parent_id = $1
	UNION ALL
	SELECT 'media' AS entry_kind, ` + mediaColumns + `, EXISTS(SELECT 1 FROM favorite_view_items fvi
		JOIN favorite_views fv ON fv.id = fvi.favorite_view_id
		WHERE fv.user_id = $2 AND fvi.media_id = m.id)
	FROM media m WHERE m.folder_id = $3`

// Postgres is a full database-backed implementation of Store backed by a
// Postgres database via the pgx stdlib driver. The schema mirrors the SQLite
// store; relative paths are never stored, they are computed on the fly.
type Postgres struct {
	db *sql.DB
}

func NewPostgres(dsn string) (*Postgres, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("postgres dsn is empty")
	}
	db, err := openLogged("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	// Cap connection lifetime too (same rationale as the SQLite handle): the
	// pool rotates instead of serving every query from the same connection.
	db.SetConnMaxLifetime(defaultConnMaxLifetime)
	store := &Postgres{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Postgres) Close() error {
	return s.db.Close()
}

// Vacuum runs PostgreSQL's VACUUM (ANALYZE). It is safe to run outside a
// transaction; Exec runs in autocommit so no transaction block is opened.
func (s *Postgres) Vacuum(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `VACUUM (ANALYZE)`)
	return err
}

func (s *Postgres) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return err
	}
	entries, err := fs.Glob(postgresMigrations, "migrations/postgres/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(entries)
	for _, name := range entries {
		version := filepath.Base(name)
		var applied string
		err := s.db.QueryRow(`SELECT version FROM schema_migrations WHERE version = $1`, version).Scan(&applied)
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		data, err := postgresMigrations.ReadFile(name)
		if err != nil {
			return err
		}
		if _, err := s.db.Exec(string(data)); err != nil {
			return fmt.Errorf("apply migration %s: %w", version, err)
		}
		if _, err := s.db.Exec(`INSERT INTO schema_migrations(version, applied_at) VALUES($1, $2)`,
			version, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return err
		}
	}
	return nil
}

// pgPlaceholders rewrites every '?' placeholder in query to a numbered $N
// placeholder, starting at start, keeping args in order.
func pgPlaceholders(query string, args []any, start int) (string, []any) {
	var b strings.Builder
	index := start
	rest := query
	for {
		pos := strings.Index(rest, "?")
		if pos < 0 {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:pos])
		b.WriteString("$")
		b.WriteString(strconv.Itoa(index))
		index++
		rest = rest[pos+1:]
	}
	return b.String(), args
}

// idsINPostgres is the Postgres variant of idsIN: numbered placeholders
// starting at start.
func idsINPostgres(ids map[int]bool, start int) (string, []any) {
	values := idSlice(ids)
	if len(values) == 0 {
		return "NULL", nil
	}
	parts := make([]string, len(values))
	args := make([]any, len(values))
	for i, id := range values {
		parts[i] = "$" + strconv.Itoa(start+i)
		args[i] = id
	}
	return strings.Join(parts, ","), args
}

// idSlice returns the sorted ids of a keep-set map as an int slice for passing
// to Postgres as a single array parameter.
func idSlice(ids map[int]bool) []int {
	values := make([]int, 0, len(ids))
	for id := range ids {
		values = append(values, id)
	}
	sort.Ints(values)
	return values
}

func (s *Postgres) SetupRequired(ctx context.Context) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return false, err
	}
	return count == 0, nil
}

func (s *Postgres) CreateInitialAdmin(ctx context.Context, user domain.User, password string) (domain.User, error) {
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
	if err := s.db.QueryRowContext(ctx, `INSERT INTO users(login, password_hash, role) VALUES($1,$2,$3) RETURNING id`,
		user.Login, user.PasswordHash, user.Role).Scan(&user.ID); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (s *Postgres) ServerSettings(ctx context.Context) (domain.ServerSettings, error) {
	settings := domain.DefaultServerSettings()
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT value_json FROM server_settings WHERE id = 0`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return settings, nil
	}
	if err != nil {
		return settings, err
	}
	if len(raw) != 0 {
		if err := json.Unmarshal(raw, &settings); err != nil {
			return settings, err
		}
	}
	return settings, nil
}

func (s *Postgres) SaveServerSettings(ctx context.Context, settings domain.ServerSettings) error {
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO server_settings(id, value_json) VALUES(0,$1::jsonb)
		ON CONFLICT(id) DO UPDATE SET value_json = excluded.value_json`, string(data))
	return err
}

func (s *Postgres) MIMETypeForExtension(ctx context.Context, extension string) (string, error) {
	var mimeType string
	err := s.db.QueryRowContext(ctx, `SELECT mime_type FROM media_mime_extensions WHERE extension = $1`, strings.ToLower(strings.TrimSpace(extension))).Scan(&mimeType)
	if err != nil {
		return "", translateErr(err)
	}
	return mimeType, nil
}

func (s *Postgres) UserSettings(ctx context.Context, userID int) (domain.UserSettings, error) {
	settings := domain.DefaultUserSettings()
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT value_json FROM user_settings WHERE user_id = $1`, userID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return settings, nil
	}
	if err != nil {
		return settings, err
	}
	if len(raw) != 0 {
		if err := json.Unmarshal(raw, &settings); err != nil {
			return settings, err
		}
	}
	return settings, nil
}

func (s *Postgres) SaveUserSettings(ctx context.Context, userID int, settings domain.UserSettings) error {
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO user_settings(user_id, value_json) VALUES($1,$2::jsonb)
		ON CONFLICT(user_id) DO UPDATE SET value_json = excluded.value_json`, userID, string(data))
	return err
}

func (s *Postgres) Authenticate(ctx context.Context, login, password string) (domain.User, error) {
	login = strings.ToLower(strings.TrimSpace(login))
	var user domain.User
	err := s.db.QueryRowContext(ctx, `SELECT id, login, password_hash, role, COALESCE(email, '') FROM users WHERE login = $1`, login).
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
		if _, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash = $1 WHERE id = $2`, string(hash), user.ID); err != nil {
			return user, err
		}
		user.PasswordHash = string(hash)
		return user, nil
	}
	return domain.User{}, ErrNotFound
}

func (s *Postgres) User(ctx context.Context, id int) (domain.User, error) {
	var user domain.User
	err := s.db.QueryRowContext(ctx, `SELECT id, login, password_hash, role, COALESCE(email, '') FROM users WHERE id = $1`, id).
		Scan(&user.ID, &user.Login, &user.PasswordHash, &user.Role, &user.Email)
	if err != nil {
		return user, translateErr(err)
	}
	return user, nil
}

func (s *Postgres) Users(ctx context.Context) ([]domain.User, error) {
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

func (s *Postgres) CreateUser(ctx context.Context, user domain.User, password string) (domain.User, error) {
	var err error
	user, err = normalizeUserForSave(user)
	if err != nil {
		return domain.User{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return domain.User{}, err
	}
	user.PasswordHash = string(hash)
	if err := s.db.QueryRowContext(ctx, `INSERT INTO users(login, password_hash, role) VALUES($1,$2,$3) RETURNING id`, user.Login, user.PasswordHash, user.Role).Scan(&user.ID); err != nil {
		return domain.User{}, err
	}
	return user, nil
}

func (s *Postgres) UpdateUser(ctx context.Context, user domain.User, password string) (domain.User, error) {
	var err error
	user, err = normalizeUserForSave(user)
	if err != nil {
		return domain.User{}, err
	}
	var res sql.Result
	if strings.TrimSpace(password) != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return domain.User{}, err
		}
		res, err = s.db.ExecContext(ctx, `UPDATE users SET login = $1, role = $2, password_hash = $3 WHERE id = $4`, user.Login, user.Role, string(hash), user.ID)
	} else {
		res, err = s.db.ExecContext(ctx, `UPDATE users SET login = $1, role = $2 WHERE id = $3`, user.Login, user.Role, user.ID)
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

func (s *Postgres) SetUserEmail(ctx context.Context, userID int, email string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE users SET email = $1 WHERE id = $2`, email, userID)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	}
	return rowsAffectedErr(res)
}

func (s *Postgres) UserByEmail(ctx context.Context, email string) (domain.User, error) {
	var user domain.User
	err := s.db.QueryRowContext(ctx, `SELECT id, login, password_hash, role, COALESCE(email, '') FROM users WHERE email = $1`, email).
		Scan(&user.ID, &user.Login, &user.PasswordHash, &user.Role, &user.Email)
	if err != nil {
		return user, translateErr(err)
	}
	return user, nil
}

func (s *Postgres) UpdatePassword(ctx context.Context, userID int, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash = $1 WHERE id = $2`, string(hash), userID)
	if err != nil {
		return err
	}
	return rowsAffectedErr(res)
}

func (s *Postgres) CreatePasswordResetToken(ctx context.Context, userID int, tokenHash string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO password_reset_tokens(token_hash, user_id, created_at, expires_at) VALUES($1,$2,$3,$4)`,
		tokenHash, userID, time.Now().UTC().Format(time.RFC3339), expiresAt.UTC().Format(time.RFC3339))
	return err
}

func (s *Postgres) ConsumePasswordResetToken(ctx context.Context, tokenHash string) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var userID int
	var expiresAt string
	err = tx.QueryRowContext(ctx, `SELECT user_id, expires_at FROM password_reset_tokens WHERE token_hash = $1`, tokenHash).
		Scan(&userID, &expiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	if expires, parseErr := time.Parse(time.RFC3339, expiresAt); parseErr != nil || time.Now().After(expires) {
		_, _ = tx.ExecContext(ctx, `DELETE FROM password_reset_tokens WHERE token_hash = $1`, tokenHash)
		_ = tx.Commit()
		return 0, ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM password_reset_tokens WHERE token_hash = $1`, tokenHash); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return userID, nil
}

func (s *Postgres) ImportSnapshot(ctx context.Context, snapshot domain.ImportSnapshot) (domain.ImportResult, error) {
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
			err := tx.QueryRowContext(ctx, `SELECT id, login, password_hash, role FROM users WHERE id = $1`, id).
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
			if err := tx.QueryRowContext(ctx, `INSERT INTO users(login, password_hash, role) VALUES($1,$2,$3) RETURNING id`,
				user.Login, user.PasswordHash, user.Role).Scan(&id); err != nil {
				continue
			}
			if user.PasswordHash == "" {
				password := temporaryPassword(strconv.Itoa(id), user.Login)
				hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
				if err != nil {
					return result, err
				}
				if _, err := tx.ExecContext(ctx, `UPDATE users SET password_hash = $1 WHERE id = $2`, string(hash), id); err != nil {
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
			if _, err := tx.ExecContext(ctx, `INSERT INTO users(id, login, password_hash, role) OVERRIDING SYSTEM VALUE VALUES($1,$2,$3,$4)`,
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
			if err := tx.QueryRowContext(ctx, `INSERT INTO libraries(name) VALUES($1) RETURNING id`, name).Scan(&id); err != nil {
				return result, err
			}
		} else if _, err := tx.ExecContext(ctx, `INSERT INTO libraries(id, name) OVERRIDING SYSTEM VALUE VALUES($1,$2)`, id, name); err != nil {
			continue
		}
		for _, root := range roots {
			if _, err := tx.ExecContext(ctx, `INSERT INTO library_roots(library_id, folder_id, watch) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`,
				id, root.ID, root.Watch); err != nil {
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
		if err := tx.QueryRowContext(ctx, `SELECT role FROM users WHERE id = $1`, userID).Scan(&role); err != nil {
			continue
		}
		if role == string(domain.RoleAdmin) {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO library_access(library_id, user_id) VALUES($1,$2) ON CONFLICT DO NOTHING`,
			libID, userID); err != nil {
			return result, err
		}
		result.Access = append(result.Access, domain.ImportAccess{LibraryID: libID, UserID: userID})
	}
	// Keep identity sequences ahead of any explicitly inserted ids.
	if _, err := tx.Exec(`SELECT setval(pg_get_serial_sequence('users', 'id'),
		(SELECT COALESCE(MAX(id), 0) FROM users), (SELECT COUNT(*) FROM users) > 0)`); err != nil {
		return result, err
	}
	if _, err := tx.Exec(`SELECT setval(pg_get_serial_sequence('libraries', 'id'),
		(SELECT COALESCE(MAX(id), 0) FROM libraries), (SELECT COUNT(*) FROM libraries) > 0)`); err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Postgres) loadRoots(ctx context.Context, libraryID int) []domain.LibraryRoot {
	rows, err := s.db.QueryContext(ctx, `SELECT lr.folder_id, f.path, COALESCE(lr.watch, FALSE) FROM library_roots lr
		JOIN media_folders f ON f.id = lr.folder_id
		WHERE lr.library_id = $1 ORDER BY f.path`, libraryID)
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
func (s *Postgres) WatchedRoots(ctx context.Context) ([]domain.WatchedRoot, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT lr.library_id, f.path FROM library_roots lr
		JOIN media_folders f ON f.id = lr.folder_id
		WHERE COALESCE(lr.watch, FALSE) = TRUE ORDER BY lr.library_id, f.path`)
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

func (s *Postgres) LibraryStats(ctx context.Context, libraryID int) (domain.KindStats, error) {
	var stats domain.KindStats
	query := `WITH RECURSIVE sub(id) AS (
		SELECT lr.folder_id FROM library_roots lr WHERE lr.library_id = $1
		UNION ALL
		SELECT f.id FROM media_folders f JOIN sub ON f.parent_id = sub.id
	)
	SELECT (SELECT COUNT(*) FROM media m JOIN media_mime_types mmt ON mmt.value = m.mime_type JOIN sub ON m.folder_id = sub.id WHERE mmt.media_type = 'image'),
		(SELECT COUNT(*) FROM media m JOIN media_mime_types mmt ON mmt.value = m.mime_type JOIN sub ON m.folder_id = sub.id WHERE mmt.media_type = 'video'),
		(SELECT COUNT(*) FROM media m JOIN media_mime_types mmt ON mmt.value = m.mime_type JOIN sub ON m.folder_id = sub.id WHERE mmt.media_type = 'document')`
	err := s.db.QueryRowContext(ctx, query, libraryID).
		Scan(&stats.Images, &stats.Videos, &stats.Documents)
	if err != nil {
		return domain.KindStats{}, translateErr(err)
	}
	return stats, nil
}

// FolderStats aggregates media kinds over the folder and every subfolder in
// one recursive query. FolderChain validates existence and read access.
func (s *Postgres) FolderStats(ctx context.Context, userID, libraryID, folderID int) (domain.KindStats, error) {
	if _, err := s.FolderChain(ctx, libraryID, folderID); err != nil {
		return domain.KindStats{}, err
	}
	var stats domain.KindStats
	query := `WITH RECURSIVE sub(id) AS (
		SELECT f.id FROM media_folders f WHERE f.id = $1
		UNION ALL
		SELECT f.id FROM media_folders f JOIN sub ON f.parent_id = sub.id
	)
	SELECT COALESCE(SUM(CASE WHEN mmt.media_type = 'image' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN mmt.media_type = 'video' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN mmt.media_type = 'document' THEN 1 ELSE 0 END), 0)
	FROM media m JOIN sub ON m.folder_id = sub.id
	JOIN media_mime_types mmt ON mmt.value = m.mime_type`
	if err := s.db.QueryRowContext(ctx, query, folderID).Scan(&stats.Images, &stats.Videos, &stats.Documents); err != nil {
		return domain.KindStats{}, translateErr(err)
	}
	return stats, nil
}

// FavoriteViewStats aggregates media kinds over direct mentions plus the full
// contents of favorite folders, mirroring FavoriteMediaExpanded's scope.
func (s *Postgres) FavoriteViewStats(ctx context.Context, userID, viewID int, admin bool) (domain.KindStats, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM favorite_views WHERE id = $1 AND user_id = $2`, viewID, userID).Scan(&exists); err != nil {
		return domain.KindStats{}, translateErr(err)
	}
	var stats domain.KindStats
	subSQL, subArgs := accessibleSubtree(userID, admin)
	subPG, args := pgPlaceholders(subSQL, subArgs, 1)
	favFolderPlaceholder := "$" + strconv.Itoa(len(args)+1)
	viewPlaceholder := "$" + strconv.Itoa(len(args)+2)
	flagPlaceholder := "$" + strconv.Itoa(len(args)+3)
	query := `WITH RECURSIVE sub(id) AS (` + subPG + `
		UNION ALL
		SELECT f.id FROM media_folders f JOIN sub ON f.parent_id = sub.id),
		fav_folders(id) AS (SELECT ff.folder_id FROM favorite_folders ff WHERE ff.favorite_view_id = ` + favFolderPlaceholder + `),
		folder_sub(id) AS (SELECT id FROM fav_folders UNION ALL SELECT f.id FROM media_folders f JOIN folder_sub ON f.parent_id = folder_sub.id),
		mentions(media_id) AS (
			SELECT fvi.media_id FROM favorite_view_items fvi WHERE fvi.favorite_view_id = ` + viewPlaceholder + `
			UNION ALL
			SELECT m2.id FROM media m2 JOIN folder_sub fs ON m2.folder_id = fs.id
		)
	SELECT COALESCE(SUM(CASE WHEN mmt.media_type = 'image' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN mmt.media_type = 'video' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN mmt.media_type = 'document' THEN 1 ELSE 0 END), 0)
	FROM mentions JOIN media m ON m.id = mentions.media_id
	JOIN media_mime_types mmt ON mmt.value = m.mime_type
	WHERE (` + flagPlaceholder + ` = 1 OR m.folder_id IN (SELECT id FROM sub))`
	args = append(args, viewID, viewID)
	if admin {
		args = append(args, 1)
	} else {
		args = append(args, 0)
	}
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&stats.Images, &stats.Videos, &stats.Documents); err != nil {
		return domain.KindStats{}, translateErr(err)
	}
	return stats, nil
}

func (s *Postgres) loadLibrary(ctx context.Context, id int) (domain.Library, error) {
	var library domain.Library
	err := s.db.QueryRowContext(ctx, `SELECT id, name FROM libraries WHERE id = $1`, id).
		Scan(&library.ID, &library.Name)
	if err != nil {
		return library, translateErr(err)
	}
	library.Roots = s.loadRoots(ctx, id)
	return library, nil
}

func (s *Postgres) LibrariesForUser(ctx context.Context, userID int, admin bool) ([]domain.Library, error) {
	var rows *sql.Rows
	var err error
	if admin {
		rows, err = s.db.QueryContext(ctx, `SELECT l.id, l.name, lr.folder_id, f.path, COALESCE(lr.watch, FALSE)
			FROM libraries l
			LEFT JOIN library_roots lr ON lr.library_id = l.id
			LEFT JOIN media_folders f ON f.id = lr.folder_id
			ORDER BY l.name, f.path`)
	} else {
		rows, err = s.db.QueryContext(ctx, `SELECT l.id, l.name, lr.folder_id, f.path, COALESCE(lr.watch, FALSE)
			FROM libraries l
			JOIN library_access la ON la.library_id = l.id AND la.user_id = $1
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
func (s *Postgres) fillLibraryStats(ctx context.Context, libraries map[int]*domain.Library, admin bool, userID int) error {
	if len(libraries) == 0 {
		return nil
	}
	accessFilter := ""
	args := []any{}
	if !admin {
		accessFilter = `JOIN library_access la ON la.library_id = lr.library_id AND la.user_id = $1`
		args = append(args, userID)
	}
	query := `WITH RECURSIVE tree(library_id, folder_id) AS (
		SELECT lr.library_id, lr.folder_id FROM library_roots lr ` + accessFilter + `
		UNION ALL
		SELECT tree.library_id, f.id FROM media_folders f JOIN tree ON f.parent_id = tree.folder_id
	)
	SELECT tree.library_id,
		COALESCE(SUM(CASE WHEN mmt.media_type = 'image' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN mmt.media_type = 'video' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN mmt.media_type = 'document' THEN 1 ELSE 0 END), 0)
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
		if err := rows.Scan(&id, &stats.Images, &stats.Videos, &stats.Documents); err != nil {
			return translateErr(err)
		}
		if library, ok := libraries[id]; ok {
			library.Stats = stats
		}
	}
	return rows.Err()
}

func (s *Postgres) Library(ctx context.Context, id int) (domain.Library, error) {
	return s.loadLibrary(ctx, id)
}

func (s *Postgres) Folder(ctx context.Context, id int) (domain.MediaFolder, error) {
	var folder domain.MediaFolder
	var parentID sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT id, parent_id, path FROM media_folders WHERE id = $1`, id).
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
func (s *Postgres) FolderChain(ctx context.Context, libraryID, folderID int) ([]domain.MediaFolder, error) {
	query := `WITH RECURSIVE chain(id, parent_id, path, depth) AS (
			SELECT id, parent_id, path, 0 FROM media_folders WHERE id = $1
			UNION ALL
			SELECT f.id, f.parent_id, f.path, c.depth + 1
			FROM media_folders f JOIN chain c ON f.id = c.parent_id)
		SELECT c.id, c.parent_id, c.path
		FROM chain c
		WHERE c.depth <= (SELECT MIN(depth) FROM chain
			WHERE id IN (SELECT folder_id FROM library_roots WHERE library_id = $2))
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

func (s *Postgres) FoldersByIDs(ctx context.Context, ids []int) (map[int]domain.MediaFolder, error) {
	out := map[int]domain.MediaFolder{}
	if len(ids) == 0 {
		return out, nil
	}
	unique := dedupeInts(ids)
	placeholders := make([]string, len(unique))
	args := make([]any, len(unique))
	for index, id := range unique {
		placeholders[index] = "$" + strconv.Itoa(index+1)
		args[index] = id
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, parent_id, path FROM media_folders WHERE id IN (`+strings.Join(placeholders, ", ")+`)`, args...)
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

func (s *Postgres) CanRead(ctx context.Context, userID, libraryID int, admin bool) (bool, error) {
	if admin {
		return true, nil
	}
	var ok bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM library_access WHERE library_id = $1 AND user_id = $2)`,
		libraryID, userID).Scan(&ok)
	return ok, err
}

func (s *Postgres) CanReadMedia(ctx context.Context, userID, mediaID int, admin bool) (bool, error) {
	var folderID int
	if err := s.db.QueryRowContext(ctx, `SELECT folder_id FROM media WHERE id = $1`, mediaID).Scan(&folderID); err != nil {
		return false, translateErr(err)
	}
	if admin {
		return true, nil
	}
	subSQL, subArgs := accessibleSubtree(userID, false)
	subPG, args := pgPlaceholders(subSQL, subArgs, 1)
	query := `WITH RECURSIVE sub(id) AS (` + subPG + `
		UNION ALL
		SELECT f.id FROM media_folders f JOIN sub ON f.parent_id = sub.id)
	SELECT EXISTS(SELECT 1 FROM sub WHERE id = ` + "$" + strconv.Itoa(len(args)+1) + `)`
	args = append(args, folderID)
	var ok bool
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&ok)
	return ok, err
}

func (s *Postgres) Entries(ctx context.Context, userID, libraryID int, dir string) ([]domain.Entry, error) {
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
				RelativePath: mappingName, Type: "folder",
				FolderThumbnail: mapping.ID})
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
		rows, err := s.db.QueryContext(ctx, folderEntriesPGSQL, parentID, userID, parentID)
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
	// Library-root/timeline views must surface trajectory flags just like
	// folder-scoped entries do, otherwise saved starts/ends seem to disappear.
	_ = s.enrichEntriesTrajectory(ctx, out)
	return out, nil
}

func (s *Postgres) EntriesForFolder(ctx context.Context, userID, libraryID, folderID int) (domain.FolderEntries, error) {
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

func (s *Postgres) entriesForParent(ctx context.Context, userID, parentID int, root string) ([]domain.Entry, error) {
	rows, err := s.db.QueryContext(ctx, folderEntriesPGSQL, parentID, userID, parentID)
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
	_ = s.enrichEntriesTrajectory(ctx, out)
	return out, nil
}

func (s *Postgres) descendantFolderID(ctx context.Context, rootID int, relativePath string) int {
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

func (s *Postgres) childFolderIDByLabel(ctx context.Context, parentID int, name string) int {
	rows, err := s.db.QueryContext(ctx, `SELECT id, path FROM media_folders WHERE parent_id = $1`, parentID)
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

func (s *Postgres) folderThumbnails(ctx context.Context, folderID int, limit int) []domain.ThumbnailRef {
	query := `WITH RECURSIVE sub(id) AS (
		SELECT $1::bigint
		UNION ALL
		SELECT f.id FROM media_folders f JOIN sub ON f.parent_id = sub.id)
	SELECT m.id FROM media m JOIN sub ON m.folder_id = sub.id
	JOIN media_mime_types mmt ON mmt.value = m.mime_type
	WHERE mmt.media_type <> 'document'
	ORDER BY (CASE WHEN m.gps <> '' THEN 1 ELSE 0 END)*2 + (CASE WHEN mmt.media_type = 'image' THEN 1 ELSE 0 END) +
		(CASE WHEN m.metadata_json <> '{}'::jsonb THEN 1 ELSE 0 END) DESC,
		m.path ASC
	LIMIT $2`
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

func (s *Postgres) FolderThumbnailRefs(ctx context.Context, folderID int, limit int) ([]domain.ThumbnailRef, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM media_folders WHERE id = $1`, folderID).Scan(&exists); err != nil {
		return nil, translateErr(err)
	}
	return s.folderThumbnails(ctx, folderID, limit), nil
}

func (s *Postgres) Media(ctx context.Context, id int) (domain.Media, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+mediaColumns+` FROM media m WHERE m.id = $1`, id)
	item, err := scanMedia(row)
	if err != nil {
		return item, translateErr(err)
	}
	item.RelativePath = s.relativePath(ctx, item.FolderID, item.Path)
	tmp := []domain.Media{item}
	if err := s.enrichMediaTrajectory(ctx, tmp); err == nil {
		item = tmp[0]
	}
	return item, nil
}

func (s *Postgres) MediaBatch(ctx context.Context, ids []int) ([]domain.Media, error) {
	if len(ids) == 0 {
		return []domain.Media{}, nil
	}
	unique := dedupeInts(ids)
	placeholders := make([]string, len(unique))
	args := make([]any, len(unique))
	for index, id := range unique {
		placeholders[index] = "$" + strconv.Itoa(index+1)
		args[index] = id
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+mediaColumns+` FROM media m WHERE m.id IN (`+strings.Join(placeholders, ", ")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Media{}
	for rows.Next() {
		item, err := scanMedia(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	s.attachRelativePaths(ctx, out)
	_ = s.enrichMediaTrajectory(ctx, out)
	return out, nil
}
// MediaInFolders returns every media item inside the given folders,
// including all nested subfolders.
func (s *Postgres) MediaInFolders(ctx context.Context, folderIDs []int) ([]domain.Media, error) {
	if len(folderIDs) == 0 {
		return []domain.Media{}, nil
	}
	unique := dedupeInts(folderIDs)
	args := make([]any, 0, len(unique))
	parts := make([]string, 0, len(unique))
	for i, id := range unique {
		args = append(args, id)
		parts = append(parts, fmt.Sprintf("$%d", i+1))
	}
	// Relative paths are computed against the nearest library-root ancestor so
	// archives of nested folders keep their on-disk structure.
	rootExpr := `COALESCE((SELECT rf.path FROM media_folders rf JOIN library_roots lr ON lr.folder_id = rf.id WHERE f.path LIKE rf.path || '/' || '%' OR f.path = rf.path ORDER BY length(rf.path) DESC LIMIT 1), f.path)`
	rel := relativePathExpr("m.path", "sub.root_path")
	query := `WITH RECURSIVE sub(id, root_path) AS (
		SELECT f.id, ` + rootExpr + ` FROM media_folders f WHERE f.id IN (` + strings.Join(parts, ",") + `)
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


func (s *Postgres) MediaByPath(ctx context.Context, mediaPath string) (domain.Media, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+mediaColumns+` FROM media m WHERE m.path = $1`, normalizePath(mediaPath))
	item, err := scanMedia(row)
	if err != nil {
		return item, translateErr(err)
	}
	return item, nil
}

func (s *Postgres) FavoriteViews(ctx context.Context, userID int) ([]domain.FavoriteView, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT fv.id, fv.name, COUNT(fvi.media_id)
		FROM favorite_views fv LEFT JOIN favorite_view_items fvi ON fvi.favorite_view_id = fv.id
		WHERE fv.user_id = $1 GROUP BY fv.id, fv.name ORDER BY fv.name`, userID)
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

func (s *Postgres) FavoriteViewsForMedia(ctx context.Context, userID, mediaID int) ([]domain.FavoriteViewMembership, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT fv.id, fv.name, COUNT(fvi.media_id),
		EXISTS(SELECT 1 FROM favorite_view_items fvi2 WHERE fvi2.favorite_view_id = fv.id AND fvi2.media_id = $1)
		FROM favorite_views fv LEFT JOIN favorite_view_items fvi ON fvi.favorite_view_id = fv.id
		WHERE fv.user_id = $2 GROUP BY fv.id, fv.name ORDER BY fv.name`, mediaID, userID)
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

func (s *Postgres) FavoriteViewsForFolder(ctx context.Context, userID, folderID int) ([]domain.FavoriteViewMembership, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT fv.id, fv.name, COUNT(ff.folder_id),
		EXISTS(SELECT 1 FROM favorite_folders ff2 WHERE ff2.favorite_view_id = fv.id AND ff2.folder_id = $1)
		FROM favorite_views fv LEFT JOIN favorite_folders ff ON ff.favorite_view_id = fv.id
		WHERE fv.user_id = $2 GROUP BY fv.id, fv.name ORDER BY fv.name`, folderID, userID)
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

func (s *Postgres) CreateFavoriteView(ctx context.Context, userID int, name string) (domain.FavoriteView, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.FavoriteView{}, ErrConflict
	}
	var id int
	err := s.db.QueryRowContext(ctx, `INSERT INTO favorite_views(user_id, name) VALUES($1,$2) RETURNING id`, userID, name).Scan(&id)
	if err != nil {
		return domain.FavoriteView{}, err
	}
	return domain.FavoriteView{ID: id, Name: name, Count: 0}, nil
}

func (s *Postgres) UpdateFavoriteView(ctx context.Context, userID, viewID int, name string) (domain.FavoriteView, error) {
	var existing domain.FavoriteView
	err := s.db.QueryRowContext(ctx, `SELECT id, name FROM favorite_views WHERE id = $1 AND user_id = $2`, viewID, userID).
		Scan(&existing.ID, &existing.Name)
	if err != nil {
		return existing, translateErr(err)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return existing, ErrConflict
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE favorite_views SET name = $1 WHERE id = $2`, name, viewID); err != nil {
		return existing, err
	}
	existing.Name = name
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM favorite_view_items WHERE favorite_view_id = $1`, viewID).Scan(&count); err != nil {
		return existing, err
	}
	existing.Count = count
	return existing, nil
}

func (s *Postgres) DeleteFavoriteView(ctx context.Context, userID, viewID int) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM favorite_views WHERE id = $1 AND user_id = $2`, viewID, userID)
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

func (s *Postgres) FavoriteMedia(ctx context.Context, userID, viewID int, admin bool) ([]domain.Media, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM favorite_views WHERE id = $1 AND user_id = $2`, viewID, userID).Scan(&exists); err != nil {
		return nil, translateErr(err)
	}
	subSQL, subArgs := accessibleSubtree(userID, admin)
	subPG, args := pgPlaceholders(subSQL, subArgs, 1)
	viewPlaceholder := "$" + strconv.Itoa(len(args)+1)
	flagPlaceholder := "$" + strconv.Itoa(len(args)+2)
	query := `WITH RECURSIVE sub(id) AS (` + subPG + `
		UNION ALL
		SELECT f.id FROM media_folders f JOIN sub ON f.parent_id = sub.id)
	SELECT ` + mediaColumns + ` FROM favorite_view_items fvi
	JOIN media m ON m.id = fvi.media_id
	WHERE fvi.favorite_view_id = ` + viewPlaceholder + ` AND (` + flagPlaceholder + ` = 1 OR m.folder_id IN (SELECT id FROM sub))
	ORDER BY m.name`
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
	defer rows.Close()
	out := []domain.Media{}
	for rows.Next() {
		item, err := scanMedia(rows)
		if err != nil {
			return nil, err
		}
		item.Favorite = true
		out = append(out, item)
	}
	s.attachRelativePaths(ctx, out)
	return out, rows.Err()
}

func (s *Postgres) FavoriteMediaExpanded(ctx context.Context, userID, viewID int, admin bool) ([]domain.Media, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM favorite_views WHERE id = $1 AND user_id = $2`, viewID, userID).Scan(&exists); err != nil {
		return nil, translateErr(err)
	}
	subSQL, subArgs := accessibleSubtree(userID, admin)
	subPG, args := pgPlaceholders(subSQL, subArgs, 1)
	favFolderPlaceholder := "$" + strconv.Itoa(len(args)+1)
	viewPlaceholder := "$" + strconv.Itoa(len(args)+2)
	flagPlaceholder := "$" + strconv.Itoa(len(args)+3)
	query := `WITH RECURSIVE sub(id) AS (` + subPG + `
		UNION ALL
		SELECT f.id FROM media_folders f JOIN sub ON f.parent_id = sub.id),
		fav_folders(id) AS (SELECT ff.folder_id FROM favorite_folders ff WHERE ff.favorite_view_id = ` + favFolderPlaceholder + `),
		folder_sub(id) AS (SELECT id FROM fav_folders UNION ALL SELECT f.id FROM media_folders f JOIN folder_sub ON f.parent_id = folder_sub.id),
		mentions(media_id) AS (
			SELECT fvi.media_id FROM favorite_view_items fvi WHERE fvi.favorite_view_id = ` + viewPlaceholder + `
			UNION ALL
			SELECT m2.id FROM media m2 JOIN folder_sub fs ON m2.folder_id = fs.id
		)
	SELECT ` + mediaColumns + ` FROM mentions JOIN media m ON m.id = mentions.media_id
	WHERE (` + flagPlaceholder + ` = 1 OR m.folder_id IN (SELECT id FROM sub))
	ORDER BY m.name`
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
	defer rows.Close()
	out := []domain.Media{}
	for rows.Next() {
		item, err := scanMedia(rows)
		if err != nil {
			return nil, err
		}
		item.Favorite = true
		out = append(out, item)
	}
	s.attachRelativePaths(ctx, out)
	return out, rows.Err()
}

func (s *Postgres) SetFavorite(ctx context.Context, userID, viewID, mediaID int, favorite bool) (domain.Media, error) {
	if _, err := s.Media(ctx, mediaID); err != nil {
		return domain.Media{}, err
	}
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM favorite_views WHERE id = $1 AND user_id = $2`, viewID, userID).Scan(&exists); err != nil {
		return domain.Media{}, translateErr(err)
	}
	if favorite {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO favorite_view_items(favorite_view_id, media_id) VALUES($1,$2) ON CONFLICT DO NOTHING`,
			viewID, mediaID); err != nil {
			return domain.Media{}, err
		}
	} else {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM favorite_view_items WHERE favorite_view_id = $1 AND media_id = $2`,
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

func (s *Postgres) IsFavorite(ctx context.Context, userID, mediaID int) (bool, error) {
	var ok bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM favorite_view_items fvi
		JOIN favorite_views fv ON fv.id = fvi.favorite_view_id
		WHERE fv.user_id = $1 AND fvi.media_id = $2)`, userID, mediaID).Scan(&ok)
	return ok, err
}

func (s *Postgres) FavoritesForUser(ctx context.Context, userID int, mediaIDs []int) (map[int]bool, error) {
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
		ph[i] = "$" + fmt.Sprintf("%d", i+2)
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT fvi.media_id FROM favorite_view_items fvi
		JOIN favorite_views fv ON fv.id = fvi.favorite_view_id
		WHERE fv.user_id = $1 AND fvi.media_id IN (`+strings.Join(ph, ",")+`)`, args...)
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

func (s *Postgres) UpdateGPS(ctx context.Context, id int, patch domain.GPSPatch) (domain.Media, error) {
	return s.UpdateMediaDetails(ctx, id, domain.MediaDetailsPatch{GPS: patch.GPS})
}

func (s *Postgres) UpdateMediaDetails(ctx context.Context, id int, patch domain.MediaDetailsPatch) (domain.Media, error) {
	if _, err := s.Media(ctx, id); err != nil {
		return domain.Media{}, err
	}
	sets := []string{}
	args := []any{}
	next := func() string {
		ph := "$" + strconv.Itoa(len(args)+1)
		return ph
	}
	if patch.Name != nil {
		sets = append(sets, "name = "+next())
		args = append(args, strings.TrimSpace(*patch.Name))
	}
	if patch.GPS != nil {
		sets = append(sets, "gps = "+next())
		args = append(args, strings.TrimSpace(*patch.GPS))
	}
	if patch.TakenAt != nil {
		sets = append(sets, "taken_at = "+next())
		args = append(args, strings.TrimSpace(*patch.TakenAt))
	}
	if len(sets) > 0 {
		args = append(args, id)
		if _, err := s.db.ExecContext(ctx, `UPDATE media SET `+strings.Join(sets, ", ")+` WHERE id = $`+strconv.Itoa(len(args)), args...); err != nil {
			return domain.Media{}, err
		}
	}
	return s.Media(ctx, id)
}

func (s *Postgres) SetTrajectoryStart(ctx context.Context, folderID, mediaID int, start bool) error {
	if _, err := s.Media(ctx, mediaID); err != nil {
		return err
	}
	if start {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO trajectory_starts(folder_id, media_id) VALUES($1, $2)
			ON CONFLICT(folder_id, media_id) DO NOTHING`, folderID, mediaID); err != nil {
			return err
		}
		return nil
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM trajectory_starts WHERE folder_id = $1 AND media_id = $2`, folderID, mediaID)
	return err
}

func (s *Postgres) SetTrajectoryName(ctx context.Context, folderID, mediaID int, name string) error {
	if _, err := s.Media(ctx, mediaID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO trajectory_starts(folder_id, media_id, name) VALUES($1, $2, $3)
		ON CONFLICT(folder_id, media_id) DO UPDATE SET name = excluded.name`, folderID, mediaID, name)
	return err
}

func (s *Postgres) SetTrajectoryEnd(ctx context.Context, folderID, mediaID int, end bool) error {
	if _, err := s.Media(ctx, mediaID); err != nil {
		return err
	}
	if end {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO trajectory_ends(folder_id, media_id) VALUES($1, $2)
			ON CONFLICT(folder_id, media_id) DO NOTHING`, folderID, mediaID); err != nil {
			return err
		}
		return nil
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM trajectory_ends WHERE folder_id = $1 AND media_id = $2`, folderID, mediaID)
	return err
}

func (s *Postgres) UpdateMediaMetadata(ctx context.Context, id int, metadata map[string]any, gps string, takenAt string, metadataError string, replaceTakenAt bool) error {
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	gps = strings.TrimSpace(gps)
	n := 1
	sets := []string{`metadata_json = $` + strconv.Itoa(n) + `::jsonb`}
	args := []any{string(metadataJSON)}
	n++
	sets = append(sets, `metadata_error = $`+strconv.Itoa(n))
	args = append(args, metadataError)
	n++
	if gps != "" {
		sets = append(sets, `gps = $`+strconv.Itoa(n))
		args = append(args, gps)
		n++
	}
	if takenAt != "" {
		if replaceTakenAt {
			sets = append(sets, `taken_at = $`+strconv.Itoa(n))
		} else {
			sets = append(sets, `taken_at = CASE WHEN taken_at = '' THEN $`+strconv.Itoa(n)+` ELSE taken_at END`)
		}
		args = append(args, takenAt)
		n++
	}
	args = append(args, id)
	if _, err := s.db.ExecContext(ctx, `UPDATE media SET `+strings.Join(sets, ", ")+` WHERE id = $`+strconv.Itoa(n), args...); err != nil {
		return err
	}
	return nil
}

func (s *Postgres) bulkTargetSubPG(ids []int, folderIDs []int) (cte, targetSub string, args []any) {
	n := 0
	if len(folderIDs) > 0 {
		folderPh := make([]string, len(folderIDs))
		folderArgs := make([]any, len(folderIDs))
		for i, fid := range folderIDs {
			n++
			folderPh[i] = "$" + strconv.Itoa(n)
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
			n++
			idPh[i] = "$" + strconv.Itoa(n)
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

func (s *Postgres) BulkUpdateMediaGPS(ctx context.Context, ids []int, folderIDs []int, gps string, lat, lng float64) ([]domain.BulkMediaResult, error) {
	cte, targetSub, args := s.bulkTargetSubPG(ids, folderIDs)
	if targetSub == "" {
		return []domain.BulkMediaResult{}, nil
	}
	n := len(args)
	setClause := "gps = $" + strconv.Itoa(n+1) + ", gps_lat = $" + strconv.Itoa(n+2) + ", gps_lng = $" + strconv.Itoa(n+3)
	fullArgs := append(args, gps, lat, lng)
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

func (s *Postgres) BulkUpdateMediaSetTime(ctx context.Context, ids []int, folderIDs []int, takenAt string) ([]domain.BulkMediaResult, error) {
	cte, targetSub, args := s.bulkTargetSubPG(ids, folderIDs)
	if targetSub == "" {
		return []domain.BulkMediaResult{}, nil
	}
	n := len(args)
	setClause := "taken_at = $" + strconv.Itoa(n+1)
	fullArgs := append(args, takenAt)
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

func (s *Postgres) BulkUpdateMediaShiftTime(ctx context.Context, ids []int, folderIDs []int, shiftMinutes float64) ([]domain.BulkMediaResult, error) {
	cte, targetSub, args := s.bulkTargetSubPG(ids, folderIDs)
	if targetSub == "" {
		return []domain.BulkMediaResult{}, nil
	}
	setClause := "taken_at = CASE WHEN taken_at = '' THEN taken_at ELSE (to_char((taken_at::timestamptz + interval '" + strconv.FormatFloat(shiftMinutes, 'f', -1, 64) + " minutes'), 'YYYY-MM-DD\"T\"HH24:MI:SS') || 'Z') END"
	if _, err := s.db.ExecContext(ctx, cte+`UPDATE media SET `+setClause+` WHERE id IN (`+targetSub+`)`, args...); err != nil {
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

func (s *Postgres) queryMediaByIDs(ctx context.Context, idCond string, idArgs []any) ([]domain.Media, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+mediaColumns+` FROM media m WHERE m.id IN (`+idCond+`)`, idArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Media
	for rows.Next() {
		item, err := scanMedia(rows)
		if err != nil {
			return nil, err
		}
		item.RelativePath = s.relativePath(ctx, item.FolderID, item.Path)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Postgres) GeotaggedMedia(ctx context.Context, userID int, admin bool, libraryID, folderID int) ([]domain.MapMedia, error) {
	var query string
	switch {
	case folderID > 0:
		query = `WITH RECURSIVE covers(folder_id, library_id) AS (
			SELECT $1::int, $2::int
			UNION ALL
			SELECT f.id, covers.library_id FROM media_folders f JOIN covers ON f.parent_id = covers.folder_id)
		SELECT ` + mediaColumns + `, MIN(covers.library_id)
		FROM media m JOIN covers ON covers.folder_id = m.folder_id
		WHERE m.gps <> '' AND ($3 = 1 OR EXISTS(SELECT 1 FROM library_access la WHERE la.library_id = covers.library_id AND la.user_id = $4))
		GROUP BY m.id`
	case libraryID > 0:
		query = `WITH RECURSIVE covers(folder_id, library_id) AS (
			SELECT lr.folder_id, lr.library_id FROM library_roots lr WHERE lr.library_id = $1::int
			UNION ALL
			SELECT f.id, covers.library_id FROM media_folders f JOIN covers ON f.parent_id = covers.folder_id)
		SELECT ` + mediaColumns + `, MIN(covers.library_id)
		FROM media m JOIN covers ON covers.folder_id = m.folder_id
		WHERE m.gps <> '' AND ($2 = 1 OR EXISTS(SELECT 1 FROM library_access la WHERE la.library_id = covers.library_id AND la.user_id = $3))
		GROUP BY m.id`
	default:
		query = `WITH RECURSIVE covers(folder_id, library_id) AS (
			SELECT lr.folder_id, lr.library_id FROM library_roots lr
			UNION ALL
			SELECT f.id, covers.library_id FROM media_folders f JOIN covers ON f.parent_id = covers.folder_id)
		SELECT ` + mediaColumns + `, MIN(covers.library_id)
		FROM media m JOIN covers ON covers.folder_id = m.folder_id
		WHERE m.gps <> '' AND ($1 = 1 OR EXISTS(SELECT 1 FROM library_access la WHERE la.library_id = covers.library_id AND la.user_id = $2))
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
	defer rows.Close()
	out := []domain.MapMedia{}
	roots := map[int]string{}
	for rows.Next() {
		var libraryID int
		item, err := scanMedia(rows, &libraryID)
		if err != nil {
			return nil, err
		}
		root, ok := roots[item.FolderID]
		if !ok {
			root = s.rootPathForFolder(ctx, item.FolderID)
			roots[item.FolderID] = root
		}
		if root != "" {
			item.RelativePath = strings.TrimPrefix(strings.TrimPrefix(item.Path, root), "/")
		}
		out = append(out, domain.MapMedia{Media: item, LibraryID: libraryID})
	}
	if err := s.enrichMapMediaTrajectory(ctx, out); err != nil {
		return nil, err
	}
	return out, rows.Err()
}

// MediaInArea returns geotagged media the user may read whose point falls inside
// bounds. The rectangle test uses PostGIS's && operator on the generated geom
// column; the GiST index media_geom_idx serves the lookup.
func (s *Postgres) MediaInArea(ctx context.Context, userID int, admin bool, libraryID, folderID int, bounds domain.Bounds) ([]domain.MapMedia, error) {
	var query string
	switch {
	case folderID > 0:
		query = `WITH RECURSIVE covers(folder_id, library_id) AS (
			SELECT $1::int, $2::int
			UNION ALL
			SELECT f.id, covers.library_id FROM media_folders f JOIN covers ON f.parent_id = covers.folder_id)
		SELECT ` + mediaColumns + `, MIN(covers.library_id)
		FROM media m JOIN covers ON covers.folder_id = m.folder_id
		WHERE m.geom && ST_MakeEnvelope($3, $4, $5, $6, 4326)
			AND ($7 = 1 OR EXISTS(SELECT 1 FROM library_access la WHERE la.library_id = covers.library_id AND la.user_id = $8))
		GROUP BY m.id`
	case libraryID > 0:
		query = `WITH RECURSIVE covers(folder_id, library_id) AS (
			SELECT lr.folder_id, lr.library_id FROM library_roots lr WHERE lr.library_id = $1::int
			UNION ALL
			SELECT f.id, covers.library_id FROM media_folders f JOIN covers ON f.parent_id = covers.folder_id)
		SELECT ` + mediaColumns + `, MIN(covers.library_id)
		FROM media m JOIN covers ON covers.folder_id = m.folder_id
		WHERE m.geom && ST_MakeEnvelope($2, $3, $4, $5, 4326)
			AND ($6 = 1 OR EXISTS(SELECT 1 FROM library_access la WHERE la.library_id = covers.library_id AND la.user_id = $7))
		GROUP BY m.id`
	default:
		query = `WITH RECURSIVE covers(folder_id, library_id) AS (
			SELECT lr.folder_id, lr.library_id FROM library_roots lr
			UNION ALL
			SELECT f.id, covers.library_id FROM media_folders f JOIN covers ON f.parent_id = covers.folder_id)
		SELECT ` + mediaColumns + `, MIN(covers.library_id)
		FROM media m JOIN covers ON covers.folder_id = m.folder_id
		WHERE m.geom && ST_MakeEnvelope($1, $2, $3, $4, 4326)
			AND ($5 = 1 OR EXISTS(SELECT 1 FROM library_access la WHERE la.library_id = covers.library_id AND la.user_id = $6))
		GROUP BY m.id`
	}
	args := []any{}
	switch {
	case folderID > 0:
		args = append(args, folderID, libraryID)
	case libraryID > 0:
		args = append(args, libraryID)
	}
	args = append(args, bounds.West, bounds.South, bounds.East, bounds.North)
	if admin {
		args = append(args, 1, 0)
	} else {
		args = append(args, 0, userID)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.MapMedia{}
	roots := map[int]string{}
	for rows.Next() {
		var libraryID int
		item, err := scanMedia(rows, &libraryID)
		if err != nil {
			return nil, err
		}
		root, ok := roots[item.FolderID]
		if !ok {
			root = s.rootPathForFolder(ctx, item.FolderID)
			roots[item.FolderID] = root
		}
		if root != "" {
			item.RelativePath = strings.TrimPrefix(strings.TrimPrefix(item.Path, root), "/")
		}
		out = append(out, domain.MapMedia{Media: item, LibraryID: libraryID})
	}
	if err := s.enrichMapMediaTrajectory(ctx, out); err != nil {
		return nil, err
	}
	return out, rows.Err()
}

// enrichMediaTrajectory applies trajectory start/end flags and names to a batch
// of media rows. It matches only rows whose own folder_id owns the marker, so
// the same media can carry different markers in different folders.
func (s *Postgres) enrichMediaTrajectory(ctx context.Context, items []domain.Media) error {
	if len(items) == 0 {
		return nil
	}
	const batchSize = 400
	idToIdx := make(map[int]int, len(items))
	for i, m := range items {
		idToIdx[m.ID] = i
	}
	for start := 0; start < len(items); start += batchSize {
		end := start + batchSize
		if end > len(items) {
			end = len(items)
		}
		ids := make([]any, end-start)
		for i, m := range items[start:end] {
			ids[i] = m.ID
		}
		placeholders := make([]string, len(ids))
		for i := range ids {
			placeholders[i] = "$" + strconv.Itoa(i+1)
		}
		in := strings.Join(placeholders, ", ")
		rows, err := s.db.QueryContext(ctx, `SELECT folder_id, media_id, name FROM trajectory_starts WHERE media_id IN (`+in+`)`, ids...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var fid, mid int
			var name string
			if err := rows.Scan(&fid, &mid, &name); err != nil {
				rows.Close()
				return err
			}
			if idx, ok := idToIdx[mid]; ok && items[idx].FolderID == fid {
				items[idx].TrajectoryStart = true
				items[idx].TrajectoryName = name
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		rows, err = s.db.QueryContext(ctx, `SELECT folder_id, media_id FROM trajectory_ends WHERE media_id IN (`+in+`)`, ids...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var fid, mid int
			if err := rows.Scan(&fid, &mid); err != nil {
				rows.Close()
				return err
			}
			if idx, ok := idToIdx[mid]; ok && items[idx].FolderID == fid {
				items[idx].TrajectoryEnd = true
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}
	return nil
}

// enrichEntriesTrajectory applies trajectory flags to the media rows inside an
// Entry slice (library-root/timeline and folder-scoped listings alike).
func (s *Postgres) enrichEntriesTrajectory(ctx context.Context, out []domain.Entry) error {
	var medias []domain.Media
	for _, e := range out {
		if e.Media != nil {
			medias = append(medias, *e.Media)
		}
	}
	if len(medias) == 0 {
		return nil
	}
	if err := s.enrichMediaTrajectory(ctx, medias); err != nil {
		return err
	}
	mediaIdx := 0
	for i := range out {
		if out[i].Media == nil {
			continue
		}
		m := medias[mediaIdx]
		mediaIdx++
		if m.ID != out[i].Media.ID {
			continue
		}
		out[i].Media.TrajectoryStart = m.TrajectoryStart
		out[i].Media.TrajectoryEnd = m.TrajectoryEnd
		out[i].Media.TrajectoryName = m.TrajectoryName
	}
	return nil
}

// enrichMapMediaTrajectory applies trajectory flags to geotagged map items so
// library-level and global maps draw the same segments as folder-scoped ones.
func (s *Postgres) enrichMapMediaTrajectory(ctx context.Context, out []domain.MapMedia) error {
	medias := make([]domain.Media, len(out))
	for i, m := range out {
		medias[i] = m.Media
	}
	if err := s.enrichMediaTrajectory(ctx, medias); err != nil {
		return err
	}
	for i := range out {
		out[i].Media = medias[i]
		// MapMedia re-declares the trajectory fields at the top level (they
		// shadow the embedded Media ones in JSON), so carry the flags over.
		out[i].TrajectoryStart = medias[i].TrajectoryStart
		out[i].TrajectoryEnd = medias[i].TrajectoryEnd
		out[i].TrajectoryName = medias[i].TrajectoryName
	}
	return nil
}

func (s *Postgres) MediaForLibrary(ctx context.Context, userID, libraryID int) ([]domain.Media, error) {
	if _, err := s.loadLibrary(ctx, libraryID); err != nil {
		return nil, err
	}
	rel := relativePathExpr("m.path", "covers.root_path")
	query := `WITH RECURSIVE covers(folder_id, root_path) AS (
		SELECT lr.folder_id, f.path FROM library_roots lr JOIN media_folders f ON f.id = lr.folder_id WHERE lr.library_id = $1
		UNION ALL
		SELECT f.id, covers.root_path FROM media_folders f JOIN covers ON f.parent_id = covers.folder_id)
	SELECT ` + mediaColumns + `, ` + rel + ` AS relative_path,
		EXISTS(SELECT 1 FROM favorite_view_items fvi
			JOIN favorite_views fv ON fv.id = fvi.favorite_view_id
			WHERE fv.user_id = $2 AND fvi.media_id = m.id)
	FROM media m JOIN covers ON covers.folder_id = m.folder_id
	ORDER BY relative_path`
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
	_ = s.enrichMediaTrajectory(ctx, out)
	return out, rows.Err()
}

func (s *Postgres) MediaForFolder(ctx context.Context, userID, libraryID, folderID int) ([]domain.Media, error) {
	if _, err := s.FolderChain(ctx, libraryID, folderID); err != nil {
		return nil, err
	}
	rel := relativePathExpr("m.path", "covers.root_path")
	query := `WITH RECURSIVE covers(folder_id, root_path) AS (
		SELECT f.id, f.path FROM media_folders f WHERE f.id = $1
		UNION ALL
		SELECT f.id, covers.root_path FROM media_folders f JOIN covers ON f.parent_id = covers.folder_id)
	SELECT ` + mediaColumns + `, ` + rel + ` AS relative_path,
		EXISTS(SELECT 1 FROM favorite_view_items fvi
			JOIN favorite_views fv ON fv.id = fvi.favorite_view_id
			WHERE fv.user_id = $2 AND fvi.media_id = m.id)
	FROM media m JOIN covers ON covers.folder_id = m.folder_id
	ORDER BY relative_path`
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
	_ = s.enrichMediaTrajectory(ctx, out)
	return out, rows.Err()
}

func (s *Postgres) FoldersForLibrary(ctx context.Context, libraryID int) ([]domain.MediaFolder, error) {
	if _, err := s.loadLibrary(ctx, libraryID); err != nil {
		return nil, err
	}
	rel := relativePathExpr("path", "root_path")
	rows, err := s.db.QueryContext(ctx, `WITH RECURSIVE covers(id, parent_id, path, root_path) AS (
		SELECT f.id, COALESCE(f.parent_id, -1), f.path, f.path FROM library_roots lr JOIN media_folders f ON f.id = lr.folder_id WHERE lr.library_id = $1
		UNION
		SELECT f.id, COALESCE(f.parent_id, -1), f.path, covers.root_path FROM media_folders f JOIN covers ON f.parent_id = covers.id)
		SELECT DISTINCT id, parent_id, path, `+rel+` AS relative_path FROM covers ORDER BY path`, libraryID)
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

func (s *Postgres) ThumbnailCleanupRefsForLibrary(ctx context.Context, libraryID int) (domain.ThumbnailCleanupRefs, error) {
	if _, err := s.loadLibrary(ctx, libraryID); err != nil {
		return domain.ThumbnailCleanupRefs{}, err
	}
	covers := `WITH RECURSIVE covers(folder_id) AS (
		SELECT lr.folder_id FROM library_roots lr WHERE lr.library_id = $1
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

func (s *Postgres) SetMediaActionError(ctx context.Context, id int, action, message string) error {
	column := ""
	switch action {
	case "metadata":
		column = "metadata_error"
	case "thumbnail":
		column = "thumbnail_error"
	default:
		return ErrNotFound
	}
	query := `UPDATE media SET ` + column + ` = $1 WHERE id = $2`
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

func (s *Postgres) PruneFolder(ctx context.Context, rootFolderID int, keepFolders, keepMedia map[int]bool) error {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM media_folders WHERE id = $1`, rootFolderID).Scan(&exists); err != nil {
		return translateErr(err)
	}
	subtree := `WITH RECURSIVE sub(id) AS (
		SELECT $1::bigint
		UNION ALL
		SELECT f.id FROM media_folders f JOIN sub ON f.parent_id = sub.id)`
	// Pass the keep-sets as single array parameters and unnest them instead of
	// building an unbounded IN (...) list, so pruning a very large root never
	// trips the server's bound-variable limit.
	mediaKeep := idSlice(keepMedia)
	folderKeep := idSlice(keepFolders)
	if _, err := s.db.ExecContext(ctx, subtree+` DELETE FROM media
		WHERE folder_id IN (SELECT id FROM sub)
		  AND id NOT IN (SELECT unnest($2::bigint[]))`,
		rootFolderID, mediaKeep); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, subtree+` DELETE FROM media_folders
		WHERE id IN (SELECT id FROM sub) AND id <> $2
		  AND id NOT IN (SELECT unnest($3::bigint[]))
		  AND id NOT IN (SELECT DISTINCT folder_id FROM media)`,
		rootFolderID, rootFolderID, folderKeep); err != nil {
		return err
	}
	return nil
}

func (s *Postgres) DeleteLibrary(ctx context.Context, id int) error {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM libraries WHERE id = $1`, id).Scan(&exists); err != nil {
		return translateErr(err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM libraries WHERE id = $1`, id); err != nil {
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

func (s *Postgres) CreateLibrary(ctx context.Context, library domain.Library) (domain.Library, error) {
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
	var id int
	if err := s.db.QueryRowContext(ctx, `INSERT INTO libraries(name) VALUES($1) RETURNING id`,
		name).Scan(&id); err != nil {
		return domain.Library{}, err
	}
	for _, root := range roots {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO library_roots(library_id, folder_id, watch) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`,
			id, root.ID, root.Watch); err != nil {
			return domain.Library{}, err
		}
	}
	library.ID = id
	library.Name = name
	library.Roots = roots
	return library, nil
}

func (s *Postgres) UpdateLibrary(ctx context.Context, library domain.Library) error {
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
	if _, err := s.db.ExecContext(ctx, `UPDATE libraries SET name = $1 WHERE id = $2`,
		name, library.ID); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM library_roots WHERE library_id = $1`, library.ID); err != nil {
		return err
	}
	for _, root := range roots {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO library_roots(library_id, folder_id, watch) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`,
			library.ID, root.ID, root.Watch); err != nil {
			return err
		}
	}
	return nil
}

func (s *Postgres) libraryNameExists(ctx context.Context, name string, exceptID int) bool {
	var id int
	err := s.db.QueryRowContext(ctx, `SELECT id FROM libraries WHERE lower(name) = lower($1) AND id <> $2 LIMIT 1`,
		name, exceptID).Scan(&id)
	return err == nil
}

func (s *Postgres) ensureRoots(ctx context.Context, roots []domain.LibraryRoot) ([]domain.LibraryRoot, error) {
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
		out = append(out, domain.LibraryRoot{ID: folderID, Path: root.Path})
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

func (s *Postgres) ensureFolderByPath(ctx context.Context, path string) (int, error) {
	var id int
	err := s.db.QueryRowContext(ctx, `SELECT id FROM media_folders WHERE path = $1`, path).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	err = s.db.QueryRowContext(ctx, `INSERT INTO media_folders(parent_id, path) VALUES(NULL, $1) ON CONFLICT(path) DO NOTHING RETURNING id`, path).Scan(&id)
	if err == nil {
		return id, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		err = s.db.QueryRowContext(ctx, `SELECT id FROM media_folders WHERE path = $1`, path).Scan(&id)
		return id, err
	}
	return 0, err
}

func (s *Postgres) ensureFolderByPathTx(ctx context.Context, tx *sql.Tx, path string) (int, error) {
	var id int
	err := tx.QueryRowContext(ctx, `SELECT id FROM media_folders WHERE path = $1`, path).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO media_folders(parent_id, path) VALUES(NULL, $1) ON CONFLICT(path) DO NOTHING RETURNING id`, path).Scan(&id)
	if err == nil {
		return id, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, `SELECT id FROM media_folders WHERE path = $1`, path).Scan(&id)
		return id, err
	}
	return 0, err
}

func (s *Postgres) SetAccess(ctx context.Context, libraryID, userID int, allowed bool) error {
	if _, err := s.Library(ctx, libraryID); err != nil {
		return err
	}
	if _, err := s.User(ctx, userID); err != nil {
		return err
	}
	if allowed {
		_, err := s.db.ExecContext(ctx, `INSERT INTO library_access(library_id, user_id) VALUES($1,$2) ON CONFLICT DO NOTHING`,
			libraryID, userID)
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM library_access WHERE library_id = $1 AND user_id = $2`, libraryID, userID)
	return err
}

func (s *Postgres) LibraryAccess(ctx context.Context, libraryID int) ([]domain.LibraryUserAccess, error) {
	if _, err := s.Library(ctx, libraryID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT u.id, u.login, u.password_hash, u.role,
		(u.role = 'admin' OR la.user_id IS NOT NULL) AS allowed
		FROM users u LEFT JOIN library_access la ON la.user_id = u.id AND la.library_id = $1
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

func (s *Postgres) UpsertFolder(ctx context.Context, folder domain.MediaFolder) (domain.MediaFolder, error) {
	folder.Path = normalizePath(folder.Path)
	var id int
	if folder.Path != "" {
		err := s.db.QueryRowContext(ctx, `SELECT id FROM media_folders WHERE path = $1`, folder.Path).Scan(&id)
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
	if folder.ID != domain.InvalidID {
		if _, err := s.db.ExecContext(ctx, `UPDATE media_folders SET parent_id = $1 WHERE id = $2`,
			parentIDOrNull(parentID), folder.ID); err != nil {
			return folder, err
		}
		folder.ParentID = parentID
		return folder, nil
	}
	var newID int
	err := s.db.QueryRowContext(ctx, `INSERT INTO media_folders(parent_id, path) VALUES($1,$2) RETURNING id`,
		parentIDOrNull(parentID), folder.Path).Scan(&newID)
	if err != nil {
		return folder, err
	}
	folder.ID = newID
	folder.ParentID = parentID
	return folder, nil
}

func (s *Postgres) folderIDByPath(ctx context.Context, path string) (int, error) {
	var id int
	err := s.db.QueryRowContext(ctx, `SELECT id FROM media_folders WHERE path = $1`, normalizePath(path)).Scan(&id)
	if err != nil {
		return domain.InvalidID, err
	}
	return id, nil
}

func (s *Postgres) UpsertMedia(ctx context.Context, media domain.Media) (domain.Media, error) {
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
	if folderID == domain.InvalidID {
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
	if media.ID == domain.InvalidID {
		var newID int
		err := s.db.QueryRowContext(ctx, `INSERT INTO media(folder_id, path, name, mime_type, size, metadata_json, gps, taken_at, metadata_error, thumbnail_error)
			VALUES($1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9,$10) RETURNING id`,
			folderID, media.Path, media.Name, media.MIMEType, media.Size,
			string(metadataJSON), media.GPS, media.TakenAt, media.MetadataError, media.ThumbnailError).Scan(&newID)
		if err != nil {
			return media, err
		}
		media.ID = newID
	} else {
		if _, err := s.db.ExecContext(ctx, `UPDATE media SET folder_id = $1, name = $2, mime_type = $3, size = $4, metadata_json = $5::jsonb, gps = $6, taken_at = $7, metadata_error = $8, thumbnail_error = $9 WHERE id = $10`,
			folderID, media.Name, media.MIMEType, media.Size,
			string(metadataJSON), media.GPS, media.TakenAt, media.MetadataError, media.ThumbnailError, media.ID); err != nil {
			return media, err
		}
	}
	media.FolderID = folderID
	return media, nil
}

func (s *Postgres) mediaIDByPath(ctx context.Context, mediaPath string) (int, error) {
	var id int
	err := s.db.QueryRowContext(ctx, `SELECT id FROM media WHERE path = $1`, normalizePath(mediaPath)).Scan(&id)
	if err != nil {
		return domain.InvalidID, err
	}
	return id, nil
}

func (s *Postgres) mediaByPath(ctx context.Context, mediaPath string) (domain.Media, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+mediaColumns+` FROM media m WHERE m.path = $1`, normalizePath(mediaPath))
	return scanMedia(row)
}

func (s *Postgres) Thumbnail(ctx context.Context, mediaID int, index int) (domain.Thumbnail, error) {
	var thumbnail domain.Thumbnail
	err := s.db.QueryRowContext(ctx, `SELECT media_id, thumbnail_index, mime_type FROM thumbnails WHERE media_id = $1 AND thumbnail_index = $2`,
		mediaID, index).Scan(&thumbnail.MediaID, &thumbnail.Index, &thumbnail.MIMEType)
	if err != nil {
		return thumbnail, translateErr(err)
	}
	return thumbnail, nil
}

func (s *Postgres) FolderThumbnailFile(ctx context.Context, folderID int) (domain.FolderThumbnail, error) {
	var thumbnail domain.FolderThumbnail
	err := s.db.QueryRowContext(ctx, `SELECT folder_id, mime_type FROM folder_thumbnail_files WHERE folder_id = $1`, folderID).Scan(&thumbnail.FolderID, &thumbnail.MIMEType)
	if err != nil {
		return thumbnail, translateErr(err)
	}
	return thumbnail, nil
}

func (s *Postgres) UpsertFolderThumbnail(ctx context.Context, thumbnail domain.FolderThumbnail) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO folder_thumbnail_files(folder_id, mime_type) VALUES($1,$2)
		ON CONFLICT(folder_id) DO UPDATE SET mime_type = excluded.mime_type`, thumbnail.FolderID, thumbnail.MIMEType); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM folder_thumbnails WHERE folder_id = $1`, thumbnail.FolderID); err != nil {
		return err
	}
	for index, ref := range thumbnail.Sources {
		if _, err := tx.ExecContext(ctx, `INSERT INTO folder_thumbnails(folder_id, thumbnail_index, source_media_id) VALUES($1,$2,$3)`,
			thumbnail.FolderID, index, ref.MediaID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Postgres) UpsertThumbnail(ctx context.Context, thumbnail domain.Thumbnail) error {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM media WHERE id = $1`, thumbnail.MediaID).Scan(&exists); err != nil {
		return translateErr(err)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO thumbnails(media_id, thumbnail_index, mime_type) VALUES($1,$2,$3)
		ON CONFLICT(media_id, thumbnail_index) DO UPDATE SET mime_type = excluded.mime_type`,
		thumbnail.MediaID, thumbnail.Index, thumbnail.MIMEType)
	return err
}

func (s *Postgres) SaveJob(ctx context.Context, job domain.BackgroundJob) error {
	options, err := json.Marshal(job.Options)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO background_jobs(id, category, type, library_id, library_name, root_path, status, paused, cancelable, current_path, processed, total, error, started_at, finished_at, options_json)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16::jsonb)
		ON CONFLICT(id) DO UPDATE SET category = excluded.category, type = excluded.type, library_id = excluded.library_id, library_name = excluded.library_name,
			root_path = excluded.root_path, status = excluded.status, paused = excluded.paused, cancelable = excluded.cancelable,
			current_path = excluded.current_path, processed = excluded.processed, total = excluded.total, error = excluded.error,
			started_at = excluded.started_at, finished_at = excluded.finished_at, options_json = excluded.options_json`,
		job.ID, job.Category, job.Type, job.LibraryID, job.LibraryName, job.RootPath, job.Status, job.Paused, job.Cancelable,
		job.CurrentPath, job.Processed, job.Total, job.Error, job.StartedAt.UTC(), job.FinishedAt, string(options))
	return err
}

func (s *Postgres) Jobs(ctx context.Context) ([]domain.BackgroundJob, error) {
	return s.jobsWhere(ctx, "")
}

func (s *Postgres) UnfinishedJobs(ctx context.Context) ([]domain.BackgroundJob, error) {
	return s.jobsWhere(ctx, `WHERE status IN ('running', 'paused', 'cancelling')`)
}

func (s *Postgres) DeleteFinishedJobsBefore(ctx context.Context, before time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM background_jobs WHERE finished_at IS NOT NULL AND finished_at < $1`, before.UTC())
	return err
}

func (s *Postgres) ScheduledTasks(ctx context.Context) ([]domain.ScheduledTask, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, task_type, library_id, cron, enabled, last_run_at, next_run_at
		FROM scheduled_tasks ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []domain.ScheduledTask{}
	for rows.Next() {
		task, err := scanPostgresScheduledTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s *Postgres) ScheduledTask(ctx context.Context, id int) (domain.ScheduledTask, error) {
	task, err := scanPostgresScheduledTask(s.db.QueryRowContext(ctx, `SELECT id, name, task_type, library_id, cron, enabled, last_run_at, next_run_at
		FROM scheduled_tasks WHERE id = $1`, id))
	if err != nil {
		return domain.ScheduledTask{}, translateErr(err)
	}
	return task, nil
}

func (s *Postgres) CreateScheduledTask(ctx context.Context, task domain.ScheduledTask) (domain.ScheduledTask, error) {
	err := s.db.QueryRowContext(ctx, `INSERT INTO scheduled_tasks(name, task_type, library_id, cron, enabled, last_run_at, next_run_at)
		VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		task.Name, task.TaskType, task.LibraryID, task.Cron, task.Enabled, task.LastRunAt, task.NextRunAt.UTC()).Scan(&task.ID)
	return task, err
}

func (s *Postgres) UpdateScheduledTask(ctx context.Context, task domain.ScheduledTask) error {
	result, err := s.db.ExecContext(ctx, `UPDATE scheduled_tasks SET name = $1, task_type = $2, library_id = $3, cron = $4, enabled = $5, last_run_at = $6, next_run_at = $7
		WHERE id = $8`,
		task.Name, task.TaskType, task.LibraryID, task.Cron, task.Enabled, task.LastRunAt, task.NextRunAt.UTC(), task.ID)
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

func (s *Postgres) DeleteScheduledTask(ctx context.Context, id int) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM scheduled_tasks WHERE id = $1`, id)
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

func (s *Postgres) DeleteScheduledTasksForLibrary(ctx context.Context, libraryID int) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM scheduled_tasks WHERE library_id = $1`, libraryID)
	return err
}

func (s *Postgres) DisableScheduledTask(ctx context.Context, id int) error {
	_, err := s.db.ExecContext(ctx, `UPDATE scheduled_tasks SET enabled = false WHERE id = $1`, id)
	return err
}

func (s *Postgres) DueScheduledTasks(ctx context.Context, now time.Time) ([]domain.ScheduledTask, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, task_type, library_id, cron, enabled, last_run_at, next_run_at
		FROM scheduled_tasks WHERE enabled = true AND next_run_at <= $1 ORDER BY next_run_at`, now.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tasks := []domain.ScheduledTask{}
	for rows.Next() {
		task, err := scanPostgresScheduledTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s *Postgres) MarkScheduledTaskRun(ctx context.Context, id int, lastRunAt, nextRunAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE scheduled_tasks SET last_run_at = $1, next_run_at = $2 WHERE id = $3`,
		lastRunAt.UTC(), nextRunAt.UTC(), id)
	return err
}

func scanPostgresScheduledTask(row interface{ Scan(...any) error }) (domain.ScheduledTask, error) {
	var task domain.ScheduledTask
	if err := row.Scan(&task.ID, &task.Name, &task.TaskType, &task.LibraryID, &task.Cron, &task.Enabled, &task.LastRunAt, &task.NextRunAt); err != nil {
		return task, err
	}
	return task, nil
}

func (s *Postgres) jobsWhere(ctx context.Context, where string) ([]domain.BackgroundJob, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, category, type, library_id, library_name, root_path, status, paused, cancelable, current_path, processed, total, error, started_at, finished_at, options_json
		FROM background_jobs `+where+` ORDER BY started_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []domain.BackgroundJob{}
	for rows.Next() {
		var job domain.BackgroundJob
		var optionsRaw []byte
		if err := rows.Scan(&job.ID, &job.Category, &job.Type, &job.LibraryID, &job.LibraryName, &job.RootPath, &job.Status, &job.Paused, &job.Cancelable,
			&job.CurrentPath, &job.Processed, &job.Total, &job.Error, &job.StartedAt, &job.FinishedAt, &optionsRaw); err != nil {
			return nil, err
		}
		if len(optionsRaw) != 0 {
			_ = json.Unmarshal(optionsRaw, &job.Options)
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// rootPathForFolder returns the path of the nearest library-root ancestor of a
// folder, or "" when the folder is not beneath any library root. Stored folder
// paths are canonical absolute paths built from the resolved root, so the
// nearest root is simply the library root whose path is the longest prefix of
// the folder's own path; no ancestor walk is needed.
func (s *Postgres) rootPathForFolder(ctx context.Context, folderID int) string {
	query := `SELECT root.path
		FROM library_roots lr
		JOIN media_folders root ON root.id = lr.folder_id
		JOIN media_folders folder ON folder.id = $1
		WHERE root.path = folder.path
		   OR substr(folder.path, 1, length(root.path) + 1) = root.path || '/'
		ORDER BY length(root.path) DESC LIMIT 1`
	var root string
	if err := s.db.QueryRowContext(ctx, query, folderID).Scan(&root); err != nil {
		return ""
	}
	return root
}

// relativePath returns the on-the-fly path of mediaPath relative to the nearest
// library root, or "" when the media is not beneath a readable root.
func (s *Postgres) relativePath(ctx context.Context, folderID int, mediaPath string) string {
	if root := s.rootPathForFolder(ctx, folderID); root != "" {
		return strings.TrimPrefix(strings.TrimPrefix(mediaPath, root), "/")
	}
	return ""
}

// attachRelativePaths fills in the computed relative path for each media item,
// memoizing the nearest root lookup per folder.
func (s *Postgres) attachRelativePaths(ctx context.Context, items []domain.Media) {
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

func (s *Postgres) FavoriteFolders(ctx context.Context, userID, viewID int) ([]domain.MediaFolder, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT mf.id, mf.parent_id, mf.path FROM favorite_folders ff JOIN media_folders mf ON mf.id = ff.folder_id WHERE ff.favorite_view_id = ?`, viewID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.MediaFolder{}
	for rows.Next() {
		var f domain.MediaFolder
		var parentID sql.NullInt64
		if err := rows.Scan(&f.ID, &parentID, &f.Path); err != nil {
			return nil, err
		}
		if parentID.Valid {
			f.ParentID = int(parentID.Int64)
		}
		f.Name = path.Base(f.Path)
		f.RelativePath = s.relativePath(ctx, 0, f.Path)
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Postgres) SetFavoriteFolder(ctx context.Context, userID, viewID, folderID int, favorite bool) error {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM favorite_views WHERE id = $1 AND user_id = $2`, viewID, userID).Scan(&exists); err != nil {
		return translateErr(err)
	}
	if favorite {
		_, err := s.db.ExecContext(ctx, `INSERT INTO favorite_folders(favorite_view_id, folder_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, viewID, folderID)
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM favorite_folders WHERE favorite_view_id = $1 AND folder_id = $2`, viewID, folderID)
	return err
}
