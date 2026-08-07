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
)

var ErrUnsupportedCodec = errors.New("unsupported transcode codec")

type Service struct {
	Target Codec
}

func ParseCodec(value string) (Codec, error) {
	codec := Codec(strings.ToLower(strings.TrimSpace(value)))
	switch codec {
	case H264, H265, VP9:
		return codec, nil
	default:
		return "", ErrUnsupportedCodec
	}
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
	case VP9:
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
	case VP9:
		return mimeType == "video/webm"
	default:
		return false
	}
}

func (s Service) ContentType() string {
	if s.Target == VP9 {
		return `video/webm; codecs="vp9,opus"`
	}
	if s.Target == H265 {
		return `video/mp4; codecs="hvc1,mp4a.40.2"`
	}
	return `video/mp4; codecs="avc1.42E01E,mp4a.40.2"`
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

func arguments(codec Codec) (video, audio string, format []string, err error) {
	switch codec {
	case H264:
		return "libx264", "aac", []string{
			"-movflags", "frag_keyframe+empty_moov+default_base_moof",
			"-pix_fmt", "yuv420p", "-f", "mp4",
		}, nil
	case H265:
		return "libx265", "aac", []string{
			"-tag:v", "hvc1",
			"-movflags", "frag_keyframe+empty_moov+default_base_moof",
			"-pix_fmt", "yuv420p", "-f", "mp4",
		}, nil
	case VP9:
		return "libvpx-vp9", "libopus", []string{
			"-deadline", "realtime", "-cpu-used", "5",
			"-f", "webm",
		}, nil
	default:
		return "", "", nil, ErrUnsupportedCodec
	}
}
