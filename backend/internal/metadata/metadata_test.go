package metadata_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"media-library/backend/internal/metadata"
)

func TestExtractorUsesExifGPSAndFFProbeForVideo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test writes shell scripts")
	}
	bin := t.TempDir()
	writeTool(t, filepath.Join(bin, "exiftool"), `#!/bin/sh
printf '%s\n' '[{"GPSLatitude":50.45,"GPSLongitude":30.52,"Make":"TestCam","Model":"PocketRocket","DateTimeOriginal":"2020:08:21 12:34:56","CreateDate":"2020:08:22 01:02:03","FNumber":3.5,"ExposureTime":0.03333333333,"ISO":1600,"FocalLength":18,"LensSpec":"18 55 3.5 5.6 6","FocusMode":"AF-A","HistoryParams":"darktable-noise","BlueTRC":"(Binary data)","MakerNoteVersion":"0210","SourceFile":"sample.mp4","Directory":"/media","FileName":"sample.mp4","FileAccessDate":"2026:07:29 06:12:16+00:00","FileInodeChangeDate":"2026:07:29 06:12:16+00:00","FileModifyDate":"2026:07:29 06:12:16+00:00","FileSize":"123 kB","FileType":"MP4","FileTypeExtension":"mp4","MIMEType":"video/mp4","ExifToolVersion":13.3,"ThumbnailImage":"(Binary data)"}]'
`)
	writeTool(t, filepath.Join(bin, "ffprobe"), `#!/bin/sh
printf '%s\n' '{"streams":[{"codec_type":"video","codec_name":"h264","width":1920,"height":1080},{"codec_type":"audio","codec_name":"aac"}],"format":{"duration":"10.5"}}'
`)
	result, err := (metadata.Extractor{
		ExifTool: filepath.Join(bin, "exiftool"),
		FFProbe:  filepath.Join(bin, "ffprobe"),
		Timeout:  time.Second,
	}).Extract(context.Background(), "sample.mp4", "video/mp4")
	if err != nil {
		t.Fatal(err)
	}
	if result.GPS != "50.45,30.52" {
		t.Fatalf("gps = %q", result.GPS)
	}
	if result.TakenAt != "2020-08-21T12:34:56Z" {
		t.Fatalf("takenAt = %q", result.TakenAt)
	}
	if result.Metadata["exif"] == nil || result.Metadata["ffprobe"] == nil {
		t.Fatalf("metadata not populated: %#v", result.Metadata)
	}
	exif, ok := result.Metadata["exif"].(map[string]any)
	if !ok {
		t.Fatalf("exif metadata has unexpected type: %#v", result.Metadata["exif"])
	}
	for _, key := range []string{"Make", "Model", "DateTimeOriginal", "CreateDate", "GPSLatitude", "GPSLongitude", "FNumber", "ExposureTime", "ISO", "FocalLength", "LensSpec", "FocusMode", "HistoryParams", "BlueTRC", "MakerNoteVersion"} {
		if _, ok := exif[key]; !ok {
			t.Fatalf("camera exif key %q was filtered out: %#v", key, exif)
		}
	}
	for _, key := range []string{"SourceFile", "Directory", "FileName", "FileAccessDate", "FileInodeChangeDate", "FileModifyDate", "FileSize", "FileType", "FileTypeExtension", "MIMEType", "ExifToolVersion", "ThumbnailImage"} {
		if _, ok := exif[key]; !ok {
			t.Fatalf("raw exif key %q should be stored: %#v", key, exif)
		}
	}
}

func TestExtractorGracefullySkipsMissingTools(t *testing.T) {
	result, err := (metadata.Extractor{
		ExifTool: filepath.Join(t.TempDir(), "missing-exiftool"),
		FFProbe:  filepath.Join(t.TempDir(), "missing-ffprobe"),
		Timeout:  time.Second,
	}).Extract(context.Background(), "sample.mp4", "video/mp4")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Metadata) != 0 || result.GPS != "" {
		t.Fatalf("missing tools should not block scan: %#v", result)
	}
}

func writeTool(t *testing.T, path, script string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}
