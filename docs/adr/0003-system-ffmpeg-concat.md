---
status: accepted
---

# Assemble progressive segments with the system FFmpeg concat process

Segment files are assembled by invoking the verified system `ffmpeg` executable with a retained `filelist.txt` concat manifest. The command uses stream-copy remuxing to an MP4 partial file and publishes that file atomically after a successful process exit. Standard output, standard error, command arguments, exit status, manifest, and partial output remain in the fallback work directory so a failed assembly can be inspected and retried. This replaces the astiav implementation from ADR 0002 because the system binary is already available and provides the most direct diagnostic path.

The downloader continues to own Segment discovery, bounded concurrent downloads, resume behavior, work retention, and publication. It does not transcode incompatible Segment streams; FFmpeg remains responsible for the concat demuxer and stream-copy compatibility checks.
