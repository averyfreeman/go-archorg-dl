package archorgdl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

const (
	defaultHTTPTimeout = 30 * time.Second
	defaultRetries     = 3
	defaultRetryDelay  = time.Second
)

var retryableStatuses = map[int]bool{
	http.StatusRequestTimeout:      true,
	http.StatusTooManyRequests:     true,
	http.StatusInternalServerError: true,
	http.StatusBadGateway:          true,
	http.StatusServiceUnavailable:  true,
	http.StatusGatewayTimeout:      true,
}

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

func newHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: defaultHTTPTimeout, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = defaultHTTPTimeout
	transport.ResponseHeaderTimeout = defaultHTTPTimeout
	transport.ExpectContinueTimeout = time.Second
	return &http.Client{Transport: transport}
}

type archiveClient struct {
	client     httpDoer
	retries    int
	retryDelay time.Duration
}

func (c *archiveClient) getItem(ctx context.Context, identifier string) (archiveItem, error) {
	requestURL := metadataURL + "/" + identifier
	var response *http.Response
	var err error
	for attempt := 0; attempt <= c.retries; attempt++ {
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, http.NoBody)
		if requestErr != nil {
			return archiveItem{}, wrapError("create metadata request", requestErr)
		}
		request.Header.Set("Accept", "application/json")
		response, err = c.client.Do(request)
		if err == nil && retryableStatuses[response.StatusCode] {
			response.Body.Close()
			err = fmt.Errorf("transient HTTP status %d", response.StatusCode)
		}
		if err == nil {
			break
		}
		if !retryableError(err) || attempt == c.retries {
			return archiveItem{}, wrapError("request metadata", err)
		}
		if sleepErr := waitRetry(ctx, c.retryDelay, attempt+1); sleepErr != nil {
			return archiveItem{}, sleepErr
		}
	}
	if response == nil {
		return archiveItem{}, wrapError("request metadata", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return archiveItem{}, fmt.Errorf("Archive.org metadata request returned HTTP %d", response.StatusCode)
	}
	body, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		return archiveItem{}, wrapError("read metadata", readErr)
	}
	item, parseErr := parseMetadata(body, identifier)
	if parseErr != nil {
		return archiveItem{}, wrapError("parse metadata", parseErr)
	}
	return item, nil
}

func retryableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return true
}

func waitRetry(ctx context.Context, delay time.Duration, attempt int) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay * time.Duration(attempt))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
