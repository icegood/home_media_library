package domain

import (
	"strconv"
	"strings"
	"time"

	"media-library/backend/internal/mail"
)

type Role string

const (
	RoleAdmin   Role = "admin"
	RoleRegular Role = "regular"
)

const InvalidID = -1

const MaxVideoThumbnailCount = 100

type User struct {
	ID           int    `json:"id"`
	Login        string `json:"login"`
	Role         Role   `json:"role"`
	Email        string `json:"email"`
	PasswordHash string `json:"-"`
}

type LibraryUserAccess struct {
	User    User `json:"user"`
	Allowed bool `json:"allowed"`
}

type ServerSettings struct {
	HTTPEnabled                      bool   `json:"httpEnabled"`
	HTTPSEnabled                     bool   `json:"httpsEnabled"`
	PublicDNS                        string `json:"publicDns"`
	ACMEEmail                        string `json:"acmeEmail"`
	ThumbnailWidth                   int    `json:"thumbnailWidth"`
	ThumbnailHeight                  int    `json:"thumbnailHeight"`
	WorkerPoolSize                   int    `json:"workerPoolSize"`
	VideoThumbnailFirstSeconds       int    `json:"videoThumbnailFirstSeconds"`
	VideoThumbnailMaxCount           int    `json:"videoThumbnailMaxCount"`
	VideoThumbnailMinIntervalSeconds int    `json:"videoThumbnailMinIntervalSeconds"`
	SessionMaxAgeHours               int    `json:"sessionMaxAgeHours"`
	FinishedJobRetentionMinutes      int    `json:"finishedJobRetentionMinutes"`
	LogLevel                         string `json:"logLevel"`
	LogRotateMaxSizeMB               int    `json:"logRotateMaxSizeMB"`
	LogRotateMaxBackups              int    `json:"logRotateMaxBackups"`
	LogRotateMaxAgeDays              int    `json:"logRotateMaxAgeDays"`
	SMTPHost                         string `json:"smtpHost"`
	SMTPPort                         int    `json:"smtpPort"`
	SMTPUsername                     string `json:"smtpUsername"`
	SMTPPassword                     string `json:"smtpPassword"`
	SMTPFrom                         string `json:"smtpFrom"`
}

// SMTP returns the configured outbound mail settings.
func (s ServerSettings) SMTP() mail.SMTPConfig {
	return mail.SMTPConfig{
		Host:     strings.TrimSpace(s.SMTPHost),
		Port:     s.SMTPPort,
		Username: s.SMTPUsername,
		Password: s.SMTPPassword,
		From:     strings.TrimSpace(s.SMTPFrom),
	}
}

func DefaultServerSettings() ServerSettings {
	return ServerSettings{
		HTTPEnabled:                      true,
		HTTPSEnabled:                     false,
		PublicDNS:                        "",
		ACMEEmail:                        "",
		ThumbnailWidth:                   480,
		ThumbnailHeight:                  360,
		WorkerPoolSize:                   4,
		VideoThumbnailFirstSeconds:       5,
		VideoThumbnailMaxCount:           MaxVideoThumbnailCount,
		VideoThumbnailMinIntervalSeconds: 120,
		SessionMaxAgeHours:               720,
		FinishedJobRetentionMinutes:      10,
		LogLevel:                         "I",
		LogRotateMaxSizeMB:               10,
		LogRotateMaxBackups:              5,
		LogRotateMaxAgeDays:              30,
		SMTPPort:                         587,
	}
}

type UserSettings struct {
	Theme string `json:"theme"`
	Codec string `json:"codec"`
	Zoom  int    `json:"zoom"`
	// DefaultThumbImage/Video/Folder pick the placeholder picture shown for
	// media whose thumbnail has not been generated yet. Values are ids in the
	// web UI's built-in picture catalog.
	DefaultThumbImage  string `json:"defaultThumbImage"`
	DefaultThumbVideo  string `json:"defaultThumbVideo"`
	DefaultThumbFolder string `json:"defaultThumbFolder"`
}

func DefaultUserSettings() UserSettings {
	return UserSettings{
		Theme: "light", Codec: "h264-aac-mp4", Zoom: 100,
		DefaultThumbImage: "mountains", DefaultThumbVideo: "mountains", DefaultThumbFolder: "mountains",
	}
}

type ScheduledTask struct {
	ID        int        `json:"id"`
	Name      string     `json:"name"`
	TaskType  string     `json:"taskType"`
	LibraryID int        `json:"libraryId"`
	Cron      string     `json:"cron"`
	Enabled   bool       `json:"enabled"`
	LastRunAt *time.Time `json:"lastRunAt,omitempty"`
	NextRunAt time.Time  `json:"nextRunAt"`
}

type BackgroundJob struct {
	ID          string         `json:"id"`
	Category    string         `json:"category"`
	Type        string         `json:"type"`
	LibraryID   int            `json:"libraryId"`
	LibraryName string         `json:"libraryName"`
	RootPath    string         `json:"rootPath"`
	Status      string         `json:"status"`
	Paused      bool           `json:"paused"`
	Cancelable  bool           `json:"cancelable"`
	CurrentPath string         `json:"currentPath"`
	Processed   int            `json:"processed"`
	Total       int            `json:"total"`
	Error       string         `json:"error"`
	StartedAt   time.Time      `json:"startedAt"`
	FinishedAt  *time.Time     `json:"finishedAt,omitempty"`
	Options     map[string]any `json:"options,omitempty"`
}

type ImportedUser struct {
	User              User   `json:"user"`
	TemporaryPassword string `json:"temporaryPassword,omitempty"`
	Existed           bool   `json:"existed"`
}

type ImportAccess struct {
	LibraryID int `json:"libraryId"`
	UserID    int `json:"userId"`
}

type ImportSnapshot struct {
	Users     []User         `json:"users"`
	Libraries []Library      `json:"libraries"`
	Access    []ImportAccess `json:"access"`
}

type ImportResult struct {
	Users     []ImportedUser `json:"users"`
	Libraries []Library      `json:"libraries"`
	Access    []ImportAccess `json:"access"`
}

type Library struct {
	ID    int           `json:"id"`
	Name  string        `json:"name"`
	Roots []LibraryRoot `json:"roots"`
	Stats LibraryStats  `json:"stats,omitempty"`
}

type FavoriteView struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type LibraryStats struct {
	Folders int `json:"folders"`
	Files   int `json:"files"`
	Images  int `json:"images"`
	Videos  int `json:"videos"`
}

type LibraryRoot struct {
	// ID is the media_folders.id value selected as this library root.
	ID   int    `json:"id"`
	Path string `json:"path,omitempty"`
}

type Kind string

const (
	KindImage Kind = "image"
	KindVideo Kind = "video"
)

func KindFromMIME(mimeType string) Kind {
	if len(mimeType) >= len("video/") && mimeType[:len("video/")] == "video/" {
		return KindVideo
	}
	return KindImage
}

type Media struct {
	ID             int            `json:"id"`
	FolderID       int            `json:"folderId"`
	Path           string         `json:"-"`
	RelativePath   string         `json:"relativePath"`
	Name           string         `json:"name"`
	Kind           Kind           `json:"kind"`
	MIMEType       string         `json:"mimeType"`
	Size           int64          `json:"size"`
	Metadata       map[string]any `json:"metadata"`
	GPS            string         `json:"gps"`
	TakenAt        string         `json:"takenAt"`
	MetadataError  string         `json:"metadataError,omitempty"`
	ThumbnailError string         `json:"thumbnailError,omitempty"`
	Favorite       bool           `json:"favorite,omitempty"`
}

type MapMedia struct {
	Media
	LibraryID int `json:"libraryId"`
}

type MediaFolder struct {
	ID           int    `json:"id"`
	ParentID     int    `json:"parentId"`
	Path         string `json:"-"`
	RelativePath string `json:"relativePath"`
	Name         string `json:"name"`
}

type Thumbnail struct {
	MediaID  int    `json:"mediaId"`
	Index    int    `json:"index"`
	Path     string `json:"-"`
	MIMEType string `json:"mimeType"`
}

type ThumbnailRef struct {
	MediaID int `json:"mediaId"`
	Index   int `json:"index"`
}

type ThumbnailCleanupRefs struct {
	Media   []ThumbnailRef
	Folders []int
}

type FolderThumbnail struct {
	FolderID int            `json:"folderId"`
	Path     string         `json:"-"`
	MIMEType string         `json:"mimeType"`
	Sources  []ThumbnailRef `json:"sources"`
}

type Entry struct {
	ID               int            `json:"id"`
	Name             string         `json:"name"`
	RelativePath     string         `json:"relativePath"`
	Type             string         `json:"type"`
	Media            *Media         `json:"media,omitempty"`
	Folder           *MediaFolder   `json:"folder,omitempty"`
	FolderThumbnails []ThumbnailRef `json:"folderThumbnails,omitempty"`
	FolderThumbnail  int            `json:"folderThumbnail,omitempty"`
}

type GPSPatch struct {
	GPS *string `json:"gps"`
}

// Bounds is a geographic rectangle in WGS84 degrees: west/east longitudes and
// south/north latitudes, in the same order as the GeoJSON bbox array.
type Bounds struct {
	West, South, East, North float64
}

type MediaDetailsPatch struct {
	Name    *string `json:"name"`
	GPS     *string `json:"gps"`
	TakenAt *string `json:"takenAt"`
}

func CanonicalGPS(value string) (string, bool) {
	parts := strings.Split(value, ",")
	if len(parts) != 2 {
		return "", false
	}
	latitude, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	longitude, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err1 != nil || err2 != nil || latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180 {
		return "", false
	}
	return strconv.FormatFloat(latitude, 'f', -1, 64) + "," + strconv.FormatFloat(longitude, 'f', -1, 64), true
}
