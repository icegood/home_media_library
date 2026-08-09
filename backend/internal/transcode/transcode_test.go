package transcode

import "testing"

func TestParseSchemaAllowList(t *testing.T) {
	for _, schema := range schemas {
		got, err := ParseSchema(schema.ID)
		if err != nil {
			t.Fatalf("ParseSchema(%q): %v", schema.ID, err)
		}
		if got.ID != schema.ID {
			t.Fatalf("ParseSchema(%q) = %q", schema.ID, got.ID)
		}
	}
	if _, err := ParseSchema("vp2"); err == nil {
		t.Fatal("unknown schema must not be accepted")
	}
}

func TestParseSchemaAcceptsLegacyCodecValues(t *testing.T) {
	legacy := map[string]string{
		"h264": "h264-aac-mp4",
		"h265": "hevc-aac-mp4",
		"vp9":  "vp9-opus-webm",
	}
	for value, want := range legacy {
		schema, err := ParseSchema(value)
		if err != nil {
			t.Fatalf("ParseSchema(%q): %v", value, err)
		}
		if schema.ID != want {
			t.Fatalf("ParseSchema(%q) = %q, want %q", value, schema.ID, want)
		}
	}
}

func TestParseCodecAllowList(t *testing.T) {
	for _, value := range []string{"h264", "h265", "vp9", "vp8", "av1", "H264"} {
		if _, err := ParseCodec(value); err != nil {
			t.Fatalf("ParseCodec(%q): %v", value, err)
		}
	}
	if _, err := ParseCodec("vp2"); err == nil {
		t.Fatal("unknown codec must not be accepted")
	}
}

func TestSchemaArgumentsCoverAllProfiles(t *testing.T) {
	for _, schema := range schemas {
		video, audio, format, err := arguments(schema)
		if err != nil {
			t.Fatalf("arguments(%q): %v", schema.ID, err)
		}
		if video == "" || audio == "" || len(format) == 0 {
			t.Fatalf("arguments(%q) produced empty profile", schema.ID)
		}
	}
}

func TestContentTypePerSchema(t *testing.T) {
	tests := map[string]string{
		"h264-aac-mp4":   `video/mp4; codecs="avc1.42E01E,mp4a.40.2"`,
		"h264-opus-mp4":  `video/mp4; codecs="avc1.42E01E,opus"`,
		"vp9-opus-webm":  `video/webm; codecs="vp9,opus"`,
		"vp9-vorbis-webm": `video/webm; codecs="vp9,vorbis"`,
		"av1-opus-webm":  `video/webm; codecs="av01.0.04M.08,opus"`,
		"hevc-aac-mp4":   `video/mp4; codecs="hvc1,mp4a.40.2"`,
		"hevc-opus-mp4":  `video/mp4; codecs="hvc1,opus"`,
		"vp8-vorbis-webm": `video/webm; codecs="vp8,vorbis"`,
		"vp8-opus-webm":  `video/webm; codecs="vp8,opus"`,
	}
	for id, want := range tests {
		schema, _ := ParseSchema(id)
		if got := (Service{Target: schema}).ContentType(); got != want {
			t.Fatalf("%s content type = %q, want %q", id, got, want)
		}
	}
}

func TestDirectPlayRequiresCompatibleAudio(t *testing.T) {
	if !DirectPlayAudioSupported(H264, "aac") || !DirectPlayAudioSupported(VP9, "opus") || !DirectPlayAudioSupported(VP9, "vorbis") {
		t.Fatal("expected standard browser audio pairings to direct play")
	}
	if !DirectPlayAudioSupported(VP8, "vorbis") || !DirectPlayAudioSupported(AV1, "opus") {
		t.Fatal("expected vp8/vorbis and av1/opus pairings to direct play")
	}
	if DirectPlayAudioSupported(H264, "dts") || DirectPlayAudioSupported(VP9, "aac") {
		t.Fatal("unsupported audio must trigger transcoding")
	}
	if !DirectPlayAudioSupported(H265, "") {
		t.Fatal("video without audio should direct play")
	}
}

func TestDirectPlayRequiresCompatibleContainer(t *testing.T) {
	if !DirectPlayContainerSupported(H264, "video/mp4") || !DirectPlayContainerSupported(VP9, "video/webm") {
		t.Fatal("expected browser-safe containers to direct play")
	}
	if !DirectPlayContainerSupported(VP8, "video/webm") || !DirectPlayContainerSupported(AV1, "video/webm") {
		t.Fatal("expected vp8 and av1 webm containers to direct play")
	}
	if DirectPlayContainerSupported(VP9, "video/x-matroska") || DirectPlayContainerSupported(H264, "video/quicktime") {
		t.Fatal("mkv and quicktime containers should trigger transcoding")
	}
}
