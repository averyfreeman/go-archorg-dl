package archorgdl

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/lrstanley/go-ytdlp"
)

type segmentExtractor interface {
	Resolve(context.Context, []string) ([]string, error)
}

type ytdlpExtractor struct {
	executable string
	workdir    string
	verbosity  int
}

func (e ytdlpExtractor) Resolve(ctx context.Context, urls []string) ([]string, error) {
	if len(urls) == 0 {
		return nil, errors.New("cannot extract an empty segment list")
	}
	command := ytdlp.New().
		NoUpdate().
		NoPlaylist().
		SkipDownload().
		DumpJSON().
		Format("best[ext=mp4]/best")
	if e.verbosity >= maxVerbosity {
		command.Verbose()
	} else {
		command.NoWarnings()
	}
	if e.executable != "" {
		command.SetExecutable(e.executable)
	} else {
		resolved, err := ytdlp.Install(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("install or resolve yt-dlp: %w", err)
		}
		command.SetExecutable(resolved.Executable)
	}
	result, runErr := command.Run(ctx, urls...)
	if result != nil {
		captured := processResult{
			Executable: result.Executable,
			Args:       result.Args,
			ExitCode:   result.ExitCode,
			Stdout:     result.Stdout,
			Stderr:     result.Stderr,
		}
		if e.workdir != "" {
			if err := persistProcessResult(e.workdir, "yt-dlp", captured); err != nil {
				return nil, fmt.Errorf("save yt-dlp diagnostics: %w", err)
			}
		}
		logRawProcessResult(e.verbosity, "yt-dlp", captured)
	}
	if runErr != nil {
		return nil, fmt.Errorf("extract segment URLs with yt-dlp: %w", runErr)
	}
	info, err := result.GetExtractedInfo()
	if err != nil {
		return nil, fmt.Errorf("parse yt-dlp metadata: %w", err)
	}
	if len(info) != len(urls) {
		return nil, fmt.Errorf("yt-dlp returned %d results for %d segments", len(info), len(urls))
	}
	resolvedURLs := make([]string, len(info))
	for i, item := range info {
		if item == nil || item.URL == nil || strings.TrimSpace(*item.URL) == "" {
			return nil, fmt.Errorf("yt-dlp returned no progressive URL for segment %d", i)
		}
		if item.Extension != "" && !strings.EqualFold(item.Extension, "mp4") {
			return nil, fmt.Errorf("yt-dlp returned non-MP4 segment %d with extension %q", i, item.Extension)
		}
		resolvedURLs[i] = *item.URL
	}
	return resolvedURLs, nil
}
