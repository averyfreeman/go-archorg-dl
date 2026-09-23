# go-archorg-dl

`archorg-dl` downloads one complete television Program from an Archive.org details URL.

The downloader follows a metadata-first path. It prefers a complete MP4 and downloads it directly with retries, HTTP Range resume, checksum verification, and atomic publication. If the direct file fails, it resolves time-bounded Segment URLs once through go-ytdlp, downloads those progressive MP4 files concurrently, and assembles them in-process through FFmpeg's concat demuxer using go-astiav.

## Requirements

- Go 1.26 or newer
- FFmpeg 9 development headers and libraries
- CGO and pkg-config

On macOS, Homebrew provides the native dependency:

```sh
brew install ffmpeg
```

The FFmpeg binary is not invoked by this project. go-ytdlp is used as a Go adapter for metadata extraction and may manage its own versioned yt-dlp executable/cache. Use `--yt-dlp-path` to provide a specific executable.

## Usage

```sh
go run ./cmd/archorg-dl \
  'https://archive.org/details/CNNW_20260913_030000_Real_Time_With_Bill_Maher'
```

By default the output is `<archive-identifier>.mp4` in the current directory. Useful options:

```text
-o, --output-dir DIR       output directory
    --overwrite            replace an existing output
    --no-resume             discard .part files before downloading
    --duration SECONDS      fallback duration override
    --segment-seconds N     fallback segment length (default: 300)
    --no-segment-fallback   disable segmented fallback
    --keep-work             retain successful fallback work files
    --workers N             concurrent Segment downloads (default: NumCPU)
    --yt-dlp-path PATH      use a user-managed yt-dlp executable
-v, --verbose               log progress and retained work paths
```

A failed fallback retains `.<identifier>.work` so completed Segment files and `.part` files can be diagnosed or resumed. A successful fallback removes that directory unless `--keep-work` is set.

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
    KeepWork:  true,
})
if err != nil {
    return err
}
fmt.Println(result.OutputPath)
```

## Development

```sh
gofmt -w *.go cmd/archorg-dl/main.go
go test ./...
go test -race ./...
go vet ./...
```

The live smoke check is intentionally separate from CI because it contacts Archive.org and never downloads a complete television program. It checks metadata and yt-dlp extraction only.

This project is licensed under the GNU General Public License v3 or later.
