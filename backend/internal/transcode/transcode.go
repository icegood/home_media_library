package transcode

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type Codec string

const (
	H264 Codec = "h264"
	H265 Codec = "h265"
	VP9  Codec = "vp9"
	VP8  Codec = "vp8"
	AV1  Codec = "av1"
)

var ErrUnsupportedCodec = errors.New("unsupported transcode codec")

// Schema is a full transcode profile the user picks as the video fallback.
// It bundles the video codec, audio codec, container, and the browser
// support / compression characteristics shown in the settings UI.
type Schema struct {
	ID          string
	Video       Codec
	Audio       string
	Container   string
	Support     string
	Compression string
}

var schemas = []Schema{
	{ID: "h264-aac-mp4", Video: H264, Audio: "aac", Container: "mp4", Support: "Excellent", Compression: "Good"},
	{ID: "h264-opus-mp4", Video: H264, Audio: "libopus", Container: "mp4", Support: "Good, but less universal", Compression: "Good"},
	{ID: "vp9-opus-webm", Video: VP9, Audio: "libopus", Container: "webm", Support: "Excellent", Compression: "Very good"},
	{ID: "vp9-vorbis-webm", Video: VP9, Audio: "libvorbis", Container: "webm", Support: "Very good", Compression: "Very good"},
	{ID: "av1-opus-webm", Video: AV1, Audio: "libopus", Container: "webm", Support: "Very good on modern devices", Compression: "Excellent"},
	{ID: "hevc-aac-mp4", Video: H265, Audio: "aac", Container: "mp4", Support: "Platform-dependent", Compression: "Excellent"},
	{ID: "hevc-opus-mp4", Video: H265, Audio: "libopus", Container: "mp4", Support: "Poor / inconsistent", Compression: "Excellent"},
	{ID: "vp8-vorbis-webm", Video: VP8, Audio: "libvorbis", Container: "webm", Support: "Excellent", Compression: "Fair"},
	{ID: "vp8-opus-webm", Video: VP8, Audio: "libopus", Container: "webm", Support: "Excellent", Compression: "Fair"},
}

// LegacyIDs are the old single-codec settings values, mapped to the closest
// schema so previously stored user settings keep working.
var legacyIDs = map[string]string{
	"h264": "h264-aac-mp4",
	"h265": "hevc-aac-mp4",
	"vp9":  "vp9-opus-webm",
}

func Schemas() []Schema {
	return append([]Schema(nil), schemas...)
}

func SchemaByID(id string) (Schema, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, schema := range schemas {
		if schema.ID == id {
			return schema, true
		}
	}
	return Schema{}, false
}

func ParseSchema(value string) (Schema, error) {
	if schema, ok := SchemaByID(value); ok {
		return schema, nil
	}
	if legacy, ok := legacyIDs[strings.ToLower(strings.TrimSpace(value))]; ok {
		if schema, found := SchemaByID(legacy); found {
			return schema, nil
		}
	}
	return Schema{}, ErrUnsupportedCodec
}

func ParseCodec(value string) (Codec, error) {
	codec := Codec(strings.ToLower(strings.TrimSpace(value)))
	switch codec {
	case H264, H265, VP9, VP8, AV1:
		return codec, nil
	default:
		return "", ErrUnsupportedCodec
	}
}

type Service struct {
	Target Schema
}

func (s Service) Probe(ctx context.Context, path string) (Codec, error) {
	output, err := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_name",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	).Output()
	if err != nil {
		return "", fmt.Errorf("probe video: %w", err)
	}
	switch strings.TrimSpace(string(output)) {
	case "h264":
		return H264, nil
	case "hevc", "h265":
		return H265, nil
	case "vp9":
		return VP9, nil
	case "vp8":
		return VP8, nil
	case "av1":
		return AV1, nil
	default:
		return "", nil
	}
}

func (s Service) ProbeAudio(ctx context.Context, path string) (string, error) {
	output, err := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-select_streams", "a:0",
		"-show_entries", "stream=codec_name",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	).Output()
	if err != nil {
		return "", fmt.Errorf("probe audio: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func DirectPlayAudioSupported(video Codec, audio string) bool {
	if audio == "" {
		return true
	}
	switch video {
	case H264, H265:
		return audio == "aac"
	case VP9, VP8:
		return audio == "opus" || audio == "vorbis"
	case AV1:
		return audio == "opus"
	default:
		return false
	}
}

func DirectPlayContainerSupported(video Codec, mimeType string) bool {
	mimeType = strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
	switch video {
	case H264, H265:
		return mimeType == "video/mp4" || mimeType == "video/x-m4v"
	case VP9, VP8, AV1:
		return mimeType == "video/webm"
	default:
		return false
	}
}

func (s Service) ContentType() string {
	switch s.Target.ID {
	case "h264-aac-mp4":
		return `video/mp4; codecs="avc1.42E01E,mp4a.40.2"`
	case "h264-opus-mp4":
		return `video/mp4; codecs="avc1.42E01E,opus"`
	case "vp9-opus-webm":
		return `video/webm; codecs="vp9,opus"`
	case "vp9-vorbis-webm":
		return `video/webm; codecs="vp9,vorbis"`
	case "av1-opus-webm":
		return `video/webm; codecs="av01.0.04M.08,opus"`
	case "hevc-aac-mp4":
		return `video/mp4; codecs="hvc1,mp4a.40.2"`
	case "hevc-opus-mp4":
		return `video/mp4; codecs="hvc1,opus"`
	case "vp8-vorbis-webm":
		return `video/webm; codecs="vp8,vorbis"`
	case "vp8-opus-webm":
		return `video/webm; codecs="vp8,opus"`
	default:
		return `video/mp4; codecs="avc1.42E01E,mp4a.40.2"`
	}
}

func (s Service) Stream(ctx context.Context, input string, output io.Writer, startSeconds float64) error {
	videoCodec, audioCodec, formatArgs, err := arguments(s.Target)
	if err != nil {
		return err
	}
	args := []string{
		"-hide_banner", "-loglevel", "error",
	}
	if startSeconds > 0 {
		args = append(args, "-ss", fmt.Sprintf("%g", startSeconds))
	}
	args = append(args,
		"-i", input,
		"-map", "0:v:0", "-map", "0:a:0?",
		"-c:v", videoCodec, "-preset", "veryfast",
		"-c:a", audioCodec, "-b:a", "160k",
	)
	args = append(args, formatArgs...)
	args = append(args, "pipe:1")
	command := exec.CommandContext(ctx, "ffmpeg", args...)
	command.Stdout = output
	var stderr strings.Builder
	command.Stderr = &stderr
	if err := command.Run(); err != nil && ctx.Err() == nil {
		return fmt.Errorf("transcode video: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return ctx.Err()
}

func arguments(schema Schema) (video, audio string, format []string, err error) {
	audio = schema.Audio
	if audio == "" {
		audio = "aac"
	}
	switch schema.ID {
	case "h264-aac-mp4":
		return "libx264", audio, []string{
			"-movflags", "frag_keyframe+empty_moov+default_base_moof",
			"-pix_fmt", "yuv420p", "-f", "mp4",
		}, nil
	case "h264-opus-mp4":
		return "libx264", audio, []string{
			"-strict", "-2",
			"-movflags", "frag_keyframe+empty_moov+default_base_moof",
			"-pix_fmt", "yuv420p", "-f", "mp4",
		}, nil
	case "vp9-opus-webm", "vp9-vorbis-webm":
		return "libvpx-vp9", audio, []string{
			"-deadline", "realtime", "-cpu-used", "5",
			"-f", "webm",
		}, nil
	case "av1-opus-webm":
		return "libaom-av1", audio, []string{
			"-deadline", "realtime", "-cpu-used", "6", "-row-mt", "1",
			"-f", "webm",
		}, nil
	case "hevc-aac-mp4":
		return "libx265", audio, []string{
			"-tag:v", "hvc1",
			"-movflags", "frag_keyframe+empty_moov+default_base_moof",
			"-pix_fmt", "yuv420p", "-f", "mp4",
		}, nil
	case "hevc-opus-mp4":
		return "libx265", audio, []string{
			"-strict", "-2",
			"-tag:v", "hvc1",
			"-movflags", "frag_keyframe+empty_moov+default_base_moof",
			"-pix_fmt", "yuv420p", "-f", "mp4",
		}, nil
	case "vp8-vorbis-webm", "vp8-opus-webm":
		return "libvpx", audio, []string{
			"-deadline", "realtime", "-cpu-used", "5",
			"-f", "webm",
		}, nil
	default:
		return "", "", nil, ErrUnsupportedCodec
	}
}
