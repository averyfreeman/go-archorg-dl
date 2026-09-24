package archorgdl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type processRunner func(context.Context, string, []string, string) (processResult, error)

func assembleSegments(ctx context.Context, segmentPaths []string, destination, workdir string, overwrite bool, verbosity int) error {
	return assembleSegmentsWithRunner(ctx, segmentPaths, destination, workdir, overwrite, verbosity, runProcess, "")
}

func assembleSegmentsWithRunner(ctx context.Context, segmentPaths []string, destination, workdir string, overwrite bool, verbosity int, runner processRunner, executable string) error {
	if len(segmentPaths) == 0 {
		return errors.New("cannot assemble an empty segment list")
	}
	if _, err := os.Stat(destination); err == nil && !overwrite {
		return fmt.Errorf("%w: %s", ErrOutputExists, destination)
	}
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		return wrapError("create assembly directory", err)
	}
	manifest := filepath.Join(workdir, "filelist.txt")
	if err := writeConcatManifest(manifest, workdir, segmentPaths); err != nil {
		return wrapError("write concat manifest", err)
	}
	partial := filepath.Join(workdir, "."+filepath.Base(destination)+".assembled.part")
	if err := os.Remove(partial); err != nil && !errors.Is(err, os.ErrNotExist) {
		return wrapError("remove previous assembly output", err)
	}

	if executable == "" {
		var err error
		executable, err = exec.LookPath("ffmpeg")
		if err != nil {
			result := processResult{ExitCode: 127, Stderr: err.Error()}
			if persistErr := persistProcessResult(workdir, "ffmpeg", result); persistErr != nil {
				return fmt.Errorf("assembly: ffmpeg is unavailable: %v; save diagnostics: %w", err, persistErr)
			}
			return fmt.Errorf("assembly: ffmpeg is unavailable: %w", err)
		}
	}

	args := []string{
		"-hide_banner",
		"-nostdin",
		"-loglevel", ffmpegLogLevel(verbosity),
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", filepath.Base(manifest),
		"-c", "copy",
		"-f", "mp4",
		filepath.Base(partial),
	}
	logInfo(verbosity, "assembling segments with ffmpeg", "segments", len(segmentPaths), "workdir", workdir)
	logDebug(verbosity, "ffmpeg command", "executable", executable, "args", args)
	result, runErr := runner(ctx, executable, args, workdir)
	if result.Executable == "" {
		result.Executable = executable
	}
	if len(result.Args) == 0 {
		result.Args = args
	}
	if err := persistProcessResult(workdir, "ffmpeg", result); err != nil {
		return wrapError("save ffmpeg diagnostics", err)
	}
	logRawProcessResult(verbosity, "ffmpeg", result)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if runErr != nil || result.ExitCode != 0 {
		return fmt.Errorf("assembly: ffmpeg failed: %w", processFailure(result, runErr))
	}
	if info, err := os.Stat(partial); err != nil {
		return fmt.Errorf("assembly: ffmpeg did not create output: %w", err)
	} else if info.Size() == 0 {
		return errors.New("assembly: ffmpeg created an empty output")
	}
	if err := replaceFile(partial, destination, overwrite); err != nil {
		return err
	}
	return nil
}

func ffmpegLogLevel(verbosity int) string {
	switch {
	case verbosity >= maxVerbosity:
		return "verbose"
	case verbosity >= 2:
		return "info"
	default:
		return "error"
	}
}

func runProcess(ctx context.Context, executable string, args []string, workdir string) (processResult, error) {
	command := exec.CommandContext(ctx, executable, args...)
	command.Dir = workdir
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	result := processResult{
		Executable: executable,
		Args:       args,
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
	}
	if command.ProcessState != nil {
		result.ExitCode = command.ProcessState.ExitCode()
	}
	return result, err
}

func writeConcatManifest(filename, workdir string, segmentPaths []string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	for _, segmentPath := range segmentPaths {
		relative, err := filepath.Rel(workdir, segmentPath)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		escaped := strings.ReplaceAll(relative, "'", "'\\''")
		if _, err := fmt.Fprintf(file, "file '%s'\n", escaped); err != nil {
			return err
		}
	}
	return nil
}
