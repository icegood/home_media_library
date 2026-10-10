package applog

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Level int32

const (
	Debug Level = iota
	Info
	Warn
	Error
)

var minLevel atomic.Int32
var fileMu sync.Mutex
var logFile *os.File

// Rotation bounds the log file: once it would exceed MaxSizeMB it is renamed
// to <path>.1 (older backups shift up to MaxBackups), and backups older than
// MaxAgeDays are deleted. Zero values disable the respective limit.
type Rotation struct {
	MaxSizeMB  int
	MaxBackups int
	MaxAgeDays int
}

// The fields below are guarded by fileMu.
var (
	logPath  string
	logSize  int64
	rotation Rotation
)

// SetRotation applies new rotation limits to the configured log file.
func SetRotation(r Rotation) {
	fileMu.Lock()
	defer fileMu.Unlock()
	rotation = r
	pruneBackupsLocked(time.Now())
}

// fileWriter appends to the current log file and rotates it by size.
type fileWriter struct{}

func (fileWriter) Write(p []byte) (int, error) {
	fileMu.Lock()
	defer fileMu.Unlock()
	if logFile == nil {
		return len(p), nil
	}
	if limit := int64(rotation.MaxSizeMB) << 20; limit > 0 && logSize > 0 && logSize+int64(len(p)) > limit {
		if err := rotateLocked(); err != nil {
			fmt.Fprintf(os.Stderr, "applog: rotate %s: %v\n", logPath, err)
		}
	}
	if logFile == nil {
		return len(p), nil
	}
	n, err := logFile.Write(p)
	logSize += int64(n)
	return n, err
}

func backupPath(index int) string {
	return logPath + "." + strconv.Itoa(index)
}

func rotateLocked() error {
	backups := rotation.MaxBackups
	if backups < 1 {
		backups = 1
	}
	_ = logFile.Close()
	_ = os.Remove(backupPath(backups))
	for index := backups - 1; index >= 1; index-- {
		_ = os.Rename(backupPath(index), backupPath(index+1))
	}
	renameErr := os.Rename(logPath, backupPath(1))
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		// Keep writing somewhere rather than losing every later line.
		file, _ = os.OpenFile(backupPath(1), os.O_APPEND|os.O_WRONLY, 0o600)
		logFile = file
		return err
	}
	logFile = file
	logSize = 0
	pruneBackupsLocked(time.Now())
	return renameErr
}

func pruneBackupsLocked(now time.Time) {
	if logPath == "" {
		return
	}
	backups := rotation.MaxBackups
	if backups < 1 {
		backups = 1
	}
	for index := 1; index <= backups+1; index++ {
		path := backupPath(index)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		expired := rotation.MaxAgeDays > 0 && now.Sub(info.ModTime()) > time.Duration(rotation.MaxAgeDays)*24*time.Hour
		if index > backups || expired {
			_ = os.Remove(path)
		}
	}
}

func init() {
	minLevel.Store(int32(Info))
}

func ParseLevel(value string) (Level, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "D", "DEBUG":
		return Debug, nil
	case "I", "INFO", "":
		return Info, nil
	case "W", "WARN", "WARNING":
		return Warn, nil
	case "E", "ERROR":
		return Error, nil
	default:
		return Info, fmt.Errorf("log level must be one of: D, I, W, E")
	}
}

func SetLevel(level Level) {
	minLevel.Store(int32(level))
}

func ConfigureFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	fileMu.Lock()
	defer fileMu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if logFile != nil {
		_ = logFile.Close()
	}
	logFile = file
	logPath = path
	logSize = 0
	if info, err := file.Stat(); err == nil {
		logSize = info.Size()
	}
	log.SetOutput(io.MultiWriter(os.Stderr, fileWriter{}))
	return nil
}

// ErrNotConfigured is returned by ClearFile when no file logger is configured
// in this process, so there is no handle to clear.
var ErrNotConfigured = errors.New("applog: no log file configured")

// ClearFile truncates the application log file to zero length through the
// writer's own open handle and resets its offset, so the next log line
// appends at the start of a clean file. The handle stays open for appending.
func ClearFile() error {
	fileMu.Lock()
	defer fileMu.Unlock()
	if logFile == nil {
		return ErrNotConfigured
	}
	if err := logFile.Truncate(0); err != nil {
		return err
	}
	logSize = 0
	_, err := logFile.Seek(0, io.SeekStart)
	return err
}

func LevelString(level Level) string {
	switch level {
	case Debug:
		return "D"
	case Info:
		return "I"
	case Warn:
		return "W"
	case Error:
		return "E"
	default:
		return "I"
	}
}

func Printf(level Level, format string, args ...any) {
	if int32(level) < minLevel.Load() {
		return
	}
	log.Printf("%s "+format, append([]any{LevelString(level)}, args...)...)
}
