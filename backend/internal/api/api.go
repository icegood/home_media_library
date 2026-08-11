package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"media-library/backend/internal/applog"
	"media-library/backend/internal/domain"
	"media-library/backend/internal/embyimport"
	"media-library/backend/internal/gatewayconfig"
	"media-library/backend/internal/jobpool"
	"media-library/backend/internal/scanner"
	"media-library/backend/internal/scheduler"
	"media-library/backend/internal/store"
	"media-library/backend/internal/transcode"
)

type API struct {
	Store             store.Store
	Scanner           scanner.Scanner
	Transcoder        transcode.Service
	JWTSecret         []byte
	GatewayConfigPath string
	CaddyDataDir      string
	GatewayEnabled    bool
	ThumbnailDir      string
	LogFile           string
	Shutdown          func()
	ContainerStop     func(context.Context) error
	WorkerPool        *jobpool.Pool
	jobMu             sync.Mutex
	jobs              map[string]*JobStatus
	jobCancels        map[string]context.CancelFunc
	jobContexts       map[string]context.Context
	folderRefsMu      sync.Mutex
	folderRefs        map[int]folderRefsEntry
}

type folderRefsEntry struct {
	refs []domain.ThumbnailRef
	at   time.Time
}

const folderRefsCacheTTL = time.Minute

type JobStatus = domain.BackgroundJob

type claims struct {
	UserID int         `json:"uid"`
	Role   domain.Role `json:"role"`
	jwt.RegisteredClaims
}

type principal struct {
	ID   int
	Role domain.Role
}

type contextKey string

const principalKey contextKey = "principal"

func (a *API) Handler() http.Handler {
	a.recoverJobs()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /api/v1/setup", a.setupStatus)
	mux.HandleFunc("POST /api/v1/setup", a.setup)
	mux.HandleFunc("POST /api/v1/auth/login", a.login)
	mux.HandleFunc("POST /api/v1/auth/logout", a.logout)
	mux.HandleFunc("POST /api/v1/auth/forgot-password", a.forgotPassword)
	mux.HandleFunc("POST /api/v1/auth/reset-password", a.resetPassword)
	mux.Handle("GET /api/v1/me", a.auth(http.HandlerFunc(a.me)))
	mux.Handle("PUT /api/v1/me/password", a.auth(http.HandlerFunc(a.changePassword)))
	mux.Handle("PUT /api/v1/me/email", a.auth(http.HandlerFunc(a.setEmail)))
	mux.Handle("GET /api/v1/settings", a.auth(http.HandlerFunc(a.userSettings)))
	mux.Handle("PUT /api/v1/settings", a.auth(http.HandlerFunc(a.updateUserSettings)))
	mux.Handle("GET /api/v1/libraries", a.auth(http.HandlerFunc(a.libraries)))
	mux.Handle("GET /api/v1/libraries/{id}/stats", a.auth(http.HandlerFunc(a.libraryStats)))
	mux.Handle("GET /api/v1/libraries/{id}/entries", a.auth(http.HandlerFunc(a.entries)))
	mux.Handle("GET /api/v1/libraries/{id}/folders/{folderId}", a.auth(http.HandlerFunc(a.folder)))
	mux.Handle("GET /api/v1/libraries/{id}/folders/{folderId}/entries", a.auth(http.HandlerFunc(a.folderEntries)))
	mux.Handle("GET /api/v1/libraries/{id}/media", a.auth(http.HandlerFunc(a.libraryMedia)))
	mux.Handle("GET /api/v1/favorite-views", a.auth(http.HandlerFunc(a.favoriteViews)))
	mux.Handle("POST /api/v1/favorite-views", a.auth(http.HandlerFunc(a.createFavoriteView)))
	mux.Handle("PUT /api/v1/favorite-views/{viewId}", a.auth(http.HandlerFunc(a.updateFavoriteView)))
	mux.Handle("DELETE /api/v1/favorite-views/{viewId}", a.auth(http.HandlerFunc(a.deleteFavoriteView)))
	mux.Handle("GET /api/v1/favorite-views/{viewId}/media", a.auth(http.HandlerFunc(a.favoriteViewMedia)))
	mux.Handle("GET /api/v1/media/{id}", a.auth(http.HandlerFunc(a.media)))
	mux.Handle("GET /api/v1/media/{id}/favorite-views", a.auth(http.HandlerFunc(a.mediaFavoriteViews)))
	mux.Handle("PUT /api/v1/favorite-views/{viewId}/media/{id}", a.auth(http.HandlerFunc(a.favoriteMedia)))
	mux.Handle("DELETE /api/v1/favorite-views/{viewId}/media/{id}", a.auth(http.HandlerFunc(a.unfavoriteMedia)))
	mux.Handle("GET /api/v1/media/{id}/content", a.auth(http.HandlerFunc(a.content)))
	mux.Handle("GET /api/v1/media/{id}/play", a.auth(http.HandlerFunc(a.play)))
	mux.Handle("GET /api/v1/media/{id}/thumbnail", a.auth(http.HandlerFunc(a.thumbnail)))
	mux.Handle("GET /api/v1/media/{id}/thumbnails", a.auth(http.HandlerFunc(a.videoThumbnails)))
	mux.Handle("GET /api/v1/folders/{id}/thumbnail", a.auth(http.HandlerFunc(a.folderThumbnail)))
	mux.Handle("PATCH /api/v1/media/{id}/gps", a.auth(http.HandlerFunc(a.gps)))
	mux.Handle("PATCH /api/v1/media/{id}/details", a.auth(http.HandlerFunc(a.mediaDetails)))
	mux.Handle("GET /api/v1/map", a.auth(http.HandlerFunc(a.mapItems)))
	mux.Handle("POST /api/v1/admin/libraries", a.auth(a.admin(http.HandlerFunc(a.createLibrary))))
	mux.Handle("PUT /api/v1/admin/libraries/{id}", a.auth(a.admin(http.HandlerFunc(a.updateLibrary))))
	mux.Handle("DELETE /api/v1/admin/libraries/{id}", a.auth(a.admin(http.HandlerFunc(a.deleteLibrary))))
	mux.Handle("POST /api/v1/admin/libraries/{id}/scan", a.auth(a.admin(http.HandlerFunc(a.scanLibrary))))
	mux.Handle("POST /api/v1/admin/libraries/{id}/metadata/renew", a.auth(a.admin(http.HandlerFunc(a.scanLibrary))))
	mux.Handle("POST /api/v1/admin/libraries/{id}/thumbnails", a.auth(a.admin(http.HandlerFunc(a.thumbnailLibrary))))
	mux.Handle("GET /api/v1/admin/scheduled-tasks", a.auth(a.admin(http.HandlerFunc(a.scheduledTasks))))
	mux.Handle("POST /api/v1/admin/scheduled-tasks", a.auth(a.admin(http.HandlerFunc(a.createScheduledTask))))
	mux.Handle("PUT /api/v1/admin/scheduled-tasks/{id}", a.auth(a.admin(http.HandlerFunc(a.updateScheduledTask))))
	mux.Handle("DELETE /api/v1/admin/scheduled-tasks/{id}", a.auth(a.admin(http.HandlerFunc(a.deleteScheduledTask))))
	mux.Handle("GET /api/v1/admin/jobs", a.auth(a.admin(http.HandlerFunc(a.jobsList))))
	mux.Handle("GET /api/v1/admin/logs", a.auth(a.admin(http.HandlerFunc(a.logs))))
	mux.Handle("GET /api/v1/admin/logs/download", a.auth(a.admin(http.HandlerFunc(a.downloadLogs))))
	mux.Handle("DELETE /api/v1/admin/logs", a.auth(a.admin(http.HandlerFunc(a.clearLogs))))
	mux.Handle("POST /api/v1/admin/jobs/{id}/pause", a.auth(a.admin(http.HandlerFunc(a.pauseJob))))
	mux.Handle("POST /api/v1/admin/jobs/{id}/resume", a.auth(a.admin(http.HandlerFunc(a.resumeJob))))
	mux.Handle("POST /api/v1/admin/jobs/{id}/cancel", a.auth(a.admin(http.HandlerFunc(a.cancelJob))))
	mux.Handle("POST /api/v1/admin/thumbnails/orphans", a.auth(a.admin(http.HandlerFunc(a.cleanupOrphanThumbnails))))
	mux.Handle("POST /api/v1/admin/db/vacuum", a.auth(a.admin(http.HandlerFunc(a.vacuumDB))))
	mux.Handle("POST /api/v1/admin/shutdown", a.auth(a.admin(http.HandlerFunc(a.shutdown))))
	mux.Handle("GET /api/v1/admin/users", a.auth(a.admin(http.HandlerFunc(a.users))))
	mux.Handle("POST /api/v1/admin/users", a.auth(a.admin(http.HandlerFunc(a.createUser))))
	mux.Handle("PUT /api/v1/admin/users/{id}", a.auth(a.admin(http.HandlerFunc(a.updateUser))))
	mux.Handle("GET /api/v1/admin/libraries/{id}/access", a.auth(a.admin(http.HandlerFunc(a.libraryAccess))))
	mux.Handle("PUT /api/v1/admin/libraries/{id}/access/{userId}", a.auth(a.admin(http.HandlerFunc(a.setAccess))))
	mux.Handle("POST /api/v1/admin/import/emby", a.auth(a.admin(http.HandlerFunc(a.importEmby))))
	mux.Handle("GET /api/v1/admin/filesystem", a.auth(a.admin(http.HandlerFunc(a.filesystem))))
	mux.Handle("GET /api/v1/admin/settings", a.auth(a.admin(http.HandlerFunc(a.getSettings))))
	mux.Handle("PUT /api/v1/admin/settings", a.auth(a.admin(http.HandlerFunc(a.updateSettings))))
	mux.HandleFunc("GET /swagger", a.swaggerRedirect)
	mux.HandleFunc("GET /swagger/", a.swaggerUI)
	mux.HandleFunc("GET /swagger/openapi.yaml", a.swaggerSpec)
	return logRequests(securityHeaders(mux))
}

func (a *API) setupStatus(w http.ResponseWriter, r *http.Request) {
	required, err := a.Store.SetupRequired(r.Context())
	if err != nil {
		problem(w, http.StatusInternalServerError, "could not read setup status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"required": required})
}

func (a *API) setup(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		problem(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	login := normalizeLogin(input.Login)
	if !validLogin(login) {
		problem(w, http.StatusBadRequest, "login must contain 3 to 64 letters, numbers, dots, dashes, or underscores")
		return
	}
	if len(input.Password) < 12 || len(input.Password) > 72 {
		problem(w, http.StatusBadRequest, "password must contain between 12 and 72 characters")
		return
	}
	user := domain.User{ID: domain.InvalidID, Login: login, Role: domain.RoleAdmin}
	user, err := a.Store.CreateInitialAdmin(r.Context(), user, input.Password)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			problem(w, http.StatusConflict, "initial setup is already complete")
			return
		}
		problem(w, http.StatusInternalServerError, "could not create administrator")
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var input struct{ Login, Password string }
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		problem(w, 400, "invalid JSON")
		return
	}
	login := normalizeLogin(input.Login)
	user, err := a.Store.Authenticate(r.Context(), login, input.Password)
	if err != nil {
		applog.Printf(applog.Warn, "failed login for %q from %s", login, remoteAddr(r))
		problem(w, 401, "invalid credentials")
		return
	}
	applog.Printf(applog.Info, "session started for user %q (id %d) from %s", user.Login, user.ID, remoteAddr(r))
	now := time.Now()
	sessionMaxAge := time.Duration(a.serverSettings(r.Context()).SessionMaxAgeHours) * time.Hour
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{UserID: user.ID, Role: user.Role,
		RegisteredClaims: jwt.RegisteredClaims{IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(sessionMaxAge))}})
	signed, err := token.SignedString(a.JWTSecret)
	if err != nil {
		problem(w, 500, "could not create token")
		return
	}
	secureCookie := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	http.SetCookie(w, &http.Cookie{Name: "media_session", Value: signed, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, Secure: secureCookie, MaxAge: int(sessionMaxAge.Seconds())})
	writeJSON(w, 200, map[string]any{"user": user})
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "media_session", Value: "", Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"), MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	p := current(r)
	user, err := a.Store.User(r.Context(), p.ID)
	if err != nil {
		problem(w, 404, "user not found")
		return
	}
	writeJSON(w, 200, user)
}

func (a *API) changePassword(w http.ResponseWriter, r *http.Request) {
	p := current(r)
	var input struct {
		Current string `json:"currentPassword"`
		New     string `json:"newPassword"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		problem(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if len(input.New) < 12 || len(input.New) > 72 {
		problem(w, http.StatusBadRequest, "password must contain between 12 and 72 characters")
		return
	}
	user, err := a.Store.User(r.Context(), p.ID)
	if err != nil {
		problem(w, http.StatusNotFound, "user not found")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Current)) != nil {
		problem(w, http.StatusForbidden, "current password is incorrect")
		return
	}
	if err := a.Store.UpdatePassword(r.Context(), p.ID, input.New); err != nil {
		problem(w, http.StatusInternalServerError, "could not update password")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *API) setEmail(w http.ResponseWriter, r *http.Request) {
	p := current(r)
	var input struct {
		Email string `json:"email"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		problem(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	email := strings.ToLower(strings.TrimSpace(input.Email))
	if email != "" && !validEmail(email) {
		problem(w, http.StatusBadRequest, "enter a valid email address")
		return
	}
	if err := a.Store.SetUserEmail(r.Context(), p.ID, email); err != nil {
		if errors.Is(err, store.ErrConflict) {
			problem(w, http.StatusConflict, "this email is already in use by another account")
			return
		}
		if errors.Is(err, store.ErrNotFound) {
			problem(w, http.StatusNotFound, "user not found")
			return
		}
		problem(w, http.StatusInternalServerError, "could not save email")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"email": email})
}

func (a *API) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email string `json:"email"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		problem(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	email := strings.ToLower(strings.TrimSpace(input.Email))
	if !validEmail(email) {
		problem(w, http.StatusBadRequest, "enter a valid email address")
		return
	}
	settings := a.serverSettings(r.Context())
	smtp := settings.SMTP()
	if !smtp.Enabled() {
		writeJSON(w, http.StatusOK, map[string]any{"sent": false, "reason": "smtpNotConfigured"})
		return
	}
	user, err := a.Store.UserByEmail(r.Context(), email)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]bool{"sent": true})
		return
	}
	token := passwordResetToken()
	expiresAt := time.Now().UTC().Add(time.Hour)
	if err := a.Store.CreatePasswordResetToken(r.Context(), user.ID, hashToken(token), expiresAt); err != nil {
		problem(w, http.StatusInternalServerError, "could not create a reset link")
		return
	}
	link := a.appBaseURL(r, settings) + "/reset?token=" + url.QueryEscape(token)
	body := "A password reset was requested for your " + a.appName() + " account.\n\n" +
		"Open this link to choose a new password (valid for one hour):\n\n" + link + "\n\n" +
		"If you did not request this, you can safely ignore this email."
	if err := smtp.Send(user.Email, "Password reset for "+a.appName(), body); err != nil {
		applog.Printf(applog.Error, "password reset email to %q failed: %v", user.Email, err)
		writeJSON(w, http.StatusOK, map[string]bool{"sent": true})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"sent": true})
}

func (a *API) resetPassword(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		problem(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if len(input.Password) < 12 || len(input.Password) > 72 {
		problem(w, http.StatusBadRequest, "password must contain between 12 and 72 characters")
		return
	}
	if strings.TrimSpace(input.Token) == "" {
		problem(w, http.StatusBadRequest, "reset token is missing")
		return
	}
	userID, err := a.Store.ConsumePasswordResetToken(r.Context(), hashToken(input.Token))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			problem(w, http.StatusBadRequest, "this reset link is invalid or has expired")
			return
		}
		problem(w, http.StatusInternalServerError, "could not use the reset link")
		return
	}
	if err := a.Store.UpdatePassword(r.Context(), userID, input.Password); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			problem(w, http.StatusBadRequest, "this reset link is invalid or has expired")
			return
		}
		problem(w, http.StatusInternalServerError, "could not update password")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func passwordResetToken() string {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return ""
	}
	return hex.EncodeToString(raw)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (a *API) appBaseURL(r *http.Request, settings domain.ServerSettings) string {
	host := strings.TrimSpace(settings.PublicDNS)
	if host == "" {
		host = r.Host
	}
	if settings.HTTPSEnabled {
		return "https://" + host
	}
	return "http://" + host
}

func (a *API) appName() string {
	return "Media Library"
}

func validEmail(value string) bool {
	if len(value) < 3 || len(value) > 254 || !strings.Contains(value, "@") {
		return false
	}
	at := strings.LastIndex(value, "@")
	domainPart := value[at+1:]
	if at == 0 || domainPart == "" || !strings.Contains(domainPart, ".") {
		return false
	}
	return true
}

func (a *API) userSettings(w http.ResponseWriter, r *http.Request) {
	p := current(r)
	settings, err := a.Store.UserSettings(r.Context(), p.ID)
	if err != nil {
		problem(w, http.StatusInternalServerError, "could not read user settings")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (a *API) updateUserSettings(w http.ResponseWriter, r *http.Request) {
	p := current(r)
	var input struct {
		Theme string `json:"theme"`
		Codec string `json:"codec"`
		Zoom  int    `json:"zoom"`
		DefaultThumbImage  string `json:"defaultThumbImage"`
		DefaultThumbVideo  string `json:"defaultThumbVideo"`
		DefaultThumbFolder string `json:"defaultThumbFolder"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		problem(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	input.Theme = strings.TrimSpace(input.Theme)
	if input.Theme != "light" && input.Theme != "dark" && input.Theme != "system" {
		problem(w, http.StatusBadRequest, "theme must be light, dark, or system")
		return
	}
	schema, err := transcode.ParseSchema(input.Codec)
	if err != nil {
		problem(w, http.StatusBadRequest, "codec must be a valid transcode schema (e.g. h264-aac-mp4, vp9-opus-webm)")
		return
	}
	zoom := input.Zoom
	if zoom == 0 {
		zoom = 100
	}
	if zoom < 80 || zoom > 140 {
		problem(w, http.StatusBadRequest, "zoom must be between 80 and 140")
		return
	}
	thumbs := domain.DefaultUserSettings()
	thumbs.DefaultThumbImage, err = normalizeDefaultThumb(input.DefaultThumbImage)
	if err != nil {
		problem(w, http.StatusBadRequest, err.Error())
		return
	}
	thumbs.DefaultThumbVideo, err = normalizeDefaultThumb(input.DefaultThumbVideo)
	if err != nil {
		problem(w, http.StatusBadRequest, err.Error())
		return
	}
	thumbs.DefaultThumbFolder, err = normalizeDefaultThumb(input.DefaultThumbFolder)
	if err != nil {
		problem(w, http.StatusBadRequest, err.Error())
		return
	}
	settings := domain.UserSettings{Theme: input.Theme, Codec: schema.ID, Zoom: zoom,
		DefaultThumbImage: thumbs.DefaultThumbImage, DefaultThumbVideo: thumbs.DefaultThumbVideo, DefaultThumbFolder: thumbs.DefaultThumbFolder}
	if err := a.Store.SaveUserSettings(r.Context(), p.ID, settings); err != nil {
		problem(w, http.StatusInternalServerError, "could not save user settings")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

// normalizeDefaultThumb validates a default-thumbnail picture id. The catalog
// of ids lives in the web UI; unknown ids fall back to the default there, so
// the backend only checks the shape.
func normalizeDefaultThumb(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "mountains", nil
	}
	if len(value) > 32 {
		return "", errors.New("default thumbnail picture id is too long")
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-' {
			continue
		}
		return "", errors.New("default thumbnail picture id may only contain lowercase letters, digits, and dashes")
	}
	return value, nil
}

func (a *API) libraries(w http.ResponseWriter, r *http.Request) {
	p := current(r)
	items, err := a.Store.LibrariesForUser(r.Context(), p.ID, p.Role == domain.RoleAdmin)
	if err != nil {
		problem(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, items)
}

func (a *API) libraryStats(w http.ResponseWriter, r *http.Request) {
	p := current(r)
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if !a.requireRead(w, r, p, id) {
		return
	}
	stats, err := a.Store.LibraryStats(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			problem(w, http.StatusNotFound, "library not found")
			return
		}
		problem(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (a *API) entries(w http.ResponseWriter, r *http.Request) {
	p := current(r)
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if !a.requireRead(w, r, p, id) {
		return
	}
	items, err := a.Store.Entries(r.Context(), p.ID, id, "")
	if err != nil {
		problem(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, items)
}

func (a *API) folderEntries(w http.ResponseWriter, r *http.Request) {
	p := current(r)
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	folderID, ok := pathID(w, r, "folderId")
	if !ok {
		return
	}
	if !a.requireRead(w, r, p, id) {
		return
	}
	result, err := a.Store.EntriesForFolder(r.Context(), p.ID, id, folderID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			problem(w, http.StatusNotFound, "folder not found in library")
			return
		}
		problem(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *API) folder(w http.ResponseWriter, r *http.Request) {
	p := current(r)
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	folderID, ok := pathID(w, r, "folderId")
	if !ok {
		return
	}
	if !a.requireRead(w, r, p, id) {
		return
	}
	if _, err := a.Store.FolderChain(r.Context(), id, folderID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			problem(w, http.StatusNotFound, "folder not found in library")
			return
		}
		problem(w, http.StatusInternalServerError, err.Error())
		return
	}
	folder, err := a.Store.Folder(r.Context(), folderID)
	if err != nil {
		problem(w, http.StatusNotFound, "folder not found")
		return
	}
	writeJSON(w, http.StatusOK, folder)
}

func (a *API) libraryMedia(w http.ResponseWriter, r *http.Request) {
	p := current(r)
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if !a.requireRead(w, r, p, id) {
		return
	}
	items, err := a.Store.MediaForLibrary(r.Context(), p.ID, id)
	if err != nil {
		problem(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, items)
}

func (a *API) media(w http.ResponseWriter, r *http.Request) {
	p := current(r)
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	item, err := a.Store.Media(r.Context(), id)
	if err != nil {
		problem(w, 404, "media not found")
		return
	}
	if !a.requireMediaRead(w, r, p, item.ID) {
		return
	}
	item = a.withFavorite(r.Context(), p, item)
	writeJSON(w, 200, item)
}

func (a *API) favoriteViews(w http.ResponseWriter, r *http.Request) {
	p := current(r)
	views, err := a.Store.FavoriteViews(r.Context(), p.ID)
	if err != nil {
		problem(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, views)
}

func (a *API) mediaFavoriteViews(w http.ResponseWriter, r *http.Request) {
	p := current(r)
	mediaID, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if !a.requireMediaRead(w, r, p, mediaID) {
		return
	}
	views, err := a.Store.FavoriteViewsForMedia(r.Context(), p.ID, mediaID)
	if err != nil {
		problem(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, views)
}

func (a *API) createFavoriteView(w http.ResponseWriter, r *http.Request) {
	p := current(r)
	var input struct {
		Name string `json:"name"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		problem(w, 400, "invalid JSON")
		return
	}
	view, err := a.Store.CreateFavoriteView(r.Context(), p.ID, input.Name)
	if err != nil {
		problem(w, 400, "favorite view name is required")
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

func (a *API) updateFavoriteView(w http.ResponseWriter, r *http.Request) {
	p := current(r)
	viewID, ok := pathID(w, r, "viewId")
	if !ok {
		return
	}
	var input struct {
		Name string `json:"name"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		problem(w, 400, "invalid JSON")
		return
	}
	view, err := a.Store.UpdateFavoriteView(r.Context(), p.ID, viewID, input.Name)
	if err != nil {
		problem(w, statusFor(err), "favorite view not found")
		return
	}
	writeJSON(w, 200, view)
}

func (a *API) deleteFavoriteView(w http.ResponseWriter, r *http.Request) {
	p := current(r)
	viewID, ok := pathID(w, r, "viewId")
	if !ok {
		return
	}
	if err := a.Store.DeleteFavoriteView(r.Context(), p.ID, viewID); err != nil {
		problem(w, statusFor(err), "favorite view not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) favoriteViewMedia(w http.ResponseWriter, r *http.Request) {
	p := current(r)
	viewID, ok := pathID(w, r, "viewId")
	if !ok {
		return
	}
	items, err := a.Store.FavoriteMedia(r.Context(), p.ID, viewID, p.Role == domain.RoleAdmin)
	if err != nil {
		problem(w, statusFor(err), "favorite view not found")
		return
	}
	writeJSON(w, 200, items)
}

func (a *API) favoriteMedia(w http.ResponseWriter, r *http.Request) {
	a.setFavorite(w, r, true)
}

func (a *API) unfavoriteMedia(w http.ResponseWriter, r *http.Request) {
	a.setFavorite(w, r, false)
}

func (a *API) setFavorite(w http.ResponseWriter, r *http.Request, favorite bool) {
	p := current(r)
	mediaID, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if !a.requireMediaRead(w, r, p, mediaID) {
		return
	}
	viewID, ok := pathID(w, r, "viewId")
	if !ok {
		return
	}
	item, err := a.Store.SetFavorite(r.Context(), p.ID, viewID, mediaID, favorite)
	if err != nil {
		problem(w, statusFor(err), "favorite view not found")
		return
	}
	item = a.withFavorite(r.Context(), p, item)
	writeJSON(w, 200, item)
}

func (a *API) withFavorite(ctx context.Context, p principal, item domain.Media) domain.Media {
	favorite, err := a.Store.IsFavorite(ctx, p.ID, item.ID)
	if err == nil {
		item.Favorite = favorite
	}
	return item
}

func (a *API) gps(w http.ResponseWriter, r *http.Request) {
	p := current(r)
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	item, err := a.Store.Media(r.Context(), id)
	if err != nil {
		problem(w, 404, "media not found")
		return
	}
	if !a.requireMediaRead(w, r, p, item.ID) {
		return
	}
	var patch domain.GPSPatch
	if json.NewDecoder(r.Body).Decode(&patch) != nil {
		problem(w, 400, "invalid JSON")
		return
	}
	if patch.GPS != nil {
		canonical, ok := domain.CanonicalGPS(*patch.GPS)
		if !ok {
			problem(w, 400, "gps must use valid latitude,longitude format")
			return
		}
		patch.GPS = &canonical
	}
	item, err = a.Store.UpdateGPS(r.Context(), item.ID, patch)
	if err != nil {
		problem(w, 500, err.Error())
		return
	}
	item = a.withFavorite(r.Context(), p, item)
	writeJSON(w, 200, item)
}

func (a *API) mediaDetails(w http.ResponseWriter, r *http.Request) {
	p := current(r)
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	item, err := a.Store.Media(r.Context(), id)
	if err != nil {
		problem(w, 404, "media not found")
		return
	}
	if !a.requireMediaRead(w, r, p, item.ID) {
		return
	}
	var patch domain.MediaDetailsPatch
	if json.NewDecoder(r.Body).Decode(&patch) != nil {
		problem(w, 400, "invalid JSON")
		return
	}
	if patch.Name != nil {
		name := strings.TrimSpace(*patch.Name)
		if name == "" {
			problem(w, 400, "name is required")
			return
		}
		patch.Name = &name
	}
	if patch.GPS != nil && strings.TrimSpace(*patch.GPS) != "" {
		canonical, ok := domain.CanonicalGPS(*patch.GPS)
		if !ok {
			problem(w, 400, "gps must use valid latitude,longitude format")
			return
		}
		patch.GPS = &canonical
	}
	if patch.GPS != nil && strings.TrimSpace(*patch.GPS) == "" {
		empty := ""
		patch.GPS = &empty
	}
	if patch.TakenAt != nil {
		normalized, ok := normalizeMediaDate(*patch.TakenAt)
		if !ok {
			problem(w, 400, "takenAt must be a valid date/time")
			return
		}
		patch.TakenAt = &normalized
	}
	item, err = a.Store.UpdateMediaDetails(r.Context(), item.ID, patch)
	if err != nil {
		problem(w, 500, err.Error())
		return
	}
	item = a.withFavorite(r.Context(), p, item)
	writeJSON(w, 200, item)
}

func normalizeMediaDate(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", true
	}
	layouts := []string{
		time.RFC3339Nano,
		"2006-01-02T15:04",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format(time.RFC3339), true
		}
	}
	return "", false
}

func (a *API) content(w http.ResponseWriter, r *http.Request) {
	item, file, ok := a.openMedia(w, r)
	if !ok {
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		problem(w, 500, "could not read media")
		return
	}
	w.Header().Set("Content-Type", item.MIMEType)
	http.ServeContent(w, r, item.Name, info.ModTime(), file)
}

func (a *API) play(w http.ResponseWriter, r *http.Request) {
	item, file, ok := a.openMedia(w, r)
	if !ok {
		return
	}
	defer file.Close()
	if item.Kind != domain.KindVideo {
		problem(w, http.StatusBadRequest, "media is not a video")
		return
	}
	sourceCodec, err := a.Transcoder.Probe(r.Context(), file.Name())
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	sourceAudio, err := a.Transcoder.ProbeAudio(r.Context(), file.Name())
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	supported := codecSet(r.URL.Query().Get("codecs"))
	if supported[sourceCodec] && transcode.DirectPlayAudioSupported(sourceCodec, sourceAudio) &&
		transcode.DirectPlayContainerSupported(sourceCodec, item.MIMEType) {
		applog.Printf(applog.Debug, "direct play media %d (%s) for user %d: source video %s", item.ID, item.RelativePath, current(r).ID, sourceCodec)
		info, err := file.Stat()
		if err != nil {
			problem(w, http.StatusInternalServerError, "could not read video")
			return
		}
		w.Header().Set("X-Media-Playback", "original")
		w.Header().Set("Content-Type", item.MIMEType)
		http.ServeContent(w, r, item.Name, info.ModTime(), file)
		return
	}
	settings, err := a.Store.UserSettings(r.Context(), current(r).ID)
	if err != nil {
		problem(w, http.StatusInternalServerError, "could not read user settings")
		return
	}
	schema, err := transcode.ParseSchema(settings.Codec)
	if err != nil {
		problem(w, http.StatusInternalServerError, "invalid stored transcode schema")
		return
	}
	transcoder := a.Transcoder
	transcoder.Target = schema
	if !supported[schema.Video] {
		problem(w, http.StatusNotAcceptable, "browser does not support source or configured transcode codec")
		return
	}
	applog.Printf(applog.Info, "transcoding media %d (%s) for user %d: source video %s -> schema %s", item.ID, item.RelativePath, current(r).ID, sourceCodec, schema.ID)
	w.Header().Set("X-Media-Playback", "transcoded")
	w.Header().Set("Content-Type", transcoder.ContentType())
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_ = transcoder.Stream(r.Context(), file.Name(), w, startSeconds(r))
}

func startSeconds(r *http.Request) float64 {
	raw := strings.TrimSpace(r.URL.Query().Get("start"))
	if raw == "" {
		return 0
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value < 0 {
		return 0
	}
	return value
}

func codecSet(value string) map[transcode.Codec]bool {
	result := map[transcode.Codec]bool{}
	for _, raw := range strings.Split(value, ",") {
		if codec, err := transcode.ParseCodec(raw); err == nil {
			result[codec] = true
		}
	}
	return result
}

func (a *API) thumbnail(w http.ResponseWriter, r *http.Request) {
	item, file, ok := a.openMedia(w, r)
	if !ok {
		return
	}
	defer file.Close()
	index := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("index")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			problem(w, http.StatusBadRequest, "thumbnail index must be a non-negative integer")
			return
		}
		index = value
	}
	target := a.thumbnailPath(item.ID, index)
	if !usableThumbnail(target) {
		if _, err := os.Stat(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			problem(w, http.StatusInternalServerError, "could not read thumbnail storage")
			return
		}
		if item.ThumbnailError != "" {
			problem(w, http.StatusUnprocessableEntity, "thumbnail skipped because previous error is not cleared: "+item.ThumbnailError)
			return
		}
		problem(w, http.StatusNotFound, "thumbnail not generated yet")
		return
	}
	if item.Kind == domain.KindVideo && index >= len(a.videoThumbnailTimes(r.Context(), item)) {
		problem(w, http.StatusNotFound, "thumbnail not configured for this video")
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, target)
}

func (a *API) thumbnailPath(mediaID int, index int) string {
	return filepath.Join(a.thumbnailRoot(), "media", thumbnailBucket(mediaID), fmt.Sprintf("%d_%d.jpg", mediaID, index))
}

func usableThumbnail(target string) bool {
	info, err := os.Stat(target)
	return err == nil && info.Size() > 0
}

func (a *API) folderThumbnailPath(folderID int) string {
	return filepath.Join(a.thumbnailRoot(), "folders", thumbnailBucket(folderID), fmt.Sprintf("%d_0.jpg", folderID))
}

func (a *API) thumbnailRoot() string {
	if a.ThumbnailDir == "" {
		return "/thumbnails"
	}
	return a.ThumbnailDir
}

func thumbnailBucket(id int) string {
	if id < 0 {
		return "invalid"
	}
	return strconv.Itoa(id / 1000)
}

func parseIndexedThumbnailName(name string) (int, int, bool) {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	parts := strings.Split(base, "_")
	if len(parts) != 2 {
		return domain.InvalidID, domain.InvalidID, false
	}
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		return domain.InvalidID, domain.InvalidID, false
	}
	index, err := strconv.Atoi(parts[1])
	if err != nil {
		return domain.InvalidID, domain.InvalidID, false
	}
	return id, index, true
}

func (a *API) cleanupThumbnailRefs(ctx context.Context, refs domain.ThumbnailCleanupRefs) {
	mediaDirs := map[string]bool{}
	existingMedia, err := a.mediaForRefs(ctx, refs.Media)
	if err != nil {
		applog.Printf(applog.Warn, "could not verify thumbnail media owners, keeping thumbnails: %s", err)
	} else {
		for _, ref := range refs.Media {
			if _, ok := existingMedia[ref.MediaID]; ok {
				continue
			}
			target := a.thumbnailPath(ref.MediaID, ref.Index)
			if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
				applog.Printf(applog.Warn, "could not delete thumbnail file %s: %s", target, err)
				continue
			}
			mediaDirs[filepath.Dir(target)] = true
			applog.Printf(applog.Info, "deleted thumbnail file %s", target)
		}
	}
	for dir := range mediaDirs {
		if err := os.Remove(dir); err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, syscall.ENOTEMPTY) {
			applog.Printf(applog.Warn, "could not delete thumbnail folder %s: %s", dir, err)
		}
	}
	existingFolders, err := a.Store.FoldersByIDs(ctx, refs.Folders)
	if err != nil {
		applog.Printf(applog.Warn, "could not verify folder thumbnail owners, keeping thumbnails: %s", err)
		return
	}
	for _, folderID := range refs.Folders {
		if _, ok := existingFolders[folderID]; ok {
			continue
		}
		target := a.folderThumbnailPath(folderID)
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			applog.Printf(applog.Warn, "could not delete folder thumbnail file %s: %s", target, err)
			continue
		}
		applog.Printf(applog.Info, "deleted folder thumbnail file %s", target)
	}
}

func (a *API) generateImageThumbnail(ctx context.Context, source string, mediaID int, index int, target string) error {
	if index != 0 {
		return fmt.Errorf("images only have thumbnail index 0")
	}
	return a.generateThumbnail(ctx, source, target, []string{"-i", source})
}

func (a *API) generateVideoThumbnail(ctx context.Context, source string, item domain.Media, index int, target string) error {
	times := a.videoThumbnailTimes(ctx, item)
	if index < 0 || index >= len(times) {
		return fmt.Errorf("thumbnail index is outside configured video thumbnail count")
	}
	seekSeconds := times[index]
	return a.generateThumbnail(ctx, source, target, []string{"-ss", fmt.Sprintf("%d", seekSeconds), "-i", source, "-frames:v", "1"})
}

func (a *API) generateThumbnail(ctx context.Context, source, target string, inputArgs []string) error {
	width, height := a.thumbnailDimensions(ctx)
	targetDir := filepath.Dir(target)
	if err := os.MkdirAll(targetDir, 0o770); err != nil {
		return fmt.Errorf("could not prepare thumbnail storage: %w", err)
	}
	_ = os.Chmod(targetDir, 0o770)
	temp, err := os.CreateTemp(targetDir, ".thumb-*.jpg")
	if err != nil {
		return fmt.Errorf("could not create thumbnail temp file: %w", err)
	}
	tempName := temp.Name()
	_ = temp.Close()
	defer os.Remove(tempName)
	args := []string{"-hide_banner", "-loglevel", "error"}
	args = append(args, inputArgs...)
	filter := fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2",
		width, height, width, height,
	)
	args = append(args, "-vf", filter, "-q:v", "3", "-y", tempName)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("could not create thumbnail: %s", strings.TrimSpace(string(output)))
	}
	info, err := os.Stat(tempName)
	if err != nil {
		return fmt.Errorf("could not stat generated thumbnail: %w", err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("ffmpeg produced an empty thumbnail (seek is likely past the end of the media)")
	}
	_ = os.Chmod(tempName, 0o660)
	if err := os.Rename(tempName, target); err != nil {
		return fmt.Errorf("could not store thumbnail: %w", err)
	}
	_ = os.Chmod(target, 0o660)
	return nil
}

func (a *API) thumbnailDimensions(ctx context.Context) (int, int) {
	settings := a.serverSettings(ctx)
	return settings.ThumbnailWidth, settings.ThumbnailHeight
}

func (a *API) videoThumbnailTiming(ctx context.Context) (int, int, int) {
	settings := a.serverSettings(ctx)
	first := settings.VideoThumbnailFirstSeconds
	maxCount := settings.VideoThumbnailMaxCount
	minInterval := settings.VideoThumbnailMinIntervalSeconds
	if maxCount < 1 {
		maxCount = 1
	}
	if maxCount > domain.MaxVideoThumbnailCount {
		maxCount = domain.MaxVideoThumbnailCount
	}
	if minInterval < 1 {
		minInterval = 120
	}
	return first, maxCount, minInterval
}

func (a *API) videoThumbnailTimes(ctx context.Context, item domain.Media) []int {
	first, maxCount, minInterval := a.videoThumbnailTiming(ctx)
	duration := metadataDurationSeconds(item.Metadata)
	if duration > 0 && float64(first) >= duration {
		return []int{}
	}
	if duration <= 0 || maxCount == 1 || duration <= float64(first) {
		return []int{first}
	}
	remaining := math.Max(1, duration-float64(first))
	possibleByMinInterval := int(math.Ceil(remaining / float64(minInterval)))
	if possibleByMinInterval < 1 {
		possibleByMinInterval = 1
	}
	count := maxCount
	if possibleByMinInterval < count {
		count = possibleByMinInterval
	}
	if count <= 1 {
		return []int{first}
	}
	step := math.Max(float64(minInterval), remaining/float64(count))
	times := make([]int, 0, count)
	previous := -1
	for index := 0; index < count; index++ {
		second := int(math.Round(float64(first) + float64(index)*step))
		if second <= previous {
			second = previous + 1
		}
		if float64(second) >= duration {
			break
		}
		if second != previous {
			times = append(times, second)
			previous = second
		}
	}
	if len(times) == 0 {
		return []int{first}
	}
	return times
}

func (a *API) videoThumbnails(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	item, err := a.Store.Media(r.Context(), id)
	if err != nil {
		problem(w, http.StatusNotFound, "media not found")
		return
	}
	if item.Kind != domain.KindVideo {
		problem(w, http.StatusBadRequest, "media is not a video")
		return
	}
	if !a.requireMediaRead(w, r, current(r), item.ID) {
		return
	}
	times := a.videoThumbnailTimes(r.Context(), item)
	out := []map[string]any{}
	for index, second := range times {
		out = append(out, map[string]any{
			"index": index, "timeSeconds": second,
			"url": fmt.Sprintf("/api/v1/media/%d/thumbnail?index=%d", item.ID, index),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func metadataDurationSeconds(metadata map[string]any) float64 {
	ffprobe, ok := metadata["ffprobe"].(map[string]any)
	if !ok {
		return 0
	}
	format, ok := ffprobe["format"].(map[string]any)
	if !ok {
		return 0
	}
	switch value := format["duration"].(type) {
	case string:
		parsed, _ := strconv.ParseFloat(value, 64)
		return parsed
	case float64:
		return value
	default:
		return 0
	}
}

func (a *API) folderThumbnail(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	folder, err := a.Store.Folder(r.Context(), id)
	if err != nil {
		problem(w, http.StatusNotFound, "folder not found")
		return
	}
	refs := a.folderThumbnailRefs(r.Context(), folder.ID, 3)
	if len(refs) == 0 {
		problem(w, http.StatusNotFound, "folder thumbnail unavailable")
		return
	}
	if !a.requireMediaRead(w, r, current(r), refs[0].MediaID) {
		return
	}
	target := a.folderThumbnailPath(folder.ID)
	if _, err := os.Stat(target); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			problem(w, http.StatusInternalServerError, "could not read folder thumbnail")
			return
		}
		problem(w, http.StatusNotFound, "folder thumbnail not generated yet")
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, target)
}

func (a *API) folderThumbnailRefs(ctx context.Context, folderID, limit int) []domain.ThumbnailRef {
	provider, ok := a.Store.(interface {
		FolderThumbnailRefs(context.Context, int, int) ([]domain.ThumbnailRef, error)
	})
	if !ok {
		return nil
	}
	a.folderRefsMu.Lock()
	if entry, hit := a.folderRefs[folderID]; hit && time.Since(entry.at) < folderRefsCacheTTL {
		refs := entry.refs
		a.folderRefsMu.Unlock()
		return refs
	}
	a.folderRefsMu.Unlock()
	refs, err := provider.FolderThumbnailRefs(ctx, folderID, limit)
	if err != nil {
		return nil
	}
	a.folderRefsMu.Lock()
	if a.folderRefs == nil {
		a.folderRefs = map[int]folderRefsEntry{}
	}
	a.folderRefs[folderID] = folderRefsEntry{refs: refs, at: time.Now()}
	a.folderRefsMu.Unlock()
	return refs
}

func (a *API) generateFolderThumbnail(ctx context.Context, folderID int, refs []domain.ThumbnailRef, target string) error {
	width, height := a.thumbnailDimensions(ctx)
	part := width / 3
	if part < 1 {
		part = 1
	}
	targetDir := filepath.Dir(target)
	if err := os.MkdirAll(targetDir, 0o770); err != nil {
		return err
	}
	_ = os.Chmod(targetDir, 0o770)
	items, err := a.mediaForRefs(ctx, refs)
	if err != nil {
		return err
	}
	inputs := []string{}
	filters := []string{}
	for index := 0; index < 3; index++ {
		ref := refs[index%len(refs)]
		item, ok := items[ref.MediaID]
		if !ok {
			return fmt.Errorf("source media %d for folder thumbnail not found", ref.MediaID)
		}
		sourceThumb, err := a.ensureThumbnailForItem(ctx, item, ref.Index)
		if err != nil {
			return err
		}
		inputs = append(inputs, "-i", sourceThumb)
		filters = append(filters, fmt.Sprintf("[%d:v]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d[v%d]", index, part, height, part, height, index))
	}
	temp, err := os.CreateTemp(targetDir, ".folder-thumb-*.jpg")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	_ = temp.Close()
	defer os.Remove(tempName)
	filter := strings.Join(filters, ";") + ";[v0][v1][v2]hstack=inputs=3"
	args := append([]string{"-hide_banner", "-loglevel", "error"}, inputs...)
	args = append(args, "-filter_complex", filter, "-q:v", "3", "-y", tempName)
	if output, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("could not create folder thumbnail: %s", strings.TrimSpace(string(output)))
	}
	_ = os.Chmod(tempName, 0o660)
	if err := os.Rename(tempName, target); err != nil {
		return err
	}
	_ = os.Chmod(target, 0o660)
	if err := a.Store.UpsertFolderThumbnail(ctx, domain.FolderThumbnail{FolderID: folderID, MIMEType: "image/jpeg", Sources: refs}); err != nil {
		return err
	}
	return nil
}

func (a *API) mediaForRefs(ctx context.Context, refs []domain.ThumbnailRef) (map[int]domain.Media, error) {
	if len(refs) == 0 {
		return map[int]domain.Media{}, nil
	}
	ids := make([]int, 0, len(refs))
	seen := make(map[int]bool, len(refs))
	for _, ref := range refs {
		if seen[ref.MediaID] {
			continue
		}
		seen[ref.MediaID] = true
		ids = append(ids, ref.MediaID)
	}
	items, err := a.Store.MediaBatch(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[int]domain.Media, len(items))
	for _, item := range items {
		out[item.ID] = item
	}
	return out, nil
}

func (a *API) ensureThumbnailForItem(ctx context.Context, item domain.Media, index int) (string, error) {
	target := a.thumbnailPath(item.ID, index)
	if usableThumbnail(target) {
		_ = a.Store.UpsertThumbnail(ctx, domain.Thumbnail{
			MediaID: item.ID, Index: index, Path: target, MIMEType: "image/jpeg",
		})
		return target, nil
	}
	if item.ThumbnailError != "" {
		return "", fmt.Errorf("thumbnail skipped because previous error is not cleared: %s", item.ThumbnailError)
	}
	source, err := a.mediaSourcePath(ctx, item)
	if err != nil {
		return "", err
	}
	if item.Kind == domain.KindVideo {
		err = a.generateVideoThumbnail(ctx, source, item, index, target)
	} else {
		err = a.generateImageThumbnail(ctx, source, item.ID, index, target)
	}
	if err != nil {
		return "", err
	}
	_ = a.Store.UpsertThumbnail(ctx, domain.Thumbnail{
		MediaID: item.ID, Index: index, Path: target, MIMEType: "image/jpeg",
	})
	return target, nil
}

func (a *API) mediaSourcePath(ctx context.Context, item domain.Media) (string, error) {
	if item.Path != "" {
		return item.Path, nil
	}
	folder, err := a.Store.Folder(ctx, item.FolderID)
	if err != nil {
		return "", err
	}
	return filepath.Join(folder.Path, item.Name), nil
}

func (a *API) openMedia(w http.ResponseWriter, r *http.Request) (domain.Media, *os.File, bool) {
	p := current(r)
	id, ok := pathID(w, r, "id")
	if !ok {
		return domain.Media{}, nil, false
	}
	item, err := a.Store.Media(r.Context(), id)
	if err != nil {
		problem(w, 404, "media not found")
		return item, nil, false
	}
	if !a.requireMediaRead(w, r, p, item.ID) {
		return item, nil, false
	}
	target := item.Path
	if target == "" {
		folder, err := a.Store.Folder(r.Context(), item.FolderID)
		if err != nil {
			problem(w, 404, "media folder not found")
			return item, nil, false
		}
		target = filepath.Join(folder.Path, item.Name)
	}
	target, _ = filepath.Abs(target)
	file, err := os.Open(target)
	if err != nil {
		problem(w, 404, "media file unavailable")
		return item, nil, false
	}
	return item, file, true
}

func (a *API) mapItems(w http.ResponseWriter, r *http.Request) {
	p := current(r)
	libraryID, folderID := 0, 0
	if raw := r.URL.Query().Get("library"); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil || id <= 0 {
			problem(w, http.StatusBadRequest, "invalid library")
			return
		}
		if !a.requireRead(w, r, p, id) {
			return
		}
		libraryID = id
	}
	if raw := r.URL.Query().Get("folder"); raw != "" {
		fid, err := strconv.Atoi(raw)
		if err != nil || fid <= 0 {
			problem(w, http.StatusBadRequest, "invalid folder")
			return
		}
		if libraryID <= 0 {
			problem(w, http.StatusBadRequest, "folder requires library")
			return
		}
		if _, err := a.Store.EntriesForFolder(r.Context(), p.ID, libraryID, fid); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				problem(w, http.StatusNotFound, "folder not found in library")
				return
			}
			problem(w, http.StatusInternalServerError, err.Error())
			return
		}
		folderID = fid
	}
	items, err := func() ([]domain.MapMedia, error) {
		raw := r.URL.Query().Get("bbox")
		if raw == "" {
			return a.Store.GeotaggedMedia(r.Context(), p.ID, p.Role == domain.RoleAdmin, libraryID, folderID)
		}
		parts := strings.Split(raw, ",")
		if len(parts) != 4 {
			problem(w, http.StatusBadRequest, "invalid bbox, want west,south,east,north")
			return nil, errSkip
		}
		values := make([]float64, 4)
		for i, part := range parts {
			value, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
			if err != nil {
				problem(w, http.StatusBadRequest, "invalid bbox")
				return nil, errSkip
			}
			values[i] = value
		}
		if values[0] < -180 || values[2] > 180 || values[1] < -90 || values[3] > 90 || values[0] >= values[2] || values[1] >= values[3] {
			problem(w, http.StatusBadRequest, "invalid bbox")
			return nil, errSkip
		}
		return a.Store.MediaInArea(r.Context(), p.ID, p.Role == domain.RoleAdmin, libraryID, folderID, domain.Bounds{
			West: values[0], South: values[1], East: values[2], North: values[3],
		})
	}()
	if errors.Is(err, errSkip) {
		return
	}
	if err != nil {
		problem(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, items)
}

func (a *API) createLibrary(w http.ResponseWriter, r *http.Request) {
	name, roots, ok := a.libraryInput(w, r)
	if !ok {
		return
	}
	library := scanner.NewLibrary(name, roots)
	library, err := a.Store.CreateLibrary(r.Context(), library)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			problem(w, http.StatusConflict, "library name already exists")
			return
		}
		problem(w, 409, err.Error())
		return
	}
	writeJSON(w, 201, library)
}

func (a *API) updateLibrary(w http.ResponseWriter, r *http.Request) {
	name, roots, ok := a.libraryInput(w, r)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	existing, err := a.Store.Library(r.Context(), id)
	if err != nil {
		problem(w, 404, "library not found")
		return
	}
	existing.Name = name
	existing.Roots = roots
	if err := a.Store.UpdateLibrary(r.Context(), existing); err != nil {
		if errors.Is(err, store.ErrConflict) {
			problem(w, http.StatusConflict, "library name already exists")
			return
		}
		problem(w, 409, err.Error())
		return
	}
	updated, _ := a.Store.Library(r.Context(), existing.ID)
	writeJSON(w, 200, updated)
}

func (a *API) deleteLibrary(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	thumbnailRefs, err := a.Store.ThumbnailCleanupRefsForLibrary(r.Context(), id)
	if err != nil {
		problem(w, 404, "library not found")
		return
	}
	if err := a.Store.DeleteLibrary(r.Context(), id); err != nil {
		problem(w, 404, "library not found")
		return
	}
	a.cleanupThumbnailRefs(r.Context(), thumbnailRefs)
	if err := a.Store.DeleteScheduledTasksForLibrary(r.Context(), id); err != nil {
		applog.Printf(applog.Error, "could not remove scheduled tasks for deleted library %d: %v", id, err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) libraryInput(w http.ResponseWriter, r *http.Request) (string, []domain.LibraryRoot, bool) {
	var input struct {
		Name  string `json:"name"`
		Roots []struct {
			Path string `json:"path"`
		} `json:"roots"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Name) == "" || len(input.Roots) == 0 {
		problem(w, 400, "name and at least one root are required")
		return "", nil, false
	}
	roots := make([]domain.LibraryRoot, 0, len(input.Roots))
	paths := map[string]bool{}
	for _, root := range input.Roots {
		root.Path = strings.TrimSpace(root.Path)
		if root.Path == "" {
			problem(w, 400, "root paths are required")
			return "", nil, false
		}
		canonicalPath, err := a.Scanner.NormalizeRoot(root.Path)
		if err != nil {
			problem(w, 400, "invalid root path: "+err.Error())
			return "", nil, false
		}
		root.Path = canonicalPath
		if paths[root.Path] {
			problem(w, 400, "root paths must be unique")
			return "", nil, false
		}
		paths[root.Path] = true
		roots = append(roots, domain.LibraryRoot{ID: domain.InvalidID, Path: root.Path})
	}
	return strings.TrimSpace(input.Name), roots, true
}

func (a *API) scanLibrary(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	library, err := a.Store.Library(r.Context(), id)
	if err != nil {
		problem(w, 404, "library not found")
		return
	}
	job := a.startScanJob(library)
	writeJSON(w, http.StatusAccepted, job)
}

func (a *API) thumbnailLibrary(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	library, err := a.Store.Library(r.Context(), id)
	if err != nil {
		problem(w, 404, "library not found")
		return
	}
	var input struct {
		RecreateExisting bool `json:"recreateExisting"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			problem(w, http.StatusBadRequest, "invalid thumbnail refresh request")
			return
		}
	}
	job := a.startThumbnailJob(library, input.RecreateExisting)
	writeJSON(w, http.StatusAccepted, job)
}

func (a *API) startScanJob(library domain.Library) JobStatus {
	return a.startJob(a.newJob("scan", library, nil), func(job *JobStatus) error {
		return a.runScanJob(job, library)
	})
}

func (a *API) runScanJob(job *JobStatus, library domain.Library) error {
	ctx := a.jobContext(context.Background(), job.ID)
	thumbnailRefs, err := a.Store.ThumbnailCleanupRefsForLibrary(ctx, library.ID)
	if err != nil {
		return err
	}
	total, err := a.Scanner.CountMediaFiles(ctx, library)
	if err != nil {
		return err
	}
	a.updateJob(job.ID, func(job *JobStatus) { job.Total = total })
	scanner := a.Scanner.WithProgress(func(path string, media bool) error {
		// The walk (media=false) blocks while the job is paused. Media imports
		// run on the shared worker pool, which parks paused jobs' work, so they
		// must not block here.
		if !media {
			if err := a.waitJobRunnable(context.Background(), job.ID); err != nil {
				return err
			}
		}
		a.updateJob(job.ID, func(job *JobStatus) {
			job.CurrentPath = path
			if media {
				job.Processed++
			}
		})
		return nil
	}).WithPool(job.ID, a.WorkerPool, func() bool {
		return a.jobPaused(job.ID)
	})
	if err := scanner.Scan(ctx, library); err != nil {
		return err
	}
	a.cleanupThumbnailRefs(ctx, thumbnailRefs)
	a.startThumbnailJob(library)
	return nil
}

func (a *API) startThumbnailJob(library domain.Library, recreateExisting ...bool) JobStatus {
	recreate := len(recreateExisting) > 0 && recreateExisting[0]
	return a.startJob(a.newJob("thumbnail-create", library, map[string]any{"recreateExisting": recreate}), func(job *JobStatus) error {
		return a.runThumbnailJob(job, library, recreate)
	})
}

func (a *API) cleanupOrphanThumbnails(w http.ResponseWriter, r *http.Request) {
	job := a.startOrphanThumbnailCleanupJob()
	writeJSON(w, http.StatusAccepted, job)
}

func (a *API) startOrphanThumbnailCleanupJob() JobStatus {
	return a.startJob(a.newJob("orphan-thumbnail-cleanup", domain.Library{ID: 0, Name: "All libraries"}, nil), func(job *JobStatus) error {
		return a.runOrphanThumbnailCleanupJob(job)
	})
}

func (a *API) runOrphanThumbnailCleanupJob(job *JobStatus) error {
	ctx := a.jobContext(context.Background(), job.ID)
	type cleanupTask struct {
		path     string
		mediaID  int
		index    int
		folderID int
	}
	tasks := []cleanupTask{}
	root := a.thumbnailRoot()
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if entry.Name() == "folders" {
			folderRoot := filepath.Join(root, "folders")
			folderEntries, err := os.ReadDir(folderRoot)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			for _, folderEntry := range folderEntries {
				if !folderEntry.IsDir() {
					continue
				}
				files, err := os.ReadDir(filepath.Join(folderRoot, folderEntry.Name()))
				if err != nil {
					return err
				}
				for _, file := range files {
					if file.IsDir() || strings.ToLower(filepath.Ext(file.Name())) != ".jpg" {
						continue
					}
					folderID, _, ok := parseIndexedThumbnailName(file.Name())
					if !ok {
						continue
					}
					tasks = append(tasks, cleanupTask{path: filepath.Join(folderRoot, folderEntry.Name(), file.Name()), folderID: folderID})
				}
			}
			continue
		}
		if entry.Name() == "media" {
			mediaRoot := filepath.Join(root, "media")
			mediaEntries, err := os.ReadDir(mediaRoot)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			for _, mediaEntry := range mediaEntries {
				if !mediaEntry.IsDir() {
					continue
				}
				files, err := os.ReadDir(filepath.Join(mediaRoot, mediaEntry.Name()))
				if err != nil {
					return err
				}
				for _, file := range files {
					if file.IsDir() || strings.ToLower(filepath.Ext(file.Name())) != ".jpg" {
						continue
					}
					mediaID, index, ok := parseIndexedThumbnailName(file.Name())
					if !ok {
						continue
					}
					tasks = append(tasks, cleanupTask{path: filepath.Join(mediaRoot, mediaEntry.Name(), file.Name()), mediaID: mediaID, index: index})
				}
			}
			continue
		}
	}
	a.updateJob(job.ID, func(job *JobStatus) { job.Total = len(tasks) })
	for _, task := range tasks {
		if err := a.waitJobRunnable(ctx, job.ID); err != nil {
			return err
		}
		a.updateJob(job.ID, func(job *JobStatus) { job.CurrentPath = task.path })
		keep := false
		if task.folderID != 0 {
			_, err = a.Store.FolderThumbnailFile(ctx, task.folderID)
			keep = err == nil
		} else {
			_, err = a.Store.Thumbnail(ctx, task.mediaID, task.index)
			keep = err == nil
		}
		if !keep {
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				return err
			}
			if err := os.Remove(task.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		a.updateJob(job.ID, func(job *JobStatus) { job.Processed++ })
	}
	return nil
}

func (a *API) runThumbnailJob(job *JobStatus, library domain.Library, recreate bool) error {
	ctx := a.jobContext(context.Background(), job.ID)
	items, err := a.Store.MediaForLibrary(ctx, 0, library.ID)
	if err != nil {
		return err
	}
	folders, err := a.Store.FoldersForLibrary(ctx, library.ID)
	if err != nil {
		return err
	}
	total := 0
	for _, item := range items {
		if item.Kind == domain.KindVideo {
			total += len(a.videoThumbnailTimes(context.Background(), item))
		} else {
			total++
		}
	}
	total += len(folders)
	a.updateJob(job.ID, func(job *JobStatus) { job.Total = total })
	type thumbnailTask struct {
		item  domain.Media
		index int
	}
	tasks := []thumbnailTask{}
	for _, item := range items {
		indexes := []int{0}
		if item.Kind == domain.KindVideo {
			times := a.videoThumbnailTimes(context.Background(), item)
			indexes = make([]int, len(times))
			for index := range times {
				indexes[index] = index
			}
		}
		for _, index := range indexes {
			tasks = append(tasks, thumbnailTask{item: item, index: index})
		}
	}
	if len(tasks) > 0 {
		work := make([]jobpool.Work, len(tasks))
		for i, task := range tasks {
			task := task
			work[i] = func(ctx context.Context) error {
				item := task.item
				index := task.index
				a.updateJob(job.ID, func(job *JobStatus) { job.CurrentPath = item.RelativePath })
				if item.ThumbnailError != "" {
					a.updateJob(job.ID, func(job *JobStatus) {
						job.Error = fmt.Sprintf("%s: skipped thumbnail because previous error is not cleared: %s", item.RelativePath, item.ThumbnailError)
						job.Processed++
					})
					return nil
				}
				if recreate {
					_ = os.Remove(a.thumbnailPath(item.ID, index))
				}
				if _, err := a.ensureThumbnailForItem(ctx, item, index); err != nil {
					applog.Printf(applog.Error, "thumbnail failed for %s: %s", item.RelativePath, err)
					_ = a.Store.SetMediaActionError(ctx, item.ID, "thumbnail", err.Error())
					a.updateJob(job.ID, func(job *JobStatus) {
						job.Error = fmt.Sprintf("%s: %v", item.RelativePath, err)
						job.Processed++
					})
					return nil
				}
				a.updateJob(job.ID, func(job *JobStatus) { job.Processed++ })
				return nil
			}
		}
		if err := a.runWork(job.ID, ctx, work); err != nil {
			return err
		}
	}
	for _, folder := range folders {
		if err := a.waitJobRunnable(ctx, job.ID); err != nil {
			return err
		}
		a.updateJob(job.ID, func(job *JobStatus) { job.CurrentPath = folder.RelativePath })
		refs, err := a.Store.FolderThumbnailRefs(ctx, folder.ID, 3)
		if err != nil || len(refs) == 0 {
			a.updateJob(job.ID, func(job *JobStatus) { job.Processed++ })
			continue
		}
		target := a.folderThumbnailPath(folder.ID)
		if recreate {
			_ = os.Remove(target)
		}
		if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
			err = a.generateFolderThumbnail(ctx, folder.ID, refs, target)
		} else if err == nil {
			err = a.Store.UpsertFolderThumbnail(ctx, domain.FolderThumbnail{FolderID: folder.ID, MIMEType: "image/jpeg", Sources: refs})
		}
		if err != nil {
			applog.Printf(applog.Error, "folder thumbnail failed for %s: %s", folder.RelativePath, err)
			a.updateJob(job.ID, func(job *JobStatus) { job.Error = fmt.Sprintf("%s: %v", folder.RelativePath, err) })
		}
		a.updateJob(job.ID, func(job *JobStatus) { job.Processed++ })
	}
	return nil
}

func (a *API) newJob(kind string, library domain.Library, options map[string]any) JobStatus {
	rootPath := ""
	if len(library.Roots) > 0 {
		rootPath = library.Roots[0].Path
	}
	now := time.Now()
	return JobStatus{
		ID: strconv.FormatInt(now.UnixNano(), 36), Category: kind, Type: kind, LibraryID: library.ID, LibraryName: library.Name,
		RootPath: rootPath, Status: "running", Cancelable: true, StartedAt: now, Options: options,
	}
}

func (a *API) startJob(job JobStatus, run func(*JobStatus) error) JobStatus {
	ctx, cancel := context.WithCancel(context.Background())
	a.jobMu.Lock()
	if a.jobs == nil {
		a.jobs = map[string]*JobStatus{}
	}
	if a.jobCancels == nil {
		a.jobCancels = map[string]context.CancelFunc{}
	}
	if a.jobContexts == nil {
		a.jobContexts = map[string]context.Context{}
	}
	copy := job
	a.jobs[job.ID] = &copy
	a.jobCancels[job.ID] = cancel
	a.jobContexts[job.ID] = ctx
	a.jobMu.Unlock()
	if err := a.Store.SaveJob(context.Background(), copy); err != nil {
		applog.Printf(applog.Error, "could not persist job %s: %v", copy.ID, err)
	}
	go a.runJob(&copy, run, cancel)
	return copy
}

func (a *API) runJob(snapshot *JobStatus, run func(*JobStatus) error, cancel context.CancelFunc) {
	defer cancel()
	err := run(snapshot)
	now := time.Now()
	a.updateJob(snapshot.ID, func(job *JobStatus) {
		job.FinishedAt = &now
		job.Cancelable = false
		job.Paused = false
		if err != nil {
			if errors.Is(err, context.Canceled) {
				job.Status = "cancelled"
			} else {
				job.Status = "failed"
				job.Error = err.Error()
				applog.Printf(applog.Error, "%s job %s failed: %v", job.Category, job.ID, err)
			}
		} else {
			job.Status = "done"
		}
	})
	a.jobMu.Lock()
	delete(a.jobCancels, snapshot.ID)
	delete(a.jobContexts, snapshot.ID)
	a.jobMu.Unlock()
}

func (a *API) recoverJobs() {
	jobs, err := a.Store.UnfinishedJobs(context.Background())
	if err != nil {
		applog.Printf(applog.Error, "could not load unfinished jobs: %v", err)
		return
	}
	for _, job := range jobs {
		if job.Status == "cancelling" {
			now := time.Now()
			job.Status = "cancelled"
			job.Paused = false
			job.Cancelable = false
			job.FinishedAt = &now
			_ = a.Store.SaveJob(context.Background(), job)
			continue
		}
		library := domain.Library{ID: 0, Name: "All libraries"}
		if job.LibraryID != 0 {
			var err error
			library, err = a.Store.Library(context.Background(), job.LibraryID)
			if err != nil {
				now := time.Now()
				job.Status = "failed"
				job.Error = "library not found during job recovery"
				job.Cancelable = false
				job.FinishedAt = &now
				_ = a.Store.SaveJob(context.Background(), job)
				continue
			}
		}
		if job.Status != "paused" {
			job.Status = "running"
			job.Paused = false
		}
		job.Cancelable = true
		if job.Options == nil {
			job.Options = map[string]any{}
		}
		switch job.Category {
		case "scan":
			a.startJob(job, func(job *JobStatus) error { return a.runScanJob(job, library) })
		case "thumbnail-create":
			recreate, _ := job.Options["recreateExisting"].(bool)
			a.startJob(job, func(job *JobStatus) error { return a.runThumbnailJob(job, library, recreate) })
		case "orphan-thumbnail-cleanup":
			a.startJob(job, func(job *JobStatus) error { return a.runOrphanThumbnailCleanupJob(job) })
		case "vacuum":
			now := time.Now()
			job.Status = "failed"
			job.Error = "vacuum interrupted by restart"
			job.Paused = false
			job.Cancelable = false
			job.FinishedAt = &now
			_ = a.Store.SaveJob(context.Background(), job)
		}
	}
}

func (a *API) jobContext(parent context.Context, id string) context.Context {
	a.jobMu.Lock()
	base := a.jobContexts[id]
	a.jobMu.Unlock()
	if base == nil {
		return parent
	}
	ctx, cancel := context.WithCancel(parent)
	go func() {
		defer cancel()
		select {
		case <-parent.Done():
		case <-base.Done():
		}
	}()
	return ctx
}

func (a *API) waitJobRunnable(ctx context.Context, id string) error {
	for {
		a.jobMu.Lock()
		job := a.jobs[id]
		paused := job != nil && job.Paused
		cancelling := job != nil && job.Status == "cancelling"
		a.jobMu.Unlock()
		if cancelling {
			return context.Canceled
		}
		if !paused {
			return ctx.Err()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// jobPaused reports the job's current pause state.
func (a *API) jobPaused(id string) bool {
	a.jobMu.Lock()
	defer a.jobMu.Unlock()
	job := a.jobs[id]
	return job != nil && job.Paused
}

// runWork runs work on the shared worker pool, or sequentially when no pool is
// configured (e.g. in tests). It returns when all work finished, the first
// work item failed, or the job's context was cancelled.
func (a *API) runWork(jobID string, ctx context.Context, work []jobpool.Work) error {
	if a.WorkerPool == nil {
		for _, fn := range work {
			if err := fn(ctx); err != nil {
				return err
			}
		}
		return nil
	}
	a.WorkerPool.Submit(jobID, ctx, a.jobPaused(jobID), work)
	return a.WorkerPool.Wait(ctx, jobID)
}

func (a *API) updateJob(id string, update func(*JobStatus)) {
	var copy JobStatus
	a.jobMu.Lock()
	if job := a.jobs[id]; job != nil {
		update(job)
		copy = *job
	}
	a.jobMu.Unlock()
	if shouldPersistJobProgress(copy) {
		if err := a.Store.SaveJob(context.Background(), copy); err != nil {
			applog.Printf(applog.Error, "could not persist job %s: %v", copy.ID, err)
		}
	}
}

func shouldPersistJobProgress(job JobStatus) bool {
	if job.ID == "" {
		return false
	}
	if job.FinishedAt != nil || job.Status == "paused" || job.Status == "cancelling" || job.Status == "cancelled" || job.Status == "failed" || job.Status == "done" {
		return true
	}
	if job.Total > 0 && job.Processed == 0 {
		return true
	}
	return job.Processed%10 == 0
}

func (a *API) jobsList(w http.ResponseWriter, r *http.Request) {
	a.pruneFinishedJobs(time.Now())
	items, err := a.Store.Jobs(r.Context())
	if err != nil {
		problem(w, http.StatusInternalServerError, "could not read jobs")
		return
	}
	a.jobMu.Lock()
	live := map[string]JobStatus{}
	for id, job := range a.jobs {
		live[id] = *job
	}
	a.jobMu.Unlock()
	seen := map[string]bool{}
	for index := range items {
		if job, ok := live[items[index].ID]; ok {
			items[index] = job
		}
		seen[items[index].ID] = true
	}
	for id, job := range live {
		if !seen[id] {
			items = append(items, job)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].StartedAt.After(items[j].StartedAt) })
	writeJSON(w, http.StatusOK, items)
}

func (a *API) scheduledTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := a.Store.ScheduledTasks(r.Context())
	if err != nil {
		problem(w, http.StatusInternalServerError, "could not read scheduled tasks")
		return
	}
	writeJSON(w, http.StatusOK, tasks)
}

func (a *API) createScheduledTask(w http.ResponseWriter, r *http.Request) {
	task, ok := a.scheduledTaskInput(w, r, false)
	if !ok {
		return
	}
	created, err := a.Store.CreateScheduledTask(r.Context(), task)
	if err != nil {
		problem(w, http.StatusInternalServerError, "could not create scheduled task")
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (a *API) updateScheduledTask(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	task, ok := a.scheduledTaskInput(w, r, true)
	if !ok {
		return
	}
	task.ID = id
	existing, err := a.Store.ScheduledTask(r.Context(), id)
	if err != nil {
		problem(w, statusFor(err), "scheduled task not found")
		return
	}
	task.LastRunAt = existing.LastRunAt
	if err := a.Store.UpdateScheduledTask(r.Context(), task); err != nil {
		problem(w, statusFor(err), "scheduled task not found")
		return
	}
	tasks, err := a.Store.ScheduledTasks(r.Context())
	if err != nil {
		problem(w, http.StatusInternalServerError, "could not read scheduled tasks")
		return
	}
	for _, item := range tasks {
		if item.ID == id {
			writeJSON(w, http.StatusOK, item)
			return
		}
	}
	writeJSON(w, http.StatusOK, task)
}

func (a *API) deleteScheduledTask(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := a.Store.DeleteScheduledTask(r.Context(), id); err != nil {
		problem(w, statusFor(err), "scheduled task not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) scheduledTaskInput(w http.ResponseWriter, r *http.Request, editing bool) (domain.ScheduledTask, bool) {
	var input struct {
		Name      string `json:"name"`
		TaskType  string `json:"taskType"`
		LibraryID int    `json:"libraryId"`
		Cron      string `json:"cron"`
		Enabled   *bool  `json:"enabled"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		problem(w, http.StatusBadRequest, "invalid JSON")
		return domain.ScheduledTask{}, false
	}
	task := domain.ScheduledTask{
		Name:      strings.TrimSpace(input.Name),
		TaskType:  strings.TrimSpace(input.TaskType),
		LibraryID: input.LibraryID,
		Cron:      strings.TrimSpace(input.Cron),
		Enabled:   true,
	}
	if input.Enabled != nil {
		task.Enabled = *input.Enabled
	}
	if task.Name == "" {
		problem(w, http.StatusBadRequest, "name is required")
		return domain.ScheduledTask{}, false
	}
	switch task.TaskType {
	case "scan", "thumbnail-create":
		if task.LibraryID == 0 {
			problem(w, http.StatusBadRequest, "libraryId is required for scan and thumbnail tasks")
			return domain.ScheduledTask{}, false
		}
	case "vacuum":
		if !editing {
			task.LibraryID = 0
		}
	default:
		problem(w, http.StatusBadRequest, "taskType must be one of: scan, thumbnail-create, vacuum")
		return domain.ScheduledTask{}, false
	}
	now := task.NextRunAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	next, err := scheduler.Next(task.Cron, now)
	if err != nil {
		problem(w, http.StatusBadRequest, "invalid cron expression: "+err.Error())
		return domain.ScheduledTask{}, false
	}
	task.NextRunAt = next
	return task, true
}

func (a *API) StartScan(library domain.Library) error {
	a.startScanJob(library)
	return nil
}

func (a *API) StartThumbnails(library domain.Library) error {
	a.startThumbnailJob(library)
	return nil
}

func (a *API) StartVacuum() error {
	if _, ok := a.startVacuumJob(); !ok {
		applog.Printf(applog.Warn, "vacuum is not supported by the configured database store; skipping scheduled vacuum")
	}
	return nil
}

// vacuumDB starts a database vacuum in the background, like the scheduled
// vacuum task, but on demand from the admin panel.
func (a *API) vacuumDB(w http.ResponseWriter, r *http.Request) {
	job, ok := a.startVacuumJob()
	if !ok {
		problem(w, http.StatusBadRequest, "vacuum is not supported by the configured database store")
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

// startVacuumJob starts a database vacuum background job. The second return
// value is false when the configured store does not support vacuuming.
func (a *API) startVacuumJob() (JobStatus, bool) {
	provider, ok := a.Store.(interface {
		Vacuum(context.Context) error
	})
	if !ok {
		return JobStatus{}, false
	}
	job := a.newJob("vacuum", domain.Library{ID: 0, Name: "Database maintenance"}, nil)
	job.Cancelable = false
	return a.startJob(job, func(job *JobStatus) error {
		job.Total = 1
		if err := provider.Vacuum(context.Background()); err != nil {
			return err
		}
		job.Processed = 1
		return nil
	}), true
}

func (a *API) logs(w http.ResponseWriter, r *http.Request) {
	limit := 300
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 2000 {
			problem(w, http.StatusBadRequest, "log limit must be between 1 and 2000")
			return
		}
		limit = parsed
	}
	path := a.LogFile
	if strings.TrimSpace(path) == "" {
		path = "/runtime/app-config/logs/app.log"
	}
	lines, err := tailLogLines(path, limit)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeJSON(w, http.StatusOK, map[string]any{"path": path, "lines": []string{}})
			return
		}
		problem(w, http.StatusInternalServerError, "could not read logs")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": path, "lines": lines})
}

// downloadLogs streams the full application log file as an attachment.
func (a *API) downloadLogs(w http.ResponseWriter, r *http.Request) {
	path := a.LogFile
	if strings.TrimSpace(path) == "" {
		path = "/runtime/app-config/logs/app.log"
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(path)))
	http.ServeFile(w, r, path)
	applog.Printf(applog.Info, "application log file downloaded by admin user %d", current(r).ID)
}

// clearLogs empties the application log file. The API keeps its file handle
// open for appending, so truncating in place is enough to reset it.
func (a *API) clearLogs(w http.ResponseWriter, r *http.Request) {
	path := a.LogFile
	if strings.TrimSpace(path) == "" {
		path = "/runtime/app-config/logs/app.log"
	}
	err := applog.ClearFile()
	if err != nil && !errors.Is(err, applog.ErrNotConfigured) {
		problem(w, http.StatusInternalServerError, "could not clear log file")
		return
	}
	if errors.Is(err, applog.ErrNotConfigured) {
		file, openErr := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o600)
		if openErr != nil {
			problem(w, http.StatusInternalServerError, "could not open log file")
			return
		}
		if err := file.Truncate(0); err != nil {
			_ = file.Close()
			problem(w, http.StatusInternalServerError, "could not clear log file")
			return
		}
		if err := file.Close(); err != nil {
			problem(w, http.StatusInternalServerError, "could not close log file")
			return
		}
	}
	applog.Printf(applog.Info, "application logs cleared by admin user %d", current(r).ID)
	writeJSON(w, http.StatusOK, map[string]any{"path": path})
}

func remoteAddr(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (a *API) shutdown(w http.ResponseWriter, r *http.Request) {
	mode := strings.TrimSpace(r.URL.Query().Get("mode"))
	if mode == "" {
		mode = "docker"
	}
	if mode != "docker" && mode != "signal" {
		problem(w, http.StatusBadRequest, "shutdown mode must be docker or signal")
		return
	}
	if mode == "docker" && a.ContainerStop == nil {
		problem(w, http.StatusServiceUnavailable, "docker container stop is not available")
		return
	}
	if mode == "signal" && a.Shutdown == nil {
		problem(w, http.StatusServiceUnavailable, "server shutdown is not available")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "stopping", "mode": mode})
	go func() {
		time.Sleep(200 * time.Millisecond)
		applog.Printf(applog.Warn, "server shutdown requested by admin using %s mode", mode)
		if mode == "docker" {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := a.ContainerStop(ctx); err != nil {
				applog.Printf(applog.Error, "docker container stop failed: %s", err)
			}
			return
		}
		a.Shutdown()
	}()
}

func tailLogLines(path string, limit int) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := strings.TrimRight(string(data), "\n")
	if text == "" {
		return []string{}, nil
	}
	lines := strings.Split(text, "\n")
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return lines, nil
}

func (a *API) pruneFinishedJobs(now time.Time) {
	keepFinishedFor := time.Duration(a.serverSettings(context.Background()).FinishedJobRetentionMinutes) * time.Minute
	if err := a.Store.DeleteFinishedJobsBefore(context.Background(), now.Add(-keepFinishedFor)); err != nil {
		applog.Printf(applog.Error, "could not prune finished jobs: %v", err)
	}
	a.jobMu.Lock()
	defer a.jobMu.Unlock()
	for id, job := range a.jobs {
		if job.FinishedAt == nil {
			continue
		}
		if now.Sub(*job.FinishedAt) > keepFinishedFor {
			delete(a.jobs, id)
			delete(a.jobCancels, id)
			delete(a.jobContexts, id)
		}
	}
}

func (a *API) pauseJob(w http.ResponseWriter, r *http.Request) {
	job, ok := a.controlJob(r.PathValue("id"), func(job *JobStatus) error {
		if !job.Cancelable || job.Status != "running" {
			return ErrInvalidJobState
		}
		job.Paused = true
		job.Status = "paused"
		return nil
	})
	if !ok {
		problem(w, http.StatusNotFound, "job not found")
		return
	}
	if job.Error == ErrInvalidJobState.Error() {
		problem(w, http.StatusConflict, "job cannot be paused")
		return
	}
	a.WorkerPool.SetJobPaused(job.ID, true)
	writeJSON(w, http.StatusOK, job)
}

func (a *API) resumeJob(w http.ResponseWriter, r *http.Request) {
	job, ok := a.controlJob(r.PathValue("id"), func(job *JobStatus) error {
		if !job.Cancelable || !job.Paused {
			return ErrInvalidJobState
		}
		job.Paused = false
		job.Status = "running"
		return nil
	})
	if !ok {
		problem(w, http.StatusNotFound, "job not found")
		return
	}
	if job.Error == ErrInvalidJobState.Error() {
		problem(w, http.StatusConflict, "job cannot be resumed")
		return
	}
	a.WorkerPool.SetJobPaused(job.ID, false)
	writeJSON(w, http.StatusOK, job)
}

func (a *API) cancelJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var cancel context.CancelFunc
	job, ok := a.controlJob(id, func(job *JobStatus) error {
		if !job.Cancelable || job.Status == "done" || job.Status == "failed" || job.Status == "cancelled" {
			return ErrInvalidJobState
		}
		job.Paused = false
		job.Status = "cancelling"
		cancel = a.jobCancels[id]
		return nil
	})
	if !ok {
		problem(w, http.StatusNotFound, "job not found")
		return
	}
	if job.Error == ErrInvalidJobState.Error() {
		problem(w, http.StatusConflict, "job cannot be cancelled")
		return
	}
	if cancel != nil {
		cancel()
	}
	a.WorkerPool.CancelJob(id)
	writeJSON(w, http.StatusOK, job)
}

var (
	ErrInvalidJobState = errors.New("invalid job state")
	// errSkip unwinds the map handler closure after a response has already been
	// written (e.g. a 400 for a malformed bbox).
	errSkip = errors.New("skip")
)

func (a *API) controlJob(id string, update func(*JobStatus) error) (JobStatus, bool) {
	var copy JobStatus
	a.jobMu.Lock()
	job := a.jobs[id]
	if job == nil {
		a.jobMu.Unlock()
		return JobStatus{}, false
	}
	copy = *job
	if err := update(job); err != nil {
		a.jobMu.Unlock()
		copy.Error = err.Error()
		return copy, true
	}
	copy = *job
	a.jobMu.Unlock()
	if err := a.Store.SaveJob(context.Background(), copy); err != nil {
		applog.Printf(applog.Error, "could not persist job %s: %v", copy.ID, err)
	}
	return copy, true
}

func (a *API) setAccess(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Allowed bool `json:"allowed"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		problem(w, 400, "invalid JSON")
		return
	}
	libraryID, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	userID, ok := pathID(w, r, "userId")
	if !ok {
		return
	}
	if err := a.Store.SetAccess(r.Context(), libraryID, userID, input.Allowed); err != nil {
		problem(w, 404, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) users(w http.ResponseWriter, r *http.Request) {
	users, err := a.Store.Users(r.Context())
	if err != nil {
		problem(w, http.StatusInternalServerError, "could not read users")
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (a *API) createUser(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Login    string      `json:"login"`
		Role     domain.Role `json:"role"`
		Password string      `json:"password"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		problem(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	user, err := a.Store.CreateUser(r.Context(), domain.User{Login: input.Login, Role: input.Role}, input.Password)
	if err != nil {
		problem(w, http.StatusBadRequest, "login, role, and 12+ character password are required")
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

func (a *API) updateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var input struct {
		Login    string      `json:"login"`
		Role     domain.Role `json:"role"`
		Password string      `json:"password"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		problem(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	user, err := a.Store.UpdateUser(r.Context(), domain.User{ID: id, Login: input.Login, Role: input.Role}, input.Password)
	if err != nil {
		problem(w, statusFor(err), "user not found or invalid")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (a *API) libraryAccess(w http.ResponseWriter, r *http.Request) {
	libraryID, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	access, err := a.Store.LibraryAccess(r.Context(), libraryID)
	if err != nil {
		problem(w, statusFor(err), "library not found")
		return
	}
	writeJSON(w, http.StatusOK, access)
}

func (a *API) importEmby(w http.ResponseWriter, r *http.Request) {
	var input embyimport.Options
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		problem(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	snapshot, err := embyimport.Read(r.Context(), input)
	if err != nil {
		problem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	result, err := a.Store.ImportSnapshot(r.Context(), snapshot)
	if err != nil {
		problem(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *API) filesystem(w http.ResponseWriter, r *http.Request) {
	target := strings.TrimSpace(r.URL.Query().Get("path"))
	if target == "" {
		target = "/"
	}
	target, err := filepath.Abs(target)
	if err != nil {
		problem(w, http.StatusBadRequest, "invalid path")
		return
	}
	if evaluated, evalErr := filepath.EvalSymlinks(target); evalErr == nil {
		target = evaluated
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		problem(w, http.StatusBadRequest, "directory is not readable")
		return
	}
	directories := make([]map[string]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		directories = append(directories, map[string]string{
			"name": entry.Name(),
			"path": filepath.ToSlash(filepath.Join(target, entry.Name())),
		})
	}
	sort.Slice(directories, func(i, j int) bool { return directories[i]["name"] < directories[j]["name"] })
	writeJSON(w, http.StatusOK, map[string]any{
		"root":        "/",
		"path":        filepath.ToSlash(target),
		"parent":      filesystemParent(target),
		"directories": directories,
	})
}

func filesystemParent(target string) string {
	parent := filepath.Dir(target)
	if filepath.Clean(parent) == filepath.Clean(target) {
		return ""
	}
	return filepath.ToSlash(parent)
}

func (a *API) getSettings(w http.ResponseWriter, r *http.Request) {
	settings := a.serverSettings(r.Context())
	// Without the optional gateway container HTTPS cannot be served, so the
	// effective transport is always HTTP-only and the HTTPS fields are hidden.
	if !a.GatewayEnabled {
		settings.HTTPEnabled = true
		settings.HTTPSEnabled = false
		settings.PublicDNS = ""
		settings.ACMEEmail = ""
	}
	transport := gatewayconfig.FromServerSettings(settings)
	certificateExpiresAt := ""
	if expires, ok := gatewayconfig.CertificateExpiration(a.CaddyDataDir, transport.PublicDNS); ok {
		certificateExpiresAt = expires.Format("2006-01-02")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"httpEnabled":                      settings.HTTPEnabled,
		"httpsEnabled":                     settings.HTTPSEnabled,
		"publicDns":                        settings.PublicDNS,
		"acmeEmail":                        settings.ACMEEmail,
		"httpsCertificateExpiresAt":        certificateExpiresAt,
		"httpsGatewayEnabled":              a.GatewayEnabled,
		"thumbnailWidth":                   settings.ThumbnailWidth,
		"thumbnailHeight":                  settings.ThumbnailHeight,
		"workerPoolSize":                   settings.WorkerPoolSize,
		"videoThumbnailFirstSeconds":       settings.VideoThumbnailFirstSeconds,
		"videoThumbnailMaxCount":           settings.VideoThumbnailMaxCount,
		"videoThumbnailMinIntervalSeconds": settings.VideoThumbnailMinIntervalSeconds,
		"sessionMaxAgeHours":               settings.SessionMaxAgeHours,
		"finishedJobRetentionMinutes":      settings.FinishedJobRetentionMinutes,
		"logLevel":                         settings.LogLevel,
		"logRotateMaxSizeMB":               settings.LogRotateMaxSizeMB,
		"logRotateMaxBackups":              settings.LogRotateMaxBackups,
		"logRotateMaxAgeDays":              settings.LogRotateMaxAgeDays,
		"smtpHost":                         settings.SMTPHost,
		"smtpPort":                         settings.SMTPPort,
		"smtpUsername":                     settings.SMTPUsername,
		"smtpFrom":                         settings.SMTPFrom,
	})
}

func (a *API) updateSettings(w http.ResponseWriter, r *http.Request) {
	input := a.serverSettings(r.Context())
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		problem(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	input.SMTPHost = strings.TrimSpace(input.SMTPHost)
	input.SMTPFrom = strings.TrimSpace(input.SMTPFrom)
	if input.SMTPHost != "" && (input.SMTPPort < 1 || input.SMTPPort > 65535) {
		problem(w, http.StatusBadRequest, "smtpPort must be between 1 and 65535")
		return
	}
	if input.SMTPPort == 0 {
		input.SMTPPort = 587
	}
	transport := gatewayconfig.Settings{
		HTTPEnabled: input.HTTPEnabled, HTTPSEnabled: input.HTTPSEnabled,
		PublicDNS: strings.TrimSpace(input.PublicDNS), ACMEEmail: strings.TrimSpace(input.ACMEEmail),
	}
	if !a.GatewayEnabled {
		// The gateway container is not part of this deployment, so HTTPS does
		// not exist: normalize the effective transport to HTTP-only.
		transport.HTTPEnabled = true
		transport.HTTPSEnabled = false
		transport.PublicDNS = ""
		transport.ACMEEmail = ""
	}
	if err := gatewayconfig.Validate(transport); err != nil {
		problem(w, http.StatusBadRequest, err.Error())
		return
	}
	if input.ThumbnailWidth < 64 || input.ThumbnailWidth > 4096 || input.ThumbnailHeight < 64 || input.ThumbnailHeight > 4096 {
		problem(w, http.StatusBadRequest, "thumbnail width and height must be between 64 and 4096")
		return
	}
	if input.WorkerPoolSize < 1 || input.WorkerPoolSize > 64 {
		problem(w, http.StatusBadRequest, "worker pool size must be between 1 and 64")
		return
	}
	if input.VideoThumbnailFirstSeconds < 0 || input.VideoThumbnailMaxCount < 1 || input.VideoThumbnailMaxCount > domain.MaxVideoThumbnailCount || input.VideoThumbnailMinIntervalSeconds < 1 {
		problem(w, http.StatusBadRequest, "video thumbnail first time must be non-negative, max count must be between 1 and "+strconv.Itoa(domain.MaxVideoThumbnailCount)+", and min interval must be positive")
		return
	}
	if input.SessionMaxAgeHours < 1 || input.SessionMaxAgeHours > 8760 {
		problem(w, http.StatusBadRequest, "session lifetime must be between 1 and 8760 hours")
		return
	}
	if input.FinishedJobRetentionMinutes < 1 || input.FinishedJobRetentionMinutes > 10080 {
		problem(w, http.StatusBadRequest, "finished job retention must be between 1 and 10080 minutes")
		return
	}
	logLevel, err := applog.ParseLevel(input.LogLevel)
	if err != nil {
		problem(w, http.StatusBadRequest, err.Error())
		return
	}
	if input.LogRotateMaxSizeMB < 1 || input.LogRotateMaxSizeMB > 1024 || input.LogRotateMaxBackups < 1 || input.LogRotateMaxBackups > 100 || input.LogRotateMaxAgeDays < 1 || input.LogRotateMaxAgeDays > 3650 {
		problem(w, http.StatusBadRequest, "log rotate size must be 1..1024 MB, backups 1..100, age 1..3650 days")
		return
	}
	input.HTTPEnabled = transport.HTTPEnabled
	input.HTTPSEnabled = transport.HTTPSEnabled
	input.PublicDNS = transport.PublicDNS
	input.ACMEEmail = transport.ACMEEmail
	input.LogLevel = applog.LevelString(logLevel)
	if err := a.Store.SaveServerSettings(r.Context(), input); err != nil {
		problem(w, http.StatusInternalServerError, "could not save settings")
		return
	}
	if a.WorkerPool != nil {
		a.WorkerPool.SetCapacity(input.WorkerPoolSize)
	}
	if a.GatewayConfigPath != "" {
		if err := gatewayconfig.Write(a.GatewayConfigPath, transport); err != nil {
			problem(w, http.StatusInternalServerError, "settings saved but gateway could not be reconfigured")
			return
		}
	}
	applog.SetLevel(logLevel)
	certificateExpiresAt := ""
	if expires, ok := gatewayconfig.CertificateExpiration(a.CaddyDataDir, transport.PublicDNS); ok {
		certificateExpiresAt = expires.Format("2006-01-02")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"httpEnabled": transport.HTTPEnabled,
		"httpsEnabled": transport.HTTPSEnabled, "publicDns": transport.PublicDNS,
		"acmeEmail":                 transport.ACMEEmail,
		"httpsCertificateExpiresAt": certificateExpiresAt,
		"thumbnailWidth":            input.ThumbnailWidth, "thumbnailHeight": input.ThumbnailHeight,
		"workerPoolSize":                   input.WorkerPoolSize,
		"videoThumbnailFirstSeconds":       input.VideoThumbnailFirstSeconds,
		"videoThumbnailMaxCount":           input.VideoThumbnailMaxCount,
		"videoThumbnailMinIntervalSeconds": input.VideoThumbnailMinIntervalSeconds,
		"sessionMaxAgeHours":               input.SessionMaxAgeHours,
		"finishedJobRetentionMinutes":      input.FinishedJobRetentionMinutes,
		"logLevel":                         applog.LevelString(logLevel),
		"logRotateMaxSizeMB":               input.LogRotateMaxSizeMB,
		"logRotateMaxBackups":              input.LogRotateMaxBackups,
		"logRotateMaxAgeDays":              input.LogRotateMaxAgeDays,
		"smtpHost":                         input.SMTPHost,
		"smtpPort":                         input.SMTPPort,
		"smtpUsername":                     input.SMTPUsername,
		"smtpFrom":                         input.SMTPFrom,
	})
}

func (a *API) serverSettings(ctx context.Context) domain.ServerSettings {
	settings, err := a.Store.ServerSettings(ctx)
	if err != nil {
		return domain.DefaultServerSettings()
	}
	return settings
}

func normalizeLogin(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func validLogin(value string) bool {
	if len(value) < 3 || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' ||
			character == '.' || character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
}

func (a *API) requireRead(w http.ResponseWriter, r *http.Request, p principal, libraryID int) bool {
	ok, err := a.Store.CanRead(r.Context(), p.ID, libraryID, p.Role == domain.RoleAdmin)
	if err != nil {
		problem(w, 404, "library not found")
		return false
	}
	if !ok {
		problem(w, 403, "access denied")
		return false
	}
	return true
}

func (a *API) requireMediaRead(w http.ResponseWriter, r *http.Request, p principal, mediaID int) bool {
	ok, err := a.Store.CanReadMedia(r.Context(), p.ID, mediaID, p.Role == domain.RoleAdmin)
	if err != nil {
		problem(w, 404, "media not found")
		return false
	}
	if !ok {
		problem(w, 403, "access denied")
		return false
	}
	return true
}

func (a *API) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if header == "" {
			if cookie, err := r.Cookie("media_session"); err == nil {
				header = cookie.Value
			}
		}
		token, err := jwt.ParseWithClaims(header, &claims{}, func(t *jwt.Token) (any, error) {
			if t.Method != jwt.SigningMethodHS256 {
				return nil, errors.New("unexpected signing method")
			}
			return a.JWTSecret, nil
		})
		if err != nil || !token.Valid {
			problem(w, 401, "authentication required")
			return
		}
		c := token.Claims.(*claims)
		ctx := context.WithValue(r.Context(), principalKey, principal{ID: c.UserID, Role: c.Role})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *API) admin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if current(r).Role != domain.RoleAdmin {
			problem(w, 403, "admin role required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func current(r *http.Request) principal { return r.Context().Value(principalKey).(principal) }

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func problem(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (int, bool) {
	raw := strings.TrimSpace(r.PathValue(name))
	id, err := strconv.Atoi(raw)
	if err != nil || id < domain.InvalidID {
		problem(w, http.StatusBadRequest, name+" must be an integer id")
		return domain.InvalidID, false
	}
	if id == domain.InvalidID {
		problem(w, http.StatusBadRequest, name+" must be a real id")
		return domain.InvalidID, false
	}
	return id, true
}

func statusFor(err error) int {
	if errors.Is(err, store.ErrNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, store.ErrForbidden) {
		return http.StatusForbidden
	}
	return http.StatusBadRequest
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

// Flush forwards flushes from streaming handlers such as video playback.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// logRequests writes one debug line per API call so the log file shows live
// activity instead of appearing dead during normal use. The line is emitted via
// defer so every call is logged, including handlers that fail or panic.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := &statusRecorder{ResponseWriter: w}
		start := time.Now()
		defer func() {
			status := recorder.status
			if status == 0 {
				status = http.StatusOK
			}
			applog.Printf(applog.Debug, "http %s %s -> %d (%s)", r.Method, r.URL.RequestURI(), status, time.Since(start).Round(time.Millisecond))
		}()
		next.ServeHTTP(recorder, r)
	})
}
