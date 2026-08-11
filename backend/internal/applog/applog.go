package applog

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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
	log.SetOutput(io.MultiWriter(os.Stderr, file))
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
