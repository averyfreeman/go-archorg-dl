# go-archorg-dl

`archorg-dl` downloads one complete television Program from an Archive.org details URL.

The downloader follows a metadata-first path. It prefers a complete MP4 and downloads it directly with retries, HTTP Range resume, checksum verification, and atomic publication. If the direct file fails, it resolves time-bounded Segment URLs once through go-ytdlp, downloads those progressive MP4 files concurrently, and assembles them with the system `ffmpeg` concat demuxer.

## Requirements

- Go 1.26 or newer
- A working `ffmpeg` executable on `PATH` for segmented fallback

On macOS, Homebrew provides the native dependency:

```sh
brew install ffmpeg
```

The project invokes the system `ffmpeg` binary only for segmented assembly. go-ytdlp is used as a Go adapter for metadata extraction and may manage its own versioned yt-dlp executable/cache. Use `--yt-dlp-path` to provide a specific executable.

## Usage

```sh
go run ./cmd/archorg-dl \
  'https://archive.org/details/CNNW_20260913_030000_Real_Time_With_Bill_Maher'
```

By default the output is `<archive-identifier>.mp4` in the current directory. Useful options:

```text
-o, --output-dir DIR       output directory
    --overwrite            replace existing output and segmented work files (default true)
    --no-overwrite         refuse to replace an existing published MP4
    --no-resume             discard .part files before downloading
    --duration SECONDS      fallback duration override
    --segment-seconds N     fallback segment length (default: 300)
    --no-segment-fallback   disable segmented fallback
    --keep-work             retain successful fallback work files (default true)
    --cleanup-work          remove fallback work files after success
    --reuse-work            reuse completed fallback segments instead of refreshing them
    --workers N             concurrent Segment downloads (default: NumCPU)
    --yt-dlp-path PATH      use a user-managed yt-dlp executable
-v, --verbose               increase diagnostic detail; repeat up to -vvv
```

The input may include additional Archive.org media, range, query, or fragment components after `/details/{identifier}`; the downloader uses the item identifier and derives its own Segment plan.

A failed or successful fallback retains `.<identifier>.work` by default. Existing Segment files are refreshed on the next run while `.part` files resume unless `--no-resume` is set. Use `--reuse-work` to reuse completed Segment files or `--cleanup-work` to remove the work directory after success.

The work directory contains the ordered `filelist.txt`, downloaded Segment files, partial files, and captured `yt-dlp`/`ffmpeg` command, status, stdout, and stderr artifacts. `-v` shows lifecycle and Segment progress, `-vv` adds detailed command information, and `-vvv` also prints raw external-process output.

## Library use

```go
import (
    "context"
    "fmt"

    archorgdl "github.com/averyfreeman/go-archorg-dl"
)

ctx := context.Background()
result, err := archorgdl.Download(ctx, archorgdl.Options{
    URL:       "https://archive.org/details/example_item",
    Workers:   0, // runtime.NumCPU()
})
if err != nil {
    return err
}
fmt.Println(result.OutputPath)
```

Library defaults match the CLI: existing published output and fallback Segment files are replaced, fallback work is retained, and verbosity is disabled. Set `NoOverwrite`, `ReuseWork`, `CleanupWork`, or `Verbosity` when a caller needs a different policy.

## Development

```sh
gofmt -w *.go cmd/archorg-dl/main.go
go test ./...
go test -race ./...
go vet ./...
```

The live smoke check is intentionally separate from CI because it contacts Archive.org and never downloads a complete television program. It checks metadata and yt-dlp extraction only.

This project is licensed under the GNU General Public License v3 or later.
