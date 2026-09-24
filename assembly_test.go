package archorgdl

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssembleSegmentsUsesFilelistAndPersistsProcessOutput(t *testing.T) {
	workdir := t.TempDir()
	segmentPaths := []string{
		filepath.Join(workdir, "00000.mp4"),
		filepath.Join(workdir, "00001.mp4"),
	}
	for _, filename := range segmentPaths {
		if err := os.WriteFile(filename, []byte("segment"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	destination := filepath.Join(workdir, "program.mp4")
	runner := func(_ context.Context, executable string, args []string, dir string) (processResult, error) {
		if executable != "/usr/bin/ffmpeg" {
			t.Fatalf("executable = %q", executable)
		}
		if args[0] != "-hide_banner" || args[5] != "-f" || args[6] != "concat" || args[7] != "-safe" || args[8] != "0" || args[9] != "-i" || args[10] != "filelist.txt" {
			t.Fatalf("unexpected ffmpeg args = %#v", args)
		}
		if err := os.WriteFile(filepath.Join(dir, args[len(args)-1]), []byte("assembled"), 0o644); err != nil {
			t.Fatal(err)
		}
		return processResult{Stdout: "ffmpeg stdout\n", Stderr: "ffmpeg stderr\n"}, nil
	}

	if err := assembleSegmentsWithRunner(context.Background(), segmentPaths, destination, workdir, true, 2, runner, "/usr/bin/ffmpeg"); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(destination); err != nil || string(got) != "assembled" {
		t.Fatalf("destination = %q, %v", got, err)
	}
	manifest, err := os.ReadFile(filepath.Join(workdir, "filelist.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(manifest), "file '00000.mp4'\nfile '00001.mp4'\n"; got != want {
		t.Fatalf("manifest = %q, want %q", got, want)
	}
	for filename, want := range map[string]string{
		"ffmpeg.command": `"/usr/bin/ffmpeg" "-hide_banner"`,
		"ffmpeg.stdout":  "ffmpeg stdout\n",
		"ffmpeg.stderr":  "ffmpeg stderr\n",
		"ffmpeg.status":  "0\n",
	} {
		data, err := os.ReadFile(filepath.Join(workdir, filename))
		if err != nil {
			t.Fatalf("read %s: %v", filename, err)
		}
		if !strings.Contains(string(data), want) {
			t.Fatalf("%s = %q, want to contain %q", filename, data, want)
		}
	}
}

func TestAssembleSegmentsRetainsDiagnosticsOnFailure(t *testing.T) {
	workdir := t.TempDir()
	segment := filepath.Join(workdir, "00000.mp4")
	if err := os.WriteFile(segment, []byte("segment"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := func(_ context.Context, _ string, args []string, dir string) (processResult, error) {
		if err := os.WriteFile(filepath.Join(dir, args[len(args)-1]), []byte("partial"), 0o644); err != nil {
			t.Fatal(err)
		}
		return processResult{ExitCode: 1, Stderr: "concat failed"}, errors.New("exit status 1")
	}

	err := assembleSegmentsWithRunner(context.Background(), []string{segment}, filepath.Join(workdir, "program.mp4"), workdir, true, 0, runner, "ffmpeg")
	if err == nil || !strings.Contains(err.Error(), "concat failed") {
		t.Fatalf("error = %v", err)
	}
	for _, filename := range []string{"filelist.txt", "ffmpeg.command", "ffmpeg.stderr", ".program.mp4.assembled.part"} {
		if _, statErr := os.Stat(filepath.Join(workdir, filename)); statErr != nil {
			t.Errorf("expected retained %s: %v", filename, statErr)
		}
	}
}
