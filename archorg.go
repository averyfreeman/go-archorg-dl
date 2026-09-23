package archorgdl

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
)

// Options controls one Archive.org program download.
type Options struct {
	URL               string
	OutputDir         string
	Overwrite         bool
	NoResume          bool
	DurationSeconds   *float64
	SegmentSeconds    int
	NoSegmentFallback bool
	KeepWork          bool
	Workers           int
	YTDLPPath         string
	Verbose           bool
}

// Result describes the published program file.
type Result struct {
	OutputPath          string
	Identifier          string
	UsedSegmentFallback bool
}

func Download(ctx context.Context, options Options) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateOptions(options); err != nil {
		return Result{}, err
	}
	parsedURL, err := parseArchiveURL(options.URL)
	if err != nil {
		return Result{}, err
	}
	outputDir := options.OutputDir
	if outputDir == "" {
		outputDir = "."
	}
	outputDir = filepath.Clean(outputDir)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return Result{}, wrapError("create output directory", err)
	}
	destination := filepath.Join(outputDir, safeFilename(parsedURL.Identifier)+".mp4")
	if _, err := os.Stat(destination); err == nil && !options.Overwrite {
		return Result{}, fmt.Errorf("%w: %s", ErrOutputExists, destination)
	}

	client := &archiveClient{client: newHTTPClient(), retries: defaultRetries, retryDelay: defaultRetryDelay}
	item, err := client.getItem(ctx, parsedURL.Identifier)
	if err != nil {
		return Result{}, err
	}
	candidate, ok := selectVideoFile(item)
	if !ok {
		return Result{}, ErrNoVideo
	}
	sourceURL := archiveDownloadURL(parsedURL.Identifier, candidate.Name)
	duration := options.DurationSeconds
	if duration == nil {
		duration = candidate.LengthSeconds
	}
	if duration == nil {
		duration = item.DurationSeconds
	}
	downloader := &fileDownloader{client: newHTTPClient(), retries: defaultRetries, retryDelay: defaultRetryDelay}

	if candidate.isMP4() {
		if options.Verbose {
			slog.Default().Info("downloading complete MP4", "file", candidate.Name)
		}
		err = downloader.download(ctx, sourceURL, destination, candidate.Size, candidate.MD5, candidate.SHA1, !options.NoResume, options.Overwrite)
		if err == nil {
			return Result{OutputPath: destination, Identifier: parsedURL.Identifier}, nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrOutputExists) {
			return Result{}, err
		}
		if options.NoSegmentFallback {
			return Result{}, err
		}
		if options.Verbose {
			slog.Default().Info("direct download failed; trying segmented fallback", "error", err)
		}
	} else if options.NoSegmentFallback {
		return Result{}, fmt.Errorf("selected video is not an MP4: %s", candidate.Name)
	}

	if duration == nil {
		return Result{}, fmt.Errorf("%w; supply --duration", ErrNoDuration)
	}
	segmentSeconds := options.SegmentSeconds
	if segmentSeconds == 0 {
		segmentSeconds = defaultSegmentSec
	}
	segments, err := buildSegmentPlan(sourceURL, *duration, segmentSeconds)
	if err != nil {
		return Result{}, err
	}
	extractor := ytdlpExtractor{executable: options.YTDLPPath}
	segmentURLs := make([]string, len(segments))
	for i := range segments {
		segmentURLs[i] = segments[i].URL
	}
	resolvedURLs, err := extractor.Resolve(ctx, segmentURLs)
	if err != nil {
		return Result{}, err
	}
	for i := range segments {
		segments[i].URL = resolvedURLs[i]
	}

	workdir := filepath.Join(outputDir, "."+safeFilename(parsedURL.Identifier)+".work")
	paths, err := downloadSegments(ctx, downloader, segments, workdir, options.Workers, !options.NoResume)
	if err != nil {
		if options.Verbose {
			slog.Default().Error("segmented download failed; work retained", "workdir", workdir, "error", err)
		}
		return Result{}, err
	}
	if err := assembleSegments(ctx, paths, destination, workdir, options.Overwrite); err != nil {
		if options.Verbose {
			slog.Default().Error("assembly failed; work retained", "workdir", workdir, "error", err)
		}
		return Result{}, err
	}
	if !options.KeepWork {
		_ = os.RemoveAll(workdir)
	}
	return Result{OutputPath: destination, Identifier: parsedURL.Identifier, UsedSegmentFallback: true}, nil
}

func validateOptions(options Options) error {
	if options.URL == "" {
		return errors.New("an Archive.org details URL is required")
	}
	if options.DurationSeconds != nil && (*options.DurationSeconds <= 0 || *options.DurationSeconds != *options.DurationSeconds) {
		return errors.New("duration must be greater than zero")
	}
	if options.SegmentSeconds < 0 {
		return errors.New("segment duration must not be negative")
	}
	if options.Workers < 0 {
		return errors.New("workers must not be negative")
	}
	if options.Workers == 0 && runtime.NumCPU() < 1 {
		return errors.New("runtime reported no CPUs")
	}
	return nil
}
