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
	"strings"

	archorgdl "github.com/averyfreeman/go-archorg-dl"
)

type verbosityFlag struct {
	level *int
}

func (f verbosityFlag) String() string {
	if f.level == nil {
		return "0"
	}
	return strconv.Itoa(*f.level)
}

func (f verbosityFlag) IsBoolFlag() bool { return true }

func (f verbosityFlag) Set(value string) error {
	if f.level == nil {
		return errors.New("verbosity flag is not configured")
	}
	switch value {
	case "true":
		if *f.level < 3 {
			*f.level = *f.level + 1
		}
		return nil
	case "false":
		*f.level = 0
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fmt.Errorf("verbosity must be a non-negative integer")
	}
	if parsed > 3 {
		parsed = 3
	}
	*f.level = parsed
	return nil
}

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
	options, err := parseOptions(args)
	if err != nil {
		return err
	}
	configureLogging(options.Verbosity)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	result, err := archorgdl.Download(ctx, options)
	if err != nil {
		return err
	}
	fmt.Println(result.OutputPath)
	return nil
}

func parseOptions(args []string) (archorgdl.Options, error) {
	flags := flag.NewFlagSet("archorg-dl", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	outputDir := flags.String("output-dir", ".", "directory for the published MP4")
	flags.StringVar(outputDir, "o", ".", "directory for the published MP4")
	overwrite := flags.Bool("overwrite", true, "replace existing output and segmented work files")
	noOverwrite := flags.Bool("no-overwrite", false, "refuse to replace an existing published MP4")
	noResume := flags.Bool("no-resume", false, "discard partial files before downloading")
	durationText := flags.String("duration", "", "program duration in seconds for segmented fallback")
	segmentSeconds := flags.Int("segment-seconds", 300, "duration of each fallback segment")
	noFallback := flags.Bool("no-segment-fallback", false, "fail instead of using segmented fallback")
	keepWork := flags.Bool("keep-work", true, "retain segmented work files after success")
	cleanupWork := flags.Bool("cleanup-work", false, "remove segmented work files after success")
	reuseWork := flags.Bool("reuse-work", false, "reuse completed segment files instead of refreshing them")
	workers := flags.Int("workers", 0, "maximum concurrent segment downloads (0: runtime.NumCPU())")
	ytdlpPath := flags.String("yt-dlp-path", "", "use this yt-dlp executable instead of go-ytdlp's resolver")
	verbosity := 0
	verbose := verbosityFlag{level: &verbosity}
	flags.Var(verbose, "verbose", "increase diagnostic detail; repeat up to -vvv")
	flags.Var(verbose, "v", "increase diagnostic detail; repeat up to -vvv")
	if err := flags.Parse(normalizeVerbosityArgs(args)); err != nil {
		return archorgdl.Options{}, err
	}
	if flags.NArg() != 1 {
		return archorgdl.Options{}, errors.New("exactly one Archive.org details URL is required")
	}

	var duration *float64
	if *durationText != "" {
		parsed, err := strconv.ParseFloat(*durationText, 64)
		if err != nil {
			return archorgdl.Options{}, fmt.Errorf("invalid duration %q: %w", *durationText, err)
		}
		duration = &parsed
	}
	return archorgdl.Options{
		URL:               flags.Arg(0),
		OutputDir:         *outputDir,
		NoOverwrite:       !*overwrite || *noOverwrite,
		NoResume:          *noResume,
		DurationSeconds:   duration,
		SegmentSeconds:    *segmentSeconds,
		NoSegmentFallback: *noFallback,
		CleanupWork:       !*keepWork || *cleanupWork,
		ReuseWork:         !*overwrite || *reuseWork,
		Workers:           *workers,
		YTDLPPath:         *ytdlpPath,
		Verbosity:         verbosity,
	}, nil
}

func normalizeVerbosityArgs(args []string) []string {
	normalized := make([]string, 0, len(args))
	for _, arg := range args {
		if len(arg) > 2 && strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && strings.Trim(arg[1:], "v") == "" {
			count := len(arg) - 1
			if count > 3 {
				count = 3
			}
			normalized = append(normalized, fmt.Sprintf("-v=%d", count))
			continue
		}
		normalized = append(normalized, arg)
	}
	return normalized
}

func configureLogging(verbosity int) {
	level := slog.LevelError
	if verbosity >= 1 {
		level = slog.LevelInfo
	}
	if verbosity >= 2 {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
}
