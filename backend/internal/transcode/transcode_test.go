package transcode

import "testing"

func TestParseCodecUsesFixedAllowList(t *testing.T) {
	for _, value := range []string{"h264", "h265", "vp9", "H264"} {
		if _, err := ParseCodec(value); err != nil {
			t.Fatalf("ParseCodec(%q): %v", value, err)
		}
	}
	if _, err := ParseCodec("av1"); err == nil {
		t.Fatal("AV1 must not be accepted")
	}
}

func TestAudioAndContainerPolicy(t *testing.T) {
	tests := []struct {
		codec Codec
		mime  string
	}{
		{H264, `video/mp4; codecs="avc1.42E01E,mp4a.40.2"`},
		{H265, `video/mp4; codecs="hvc1,mp4a.40.2"`},
		{VP9, `video/webm; codecs="vp9,opus"`},
	}
	for _, test := range tests {
		if got := (Service{Target: test.codec}).ContentType(); got != test.mime {
			t.Fatalf("%s content type = %q", test.codec, got)
		}
	}
}

func TestDirectPlayRequiresCompatibleAudio(t *testing.T) {
	if !DirectPlayAudioSupported(H264, "aac") || !DirectPlayAudioSupported(VP9, "opus") {
		t.Fatal("expected standard browser audio pairings to direct play")
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
	if DirectPlayContainerSupported(VP9, "video/x-matroska") || DirectPlayContainerSupported(H264, "video/quicktime") {
		t.Fatal("mkv and quicktime containers should trigger transcoding")
	}
}
