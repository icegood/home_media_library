# Playback / transcoding (`internal/transcode`)

- `/play` probes the source. If the browser-advertised codecs match and the
  container + audio are direct-play safe, serve original bytes (HTTP range
  supported). Otherwise FFmpeg transcodes to the **requesting user's** fallback
  codec (`h264` / `h265` / `vp9`, per-account user setting).
- Transcode reasons (codec / container / audio mismatch) are user-visible texts;
  keep them in sync with the "%s" templates in `web/src/i18n-ui.ts`.
- Transcoded streams have no arbitrary seeking yet (HLS/segment cache is roadmap).
- Kill the FFmpeg process when the client disconnects (request context).
