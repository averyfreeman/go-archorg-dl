package archorgdl

import (
	"bytes"
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type responseClient struct {
	fn func(*http.Request) (*http.Response, error)
}

func (c responseClient) Do(request *http.Request) (*http.Response, error) {
	return c.fn(request)
}

func response(status int, body string, headers map[string]string) *http.Response {
	header := make(http.Header)
	for key, value := range headers {
		header.Set(key, value)
	}
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestFileDownloaderStreamsAndVerifiesChecksum(t *testing.T) {
	content := []byte("video-data-video-data")
	digest := md5.Sum(content)
	client := responseClient{fn: func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Range") != "" {
			t.Fatalf("unexpected range request: %s", request.Header.Get("Range"))
		}
		return response(http.StatusOK, string(content), map[string]string{"Content-Length": strconv.Itoa(len(content))}), nil
	}}
	destination := filepath.Join(t.TempDir(), "program.mp4")
	size := int64(len(content))
	downloader := fileDownloader{client: client, retries: 0}
	if err := downloader.download(context.Background(), "https://example.invalid/program.mp4", destination, &size, md5Hex(digest[:]), "", true, false); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(destination); err != nil || !bytes.Equal(got, content) {
		t.Fatalf("destination = %q, %v", got, err)
	}
	if _, err := os.Stat(destination + ".part"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial file still exists: %v", err)
	}
}

func TestFileDownloaderResumesRange(t *testing.T) {
	content := []byte("0123456789")
	directory := t.TempDir()
	destination := filepath.Join(directory, "program.mp4")
	if err := os.WriteFile(destination+".part", content[:4], 0o644); err != nil {
		t.Fatal(err)
	}
	var requestedRange string
	client := responseClient{fn: func(request *http.Request) (*http.Response, error) {
		requestedRange = request.Header.Get("Range")
		return response(http.StatusPartialContent, string(content[4:]), nil), nil
	}}
	downloader := fileDownloader{client: client, retries: 0}
	if err := downloader.download(context.Background(), "https://example.invalid/program.mp4", destination, nil, "", "", true, false); err != nil {
		t.Fatal(err)
	}
	if requestedRange != "bytes=4-" {
		t.Fatalf("range = %q", requestedRange)
	}
	got, err := os.ReadFile(destination)
	if err != nil || !bytes.Equal(got, content) {
		t.Fatalf("destination = %q, %v", got, err)
	}
}

func TestFileDownloaderOverwritesExistingDestination(t *testing.T) {
	directory := t.TempDir()
	destination := filepath.Join(directory, "program.mp4")
	if err := os.WriteFile(destination, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := responseClient{fn: func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Range") != "" {
			t.Fatalf("unexpected range request: %s", request.Header.Get("Range"))
		}
		return response(http.StatusOK, "new", nil), nil
	}}
	downloader := fileDownloader{client: client, retries: 0}
	if err := downloader.download(context.Background(), "https://example.invalid/program.mp4", destination, nil, "", "", true, true); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination)
	if err != nil || string(got) != "new" {
		t.Fatalf("destination = %q, %v", got, err)
	}
}

func TestDownloadSegmentsBoundsConcurrencyAndPreservesOrder(t *testing.T) {
	var active, maximum atomic.Int32
	var mu sync.Mutex
	var requested []string
	client := responseClient{fn: func(request *http.Request) (*http.Response, error) {
		current := active.Add(1)
		for {
			old := maximum.Load()
			if current <= old || maximum.CompareAndSwap(old, current) {
				break
			}
		}
		defer active.Add(-1)
		mu.Lock()
		requested = append(requested, request.URL.String())
		mu.Unlock()
		time.Sleep(10 * time.Millisecond)
		return response(http.StatusOK, request.URL.Path, nil), nil
	}}
	downloader := fileDownloader{client: client, retries: 0}
	segments := []segment{
		{Index: 0, Start: 0, End: 1, URL: "https://example.invalid/0"},
		{Index: 1, Start: 1, End: 2, URL: "https://example.invalid/1"},
		{Index: 2, Start: 2, End: 3, URL: "https://example.invalid/2"},
		{Index: 3, Start: 3, End: 4, URL: "https://example.invalid/3"},
	}
	paths, err := downloadSegments(context.Background(), &downloader, segments, t.TempDir(), 2, true, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if maximum.Load() > 2 {
		t.Fatalf("maximum concurrent downloads = %d", maximum.Load())
	}
	for i, filename := range paths {
		data, err := os.ReadFile(filename)
		if err != nil {
			t.Fatalf("path %d missing: %v", i, err)
		}
		if string(data) != "/"+strconv.Itoa(i) {
			t.Fatalf("path %d contains %q", i, data)
		}
	}
}

func TestDownloadSegmentsReusesExistingFilesWhenRequested(t *testing.T) {
	workdir := t.TempDir()
	existing := filepath.Join(workdir, "00000.mp4")
	if err := os.WriteFile(existing, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	called := false
	client := responseClient{fn: func(request *http.Request) (*http.Response, error) {
		called = true
		return response(http.StatusOK, "replacement", nil), nil
	}}
	downloader := fileDownloader{client: client, retries: 0}
	segments := []segment{{Index: 0, Start: 0, End: 1, URL: "https://example.invalid/0"}}
	paths, err := downloadSegments(context.Background(), &downloader, segments, workdir, 1, true, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("existing segment was downloaded despite reuse request")
	}
	if len(paths) != 1 || paths[0] != existing {
		t.Fatalf("paths = %#v", paths)
	}
}

func md5Hex(value []byte) string {
	return fmt.Sprintf("%x", value)
}
