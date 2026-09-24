//go:build integration

package archorgdl

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFFmpegAssembly(t *testing.T) {
	fixture := os.Getenv("ARCHORG_MP4_FIXTURE")
	if fixture == "" {
		t.Skip("set ARCHORG_MP4_FIXTURE to a small progressive MP4 fixture")
	}
	if _, err := os.Stat(fixture); err != nil {
		t.Fatal(err)
	}
	workdir := t.TempDir()
	segmentOne := filepath.Join(workdir, "00000.mp4")
	segmentTwo := filepath.Join(workdir, "00001.mp4")
	for _, destination := range []string{segmentOne, segmentTwo} {
		data, err := os.ReadFile(fixture)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	output := filepath.Join(workdir, "assembled.mp4")
	if err := assembleSegments(context.Background(), []string{segmentOne, segmentTwo}, output, workdir, true, 0); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(output)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("assembly output is empty")
	}
}
