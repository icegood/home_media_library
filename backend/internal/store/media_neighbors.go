package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"media-library/backend/internal/domain"
)

// mediaNeighborsSQL builds a keyset query that returns up to `limit` media rows
// either before or after the anchor in a name- or date-sorted scope. The scope
// is the whole library (scopeLibrary true) or the subtree rooted at scopeID.
// The sort ordering mirrors the client sortMedia: empty taken_at sorts before
// every real date (mediaTime maps unknown dates to 0), dates sort ascending
// under date-asc and descending under date, and the name tie-break is always
// ascending; id breaks remaining ties deterministically.
func mediaNeighborsSQL(driver string, scopeLibrary bool, scopeID int, userID int, anchor domain.Media, sort, kind, gps, direction string, limit int) (string, []any) {
	placeholder := 0
	next := func() string {
		placeholder++
		if driver == "postgres" {
			return fmt.Sprintf("$%d", placeholder)
		}
		return "?"
	}
	name := "m.name COLLATE NOCASE"
	if driver == "postgres" {
		name = "LOWER(m.name)"
	}
	var b strings.Builder
	b.Grow(720)
	b.WriteString("WITH RECURSIVE covers(folder_id) AS (SELECT f.id FROM ")
	if scopeLibrary {
		b.WriteString("library_roots lr JOIN media_folders f ON f.id = lr.folder_id WHERE lr.library_id = ")
	} else {
		b.WriteString("media_folders f WHERE f.id = ")
	}
	args := []any{scopeID}
	b.WriteString(next())
	b.WriteString(" UNION ALL SELECT f.id FROM media_folders f JOIN covers ON f.parent_id = covers.folder_id)")
	b.WriteString(" SELECT " + mediaColumns + ", ")
	args = append(args, userID)
	if driver == "postgres" {
		b.WriteString(`EXISTS(SELECT 1 FROM favorite_view_items fvi JOIN favorite_views fv ON fv.id = fvi.favorite_view_id WHERE fv.user_id = ` + next() + ` AND fvi.media_id = m.id)`)
	} else {
		b.WriteString(favoriteExpr)
	}
	b.WriteString(" FROM media m JOIN covers ON covers.folder_id = m.folder_id")
	if kind != "" {
		b.WriteString(" JOIN media_mime_types mmt ON mmt.value = m.mime_type")
	}
	var where []string
	if kind != "" {
		args = append(args, kind)
		where = append(where, "mmt.media_type = "+next())
	}
	switch gps {
	case "gps":
		where = append(where, "m.gps <> ''")
	case "nogps":
		where = append(where, "m.gps = ''")
	}

	var cmp, order string
	switch sort {
	case "name":
		args = append(args, anchor.Name, anchor.Name, anchor.ID)
		if direction == "before" {
			// Nearest-first: walk the list backward from the anchor.
			cmp = name + " < " + next() + " OR (" + name + " = " + next() + " AND m.id < " + next() + ")"
			order = name + " DESC, m.id DESC"
		} else {
			cmp = name + " > " + next() + " OR (" + name + " = " + next() + " AND m.id > " + next() + ")"
			order = name + " ASC, m.id ASC"
		}
	default:
		takenAt := "m.taken_at"
		args = append(args, anchor.TakenAt, anchor.TakenAt, anchor.Name, anchor.TakenAt, anchor.Name, anchor.ID)
		if direction == "before" {
			// Nearest-first window toward the start of the list: the neighbor
			// just before the anchor comes back first. Under date-asc the list
			// start is the oldest (T < anchor); under date it is the newest
			// (T > anchor); both reverse the primary order inside the window.
			if sort == "date-asc" {
				cmp = takenAt + " < " + next() + " OR (" + takenAt + " = " + next() + " AND " + name + " < " + next() + ") OR (" + takenAt + " = " + next() + " AND " + name + " = " + next() + " AND m.id < " + next() + ")"
				order = takenAt + " DESC, " + name + " DESC, m.id DESC"
			} else {
				cmp = takenAt + " > " + next() + " OR (" + takenAt + " = " + next() + " AND " + name + " < " + next() + ") OR (" + takenAt + " = " + next() + " AND " + name + " = " + next() + " AND m.id < " + next() + ")"
				order = takenAt + " ASC, " + name + " DESC, m.id DESC"
			}
		} else {
			// Nearest-first window toward the end of the list, in list order.
			if sort == "date-asc" {
				cmp = takenAt + " > " + next() + " OR (" + takenAt + " = " + next() + " AND " + name + " > " + next() + ") OR (" + takenAt + " = " + next() + " AND " + name + " = " + next() + " AND m.id > " + next() + ")"
				order = takenAt + " ASC, " + name + " ASC, m.id ASC"
			} else {
				cmp = takenAt + " < " + next() + " OR (" + takenAt + " = " + next() + " AND " + name + " > " + next() + ") OR (" + takenAt + " = " + next() + " AND " + name + " = " + next() + " AND m.id > " + next() + ")"
				order = takenAt + " DESC, " + name + " ASC, m.id ASC"
			}
		}
	}
	args = append(args, limit)
	b.WriteString(" WHERE ")
	if len(where) > 0 {
		b.WriteString(strings.Join(where, " AND "))
		b.WriteString(" AND ")
	}
	b.WriteString("(" + cmp + ") ORDER BY " + order + " LIMIT " + next())
	return b.String(), args
}

// mediaNeighbors resolves the anchor media and its before/after window. It is
// shared by the SQLite and Postgres stores; the only driver-specific pieces
// (placeholder syntax, case-insensitive name collation) live in mediaNeighborsSQL.
func mediaNeighbors(ctx context.Context, db *sql.DB, enrich func(context.Context, []domain.Media) error, driver string, userID, libraryID, folderID, anchorID int, sort, kind, gps string, before, after int) (domain.MediaNeighbors, error) {
	if sort != "name" && sort != "date" && sort != "date-asc" {
		sort = "name"
	}
	anchorSQL := `SELECT ` + mediaColumns + `, ` + favoriteExpr + ` FROM media m WHERE m.id = ?`
	anchorArgs := []any{userID, anchorID}
	if driver == "postgres" {
		anchorSQL = `SELECT ` + mediaColumns + `, EXISTS(SELECT 1 FROM favorite_view_items fvi JOIN favorite_views fv ON fv.id = fvi.favorite_view_id WHERE fv.user_id = $1 AND fvi.media_id = m.id) FROM media m WHERE m.id = $2`
	}
	row := db.QueryRowContext(ctx, anchorSQL, anchorArgs...)
	var favorite bool
	anchor, err := scanMedia(row, &favorite)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.MediaNeighbors{}, ErrNotFound
		}
		return domain.MediaNeighbors{}, err
	}
	anchor.Favorite = favorite
	scopeLibrary := folderID <= 0
	scopeID := folderID
	if scopeLibrary {
		scopeID = libraryID
	}
	result := domain.MediaNeighbors{Anchor: anchor}
	query := func(direction string, limit int) ([]domain.Media, error) {
		statement, args := mediaNeighborsSQL(driver, scopeLibrary, scopeID, userID, anchor, sort, kind, gps, direction, limit)
		rows, err := db.QueryContext(ctx, statement, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []domain.Media{}
		for rows.Next() {
			var fav bool
			item, err := scanMedia(rows, &fav)
			if err != nil {
				return nil, err
			}
			item.Favorite = fav
			out = append(out, item)
		}
		return out, rows.Err()
	}
	if before > 0 {
		result.Before, err = query("before", before)
		if err != nil {
			return result, err
		}
	}
	if after > 0 {
		result.After, err = query("after", after)
		if err != nil {
			return result, err
		}
	}
	_ = enrich(ctx, result.Before)
	_ = enrich(ctx, result.After)
	enriched := []domain.Media{result.Anchor}
	_ = enrich(ctx, enriched)
	result.Anchor = enriched[0]
	return result, nil
}

func (s *SQLite) MediaNeighbors(ctx context.Context, userID, libraryID, folderID, anchorID int, sort, kind, gps string, before, after int) (domain.MediaNeighbors, error) {
	return mediaNeighbors(ctx, s.db, s.enrichMediaTrajectory, "sqlite", userID, libraryID, folderID, anchorID, sort, kind, gps, before, after)
}

func (s *Postgres) MediaNeighbors(ctx context.Context, userID, libraryID, folderID, anchorID int, sort, kind, gps string, before, after int) (domain.MediaNeighbors, error) {
	return mediaNeighbors(ctx, s.db, s.enrichMediaTrajectory, "postgres", userID, libraryID, folderID, anchorID, sort, kind, gps, before, after)
}