---
status: accepted
---

# Assemble progressive segments with astiav and FFmpeg concat

Segment files are assembled through FFmpeg's concat demuxer and stream-copy remux API exposed by go-astiav, producing an MP4 without an FFmpeg subprocess. This is preferred over a custom packet timestamp muxer or a pure-Go progressive MP4 joiner because the input files carry audio/video timing and container details that FFmpeg already understands. The final assembly remains ordered and serial; only segment downloads are concurrent.

## Consequences

- The build requires CGO, FFmpeg 9 development headers/libraries, and pkg-config.
- Segment streams must be compatible enough for concat demuxing and stream copy; the implementation does not transcode incompatible media.
- Pure-Go alternatives such as go-mp4 or mp4ff remain possible future replacements but are not the v0.1.0 path.
