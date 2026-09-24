package archorgdl

import (
	"context"
	"crypto/md5"
	"crypto/sha1"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const downloadBufferSize = 1024 * 1024

type fileDownloader struct {
	client     httpDoer
	retries    int
	retryDelay time.Duration
}

func (d *fileDownloader) download(ctx context.Context, rawURL string, destination string, expectedSize *int64, expectedMD5, expectedSHA1 string, resume, overwrite bool) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return wrapError("create output directory", err)
	}
	if _, err := os.Stat(destination); err == nil && !overwrite {
		return fmt.Errorf("%w: %s", ErrOutputExists, destination)
	}
	partial := destination + ".part"
	if !resume {
		if err := os.Remove(partial); err != nil && !errors.Is(err, os.ErrNotExist) {
			return wrapError("remove partial file", err)
		}
	}

	if expectedSize != nil {
		if info, err := os.Stat(partial); err == nil && info.Size() == *expectedSize {
			if err := verifyFile(partial, expectedSize, expectedMD5, expectedSHA1); err == nil {
				return replaceFile(partial, destination, overwrite)
			}
		}
	}

	var lastErr error
	for attempt := 0; attempt <= d.retries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		offset := int64(0)
		if resume {
			if info, err := os.Stat(partial); err == nil {
				offset = info.Size()
			}
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
		if err != nil {
			return wrapError("create download request", err)
		}
		request.Header.Set("Accept", "video/mp4,video/*;q=0.9,*/*;q=0.1")
		if offset > 0 {
			request.Header.Set("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")
		}
		response, requestErr := d.client.Do(request)
		if requestErr != nil {
			lastErr = requestErr
		} else {
			lastErr = d.writeResponse(response, partial, offset, expectedSize, expectedMD5, expectedSHA1)
			response.Body.Close()
			if lastErr == nil {
				return replaceFile(partial, destination, overwrite)
			}
		}
		if !retryableError(lastErr) || attempt == d.retries {
			break
		}
		if err := waitRetry(ctx, d.retryDelay, attempt+1); err != nil {
			return err
		}
	}
	return wrapError("download file", fmt.Errorf("download failed after %d attempt(s): %w", d.retries+1, lastErr))
}

func (d *fileDownloader) writeResponse(response *http.Response, partial string, offset int64, expectedSize *int64, expectedMD5, expectedSHA1 string) error {
	if response == nil {
		return errors.New("empty HTTP response")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		if retryableStatuses[response.StatusCode] {
			return fmt.Errorf("transient HTTP status %d", response.StatusCode)
		}
		return fmt.Errorf("HTTP %d", response.StatusCode)
	}
	mode := os.O_CREATE | os.O_WRONLY
	if offset > 0 && response.StatusCode == http.StatusPartialContent {
		mode |= os.O_APPEND
	} else {
		mode |= os.O_TRUNC
		offset = 0
	}
	stream, err := os.OpenFile(partial, mode, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.CopyBuffer(stream, response.Body, make([]byte, downloadBufferSize))
	closeErr := stream.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if info, statErr := os.Stat(partial); statErr != nil {
		return statErr
	} else if info.Size() == 0 {
		return errors.New("empty response")
	}
	return verifyFile(partial, expectedSize, expectedMD5, expectedSHA1)
}

func verifyFile(filename string, expectedSize *int64, expectedMD5, expectedSHA1 string) error {
	info, err := os.Stat(filename)
	if err != nil {
		return err
	}
	if expectedSize != nil && info.Size() != *expectedSize {
		return fmt.Errorf("size mismatch: expected %d, got %d", *expectedSize, info.Size())
	}
	if expectedMD5 == "" && expectedSHA1 == "" {
		return nil
	}
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	var hashes []hash.Hash
	var writers []io.Writer
	var expected []string
	if expectedMD5 != "" {
		digest := md5.New()
		hashes = append(hashes, digest)
		writers = append(writers, digest)
		expected = append(expected, strings.ToLower(expectedMD5))
	}
	if expectedSHA1 != "" {
		digest := sha1.New()
		hashes = append(hashes, digest)
		writers = append(writers, digest)
		expected = append(expected, strings.ToLower(expectedSHA1))
	}
	writer := io.MultiWriter(writers...)
	if _, err := io.CopyBuffer(writer, file, make([]byte, downloadBufferSize)); err != nil {
		return err
	}
	for i, digest := range hashes {
		if actual := fmt.Sprintf("%x", digest.Sum(nil)); actual != expected[i] {
			return fmt.Errorf("checksum mismatch: expected %s, got %s", expected[i], actual)
		}
	}
	return nil
}

func replaceFile(partial, destination string, overwrite bool) error {
	if overwrite && runtime.GOOS == "windows" {
		if err := os.Remove(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
			return wrapError("replace output", err)
		}
	}
	if err := os.Rename(partial, destination); err != nil {
		return wrapError("publish output", err)
	}
	return nil
}

func downloadSegments(ctx context.Context, downloader *fileDownloader, segments []segment, workdir string, workers int, resume, overwrite bool, verbosity int) ([]string, error) {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if workers < 1 {
		workers = 1
	}
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		return nil, wrapError("create work directory", err)
	}
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	paths := make([]string, len(segments))
	jobs := make(chan segment)
	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex
	worker := func() {
		defer wg.Done()
		for {
			select {
			case <-childCtx.Done():
				return
			case current, ok := <-jobs:
				if !ok {
					return
				}
				filename := filepath.Join(workdir, fmt.Sprintf("%05d.mp4", current.Index))
				if !overwrite {
					if info, err := os.Stat(filename); err == nil && info.Size() > 0 {
						paths[current.Index] = filename
						logInfo(verbosity, "reusing segment", "segment", current.Index, "start", current.Start, "end", current.End)
						continue
					} else if err == nil {
						if removeErr := os.Remove(filename); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
							errMu.Lock()
							if firstErr == nil {
								firstErr = fmt.Errorf("remove empty segment %d: %w", current.Index, removeErr)
								cancel()
							}
							errMu.Unlock()
							return
						}
					}
				}
				logInfo(verbosity, "downloading segment", "segment", current.Index, "start", current.Start, "end", current.End)
				if err := downloader.download(childCtx, current.URL, filename, nil, "", "", resume, overwrite); err != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = fmt.Errorf("segment %d (%d-%d seconds): %w", current.Index, current.Start, current.End, err)
						cancel()
					}
					errMu.Unlock()
					return
				}
				paths[current.Index] = filename
				logInfo(verbosity, "segment downloaded", "segment", current.Index, "start", current.Start, "end", current.End)
			}
		}
	}
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go worker()
	}
	for _, current := range segments {
		select {
		case <-childCtx.Done():
			break
		case jobs <- current:
		}
		if childCtx.Err() != nil {
			break
		}
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return paths, nil
}
