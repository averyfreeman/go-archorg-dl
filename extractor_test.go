package archorgdl

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestYTDLPExtractorPersistsProcessOutputOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test fixture uses a POSIX shell")
	}
	workdir := t.TempDir()
	executable := filepath.Join(workdir, "fake-yt-dlp")
	script := "#!/bin/sh\necho resolver-output\necho resolver-error >&2\nexit 7\n"
	if err := os.WriteFile(executable, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := (ytdlpExtractor{executable: executable, workdir: workdir}).Resolve(context.Background(), []string{"https://example.invalid/segment"})
	if err == nil || !strings.Contains(err.Error(), "extract segment URLs") {
		t.Fatalf("error = %v", err)
	}
	for filename, want := range map[string]string{
		"yt-dlp.stdout": "resolver-output",
		"yt-dlp.stderr": "resolver-error",
		"yt-dlp.status": "7",
	} {
		data, readErr := os.ReadFile(filepath.Join(workdir, filename))
		if readErr != nil {
			t.Fatalf("read %s: %v", filename, readErr)
		}
		if !strings.Contains(string(data), want) {
			t.Fatalf("%s = %q, want %q", filename, data, want)
		}
	}
}
