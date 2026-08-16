package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"media-library/backend/internal/domain"
)

type Extractor struct {
	ExifTool string
	FFProbe  string
	Timeout  time.Duration
}

type Result struct {
	Metadata map[string]any
	GPS      string
	TakenAt  string
	Error    string
}

func New() Extractor {
	return Extractor{ExifTool: "exiftool", FFProbe: "ffprobe", Timeout: 20 * time.Second}
}

func (e Extractor) Extract(ctx context.Context, path, mimeType string) (Result, error) {
	if e.ExifTool == "" {
		e.ExifTool = "exiftool"
	}
	if e.FFProbe == "" {
		e.FFProbe = "ffprobe"
	}
	if e.Timeout <= 0 {
		e.Timeout = 20 * time.Second
	}
	result := Result{Metadata: map[string]any{}}
	exif, gps, takenAt, err := e.extractEXIF(ctx, path)
	if err != nil && !isToolUnavailable(err) {
		result.Metadata["exifError"] = err.Error()
		result.Error = appendActionError(result.Error, err)
	}
	if len(exif) != 0 {
		result.Metadata["exif"] = exif
	}
	result.GPS = gps
	result.TakenAt = takenAt
	if strings.HasPrefix(mimeType, "video/") {
		ffprobe, err := e.extractFFProbe(ctx, path)
		if err != nil && !isToolUnavailable(err) {
			result.Metadata["ffprobeError"] = err.Error()
			result.Error = appendActionError(result.Error, err)
		}
		if len(ffprobe) != 0 {
			result.Metadata["ffprobe"] = ffprobe
		}
	}
	return result, nil
}

func appendActionError(existing string, err error) string {
	if err == nil {
		return existing
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return existing
	}
	if existing == "" {
		return message
	}
	return existing + "; " + message
}

func (e Extractor) extractEXIF(ctx context.Context, path string) (map[string]any, string, string, error) {
	output, err := e.command(ctx, e.ExifTool, "-json", "-n", path)
	if err != nil {
		return nil, "", "", fmt.Errorf("exiftool: %w", err)
	}
	var documents []map[string]any
	if err := json.Unmarshal(output, &documents); err != nil {
		return nil, "", "", fmt.Errorf("decode exiftool output: %w", err)
	}
	if len(documents) == 0 {
		return nil, "", "", nil
	}
	document := documents[0]
	gps := gpsFromEXIF(document)
	takenAt := takenAtFromEXIF(document)
	return document, gps, takenAt, nil
}

func (e Extractor) extractFFProbe(ctx context.Context, path string) (map[string]any, error) {
	output, err := e.command(ctx, e.FFProbe,
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	)
	if err != nil {
		return nil, fmt.Errorf("ffprobe: %w", err)
	}
	var document map[string]any
	if err := json.Unmarshal(output, &document); err != nil {
		return nil, fmt.Errorf("decode ffprobe output: %w", err)
	}
	return document, nil
}

func (e Extractor) command(ctx context.Context, name string, args ...string) ([]byte, error) {
	commandCtx, cancel := context.WithTimeout(ctx, e.Timeout)
	defer cancel()
	command := exec.CommandContext(commandCtx, name, args...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if commandCtx.Err() != nil {
		return nil, commandCtx.Err()
	}
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return nil, fmt.Errorf("%w: %s", err, message)
		}
		return nil, err
	}
	return output, nil
}

func gpsFromEXIF(document map[string]any) string {
	latitude, ok1 := number(document["GPSLatitude"])
	longitude, ok2 := number(document["GPSLongitude"])
	if !ok1 || !ok2 {
		latitude, ok1 = number(document["Composite:GPSLatitude"])
		longitude, ok2 = number(document["Composite:GPSLongitude"])
	}
	if !ok1 || !ok2 {
		return ""
	}
	gps, ok := domain.CanonicalGPS(fmt.Sprintf("%f,%f", latitude, longitude))
	if !ok {
		return ""
	}
	return gps
}

func takenAtFromEXIF(document map[string]any) string {
	var fileModify time.Time
	fileModifyValid := false
	if value, ok := stringValue(document["FileModifyDate"]); ok {
		if parsed, ok := parseExifDate(value); ok {
			fileModify = parsed
			fileModifyValid = true
		}
	}
	for _, key := range []string{
		"DateTimeOriginal",
		"CreateDate",
		"MediaCreateDate",
		"TrackCreateDate",
		"ModifyDate",
	} {
		if value, ok := stringValue(document[key]); ok {
			parsed, ok := parseExifDate(value)
			if !ok {
				continue
			}
			// Camera dates carry no timezone: the wall-clock time in the EXIF
			// matches the file's mtime (both follow the camera clock). Use the
			// mtime, which is an absolute instant, to convert to UTC; otherwise
			// the naive string would be treated as UTC and drift by the camera
			// offset in the UI.
			if !hasExplicitOffset(value) && fileModifyValid {
				correction := fileModify.Sub(parsed)
				if correction > -12*time.Hour && correction < 12*time.Hour {
					parsed = parsed.Add(correction)
				}
			}
			return parsed.UTC().Format(time.RFC3339)
		}
	}
	return ""
}

var explicitOffsetRe = regexp.MustCompile(`(?i)(?:z|[+-]\d{2}:?\d{2})\s*$`)

func hasExplicitOffset(value string) bool {
	return explicitOffsetRe.MatchString(strings.TrimSpace(value))
}

func stringValue(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		return trimmed, trimmed != ""
	default:
		return "", false
	}
}

func parseExifDate(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "0000:00:00") {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339Nano,
		"2006:01:02 15:04:05-07:00",
		"2006:01:02 15:04:05-07:00",
		"2006:01:02 15:04:05Z07:00",
		"2006:01:02 15:04:05",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func number(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case json.Number:
		value, err := typed.Float64()
		return value, err == nil
	default:
		return 0, false
	}
}

func isToolUnavailable(err error) bool {
	var pathErr *exec.Error
	return errors.As(err, &pathErr) || errors.Is(err, os.ErrNotExist)
}
