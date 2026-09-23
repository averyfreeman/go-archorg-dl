//go:build integration

package archorgdl

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLiveSmoke(t *testing.T) {
	if os.Getenv("ARCHORG_LIVE") != "1" {
		t.Skip("set ARCHORG_LIVE=1 to contact Archive.org")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	parsed, err := parseArchiveURL("https://archive.org/details/CNNW_20260913_030000_Real_Time_With_Bill_Maher")
	if err != nil {
		t.Fatal(err)
	}
	client := &archiveClient{client: newHTTPClient(), retries: 1, retryDelay: 250 * time.Millisecond}
	item, err := client.getItem(ctx, parsed.Identifier)
	if err != nil {
		t.Fatal(err)
	}
	candidate, ok := selectVideoFile(item)
	if !ok {
		t.Fatal(ErrNoVideo)
	}
	duration := item.DurationSeconds
	if duration == nil {
		duration = candidate.LengthSeconds
	}
	if duration == nil {
		t.Fatal(ErrNoDuration)
	}
	segments, err := buildSegmentPlan(archiveDownloadURL(parsed.Identifier, candidate.Name), *duration, defaultSegmentSec)
	if err != nil {
		t.Fatal(err)
	}
	extractor := ytdlpExtractor{}
	if _, err := extractor.Resolve(ctx, []string{segments[0].URL}); err != nil {
		t.Fatal(err)
	}
}
