package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"

	archorgdl "github.com/averyfreeman/go-archorg-dl"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintf(os.Stderr, "archorg-dl: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("archorg-dl", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	outputDir := flags.String("output-dir", ".", "directory for the published MP4")
	flags.StringVar(outputDir, "o", ".", "directory for the published MP4")
	overwrite := flags.Bool("overwrite", false, "replace an existing output file")
	noResume := flags.Bool("no-resume", false, "discard partial files before downloading")
	durationText := flags.String("duration", "", "program duration in seconds for segmented fallback")
	segmentSeconds := flags.Int("segment-seconds", 300, "duration of each fallback segment")
	noFallback := flags.Bool("no-segment-fallback", false, "fail instead of using segmented fallback")
	keepWork := flags.Bool("keep-work", false, "keep segmented work files after success")
	workers := flags.Int("workers", 0, "maximum concurrent segment downloads (0: runtime.NumCPU())")
	ytdlpPath := flags.String("yt-dlp-path", "", "use this yt-dlp executable instead of go-ytdlp's resolver")
	verbose := flags.Bool("verbose", false, "log download and fallback progress")
	flags.BoolVar(verbose, "v", false, "log download and fallback progress")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("exactly one Archive.org details URL is required")
	}

	var duration *float64
	if *durationText != "" {
		parsed, err := strconv.ParseFloat(*durationText, 64)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", *durationText, err)
		}
		duration = &parsed
	}
	if *verbose {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	result, err := archorgdl.Download(ctx, archorgdl.Options{
		URL:               flags.Arg(0),
		OutputDir:         *outputDir,
		Overwrite:         *overwrite,
		NoResume:          *noResume,
		DurationSeconds:   duration,
		SegmentSeconds:    *segmentSeconds,
		NoSegmentFallback: *noFallback,
		KeepWork:          *keepWork,
		Workers:           *workers,
		YTDLPPath:         *ytdlpPath,
		Verbose:           *verbose,
	})
	if err != nil {
		return err
	}
	fmt.Println(result.OutputPath)
	return nil
}
