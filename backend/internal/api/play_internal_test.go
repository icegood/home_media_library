package api

import (
	"net/url"
	"testing"

	"media-library/backend/internal/transcode"
)

func TestServesOriginal(t *testing.T) {
	cases := []struct {
		name  string
		query string
		codec transcode.Codec
		audio string
		mime  string
		want  bool
	}{
		{"advertised codec, matching audio and container", "codecs=h264", transcode.H264, "aac", "video/mp4", true},
		{"codec not advertised", "codecs=h264", transcode.H265, "aac", "video/mp4", false},
		{"advertised codec in a foreign container", "codecs=h264", transcode.H264, "aac", "video/x-msvideo", false},
		{"advertised codec with foreign audio", "codecs=h264", transcode.H264, "mp3", "video/mp4", false},
		// The browser tries the original first and falls back by itself.
		{"direct=1 without the codec", "codecs=h264&direct=1", transcode.H265, "aac", "video/mp4", true},
		{"direct=1 for an unknown codec and container", "direct=1", "", "mp3", "video/x-matroska", true},
		{"direct other than 1 is ignored", "codecs=h264&direct=true", transcode.H265, "aac", "video/mp4", false},
	}
	for _, tc := range cases {
		query, err := url.ParseQuery(tc.query)
		if err != nil {
			t.Fatal(err)
		}
		if got := servesOriginal(query, tc.codec, tc.audio, tc.mime); got != tc.want {
			t.Errorf("%s: servesOriginal = %v, want %v", tc.name, got, tc.want)
		}
	}
}
