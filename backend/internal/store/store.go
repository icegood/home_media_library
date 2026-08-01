package store

import (
	"context"
	"errors"
	"time"

	"media-library/backend/internal/domain"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrForbidden = errors.New("forbidden")
	ErrConflict  = errors.New("conflict")
)

type Store interface {
	SetupRequired(ctx context.Context) (bool, error)
	CreateInitialAdmin(ctx context.Context, user domain.User, password string) (domain.User, error)
	ServerSettings(ctx context.Context) (domain.ServerSettings, error)
	SaveServerSettings(ctx context.Context, settings domain.ServerSettings) error
	MIMETypeForExtension(ctx context.Context, extension string) (string, error)
	UserSettings(ctx context.Context, userID int) (domain.UserSettings, error)
	SaveUserSettings(ctx context.Context, userID int, settings domain.UserSettings) error
	Authenticate(ctx context.Context, login, password string) (domain.User, error)
	User(ctx context.Context, id int) (domain.User, error)
	Users(ctx context.Context) ([]domain.User, error)
	CreateUser(ctx context.Context, user domain.User, password string) (domain.User, error)
	UpdateUser(ctx context.Context, user domain.User, password string) (domain.User, error)
	ImportSnapshot(ctx context.Context, snapshot domain.ImportSnapshot) (domain.ImportResult, error)
	LibrariesForUser(ctx context.Context, userID int, admin bool) ([]domain.Library, error)
	Library(ctx context.Context, id int) (domain.Library, error)
	Folder(ctx context.Context, id int) (domain.MediaFolder, error)
	CanRead(ctx context.Context, userID, libraryID int, admin bool) (bool, error)
	CanReadMedia(ctx context.Context, userID, mediaID int, admin bool) (bool, error)
	Entries(ctx context.Context, libraryID int, relativeDir string) ([]domain.Entry, error)
	EntriesForFolder(ctx context.Context, libraryID, folderID int) ([]domain.Entry, error)
	Media(ctx context.Context, id int) (domain.Media, error)
	MediaByPath(ctx context.Context, path string) (domain.Media, error)
	FavoriteViews(ctx context.Context, userID int) ([]domain.FavoriteView, error)
	CreateFavoriteView(ctx context.Context, userID int, name string) (domain.FavoriteView, error)
	UpdateFavoriteView(ctx context.Context, userID, viewID int, name string) (domain.FavoriteView, error)
	DeleteFavoriteView(ctx context.Context, userID, viewID int) error
	FavoriteMedia(ctx context.Context, userID, viewID int, admin bool) ([]domain.Media, error)
	SetFavorite(ctx context.Context, userID, viewID, mediaID int, favorite bool) (domain.Media, error)
	IsFavorite(ctx context.Context, userID, mediaID int) (bool, error)
	UpdateGPS(ctx context.Context, id int, patch domain.GPSPatch) (domain.Media, error)
	UpdateMediaDetails(ctx context.Context, id int, patch domain.MediaDetailsPatch) (domain.Media, error)
	GeotaggedMedia(ctx context.Context, userID int, admin bool) ([]domain.MapMedia, error)
	MediaForLibrary(ctx context.Context, libraryID int) ([]domain.Media, error)
	FoldersForLibrary(ctx context.Context, libraryID int) ([]domain.MediaFolder, error)
	ThumbnailCleanupRefsForLibrary(ctx context.Context, libraryID int) (domain.ThumbnailCleanupRefs, error)
	SetMediaActionError(ctx context.Context, id int, action, message string) error
	PruneFolder(ctx context.Context, rootFolderID int, keepFolders, keepMedia map[int]bool) error
	CreateLibrary(ctx context.Context, library domain.Library) (domain.Library, error)
	UpdateLibrary(ctx context.Context, library domain.Library) error
	DeleteLibrary(ctx context.Context, id int) error
	LibraryAccess(ctx context.Context, libraryID int) ([]domain.LibraryUserAccess, error)
	SetAccess(ctx context.Context, libraryID, userID int, allowed bool) error
	UpsertFolder(ctx context.Context, folder domain.MediaFolder) (domain.MediaFolder, error)
	UpsertMedia(ctx context.Context, media domain.Media) (domain.Media, error)
	Thumbnail(ctx context.Context, mediaID int, index int) (domain.Thumbnail, error)
	FolderThumbnailFile(ctx context.Context, folderID int) (domain.FolderThumbnail, error)
	FolderThumbnailRefs(ctx context.Context, folderID int, limit int) ([]domain.ThumbnailRef, error)
	UpsertFolderThumbnail(ctx context.Context, thumbnail domain.FolderThumbnail) error
	UpsertThumbnail(ctx context.Context, thumbnail domain.Thumbnail) error
	SaveJob(ctx context.Context, job domain.BackgroundJob) error
	Jobs(ctx context.Context) ([]domain.BackgroundJob, error)
	UnfinishedJobs(ctx context.Context) ([]domain.BackgroundJob, error)
	DeleteFinishedJobsBefore(ctx context.Context, before time.Time) error
}
