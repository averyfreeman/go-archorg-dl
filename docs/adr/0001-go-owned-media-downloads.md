---
status: accepted
---

# Use Go-owned media downloads behind library adapters

The downloader uses Archive.org metadata and native Go HTTP for the primary MP4 path, then invokes one metadata-only extraction pass through go-ytdlp when segmented fallback is required. The project owns retries, Range resume, `.part` files, checksums, worker cancellation, and publication; go-ytdlp is deliberately limited to resolving progressive URLs. The separate FFmpeg assembly process is specified by ADR 0003 and is not delegated to go-ytdlp.

## Considered options

- Use yt-dlp for every download: rejected because it would surrender resume, checksum, and worker ownership to the extractor.
- Reimplement Archive.org extraction natively: deferred because yt-dlp already handles the fallback URL shape and compatibility surface.
