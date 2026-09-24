package archorgdl

import (
	"encoding/json"
	"math"
	"testing"
)

func TestParseArchiveURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
		good bool
	}{
		{name: "valid query", url: "https://archive.org/details/Show_20260101_010000?q=ignored", want: "Show_20260101_010000", good: true},
		{name: "valid trailing media path", url: "https://archive.org/details/Show_20260101_010000/Show_20260101_010000.mp4/start/0/end/300?ignore=x.mp4#player", want: "Show_20260101_010000", good: true},
		{name: "http rejected", url: "http://archive.org/details/show", good: false},
		{name: "wrong path rejected", url: "https://archive.org/download/show", good: false},
		{name: "unsafe identifier rejected", url: "https://archive.org/details/not.valid", good: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := parseArchiveURL(test.url)
			if test.good {
				if err != nil || parsed.Identifier != test.want {
					t.Fatalf("parseArchiveURL() = %#v, %v", parsed, err)
				}
			} else if err == nil {
				t.Fatal("parseArchiveURL() accepted invalid URL")
			}
		})
	}
}

func TestParseMetadataAndSelectVideo(t *testing.T) {
	payload := map[string]any{
		"metadata": map[string]any{"title": "Example Show"},
		"duration": "01:02:03",
		"files": []any{
			map[string]any{"name": "thumb.jpg", "format": "JPEG Thumb", "source": "original"},
			map[string]any{"name": "show.mkv", "format": "Matroska", "source": "original", "size": "20"},
			map[string]any{"name": "show.mp4", "format": "MPEG4", "source": "derivative", "size": "10"},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	item, err := parseMetadata(raw, "show")
	if err != nil {
		t.Fatal(err)
	}
	if item.Title != "Example Show" || item.DurationSeconds == nil || *item.DurationSeconds != 3723 {
		t.Fatalf("parsed item = %#v", item)
	}
	selected, ok := selectVideoFile(item)
	if !ok || selected.Name != "show.mp4" {
		t.Fatalf("selected video = %#v, ok=%v", selected, ok)
	}
}

func TestBuildSegmentPlanCoversFinalPartialSegment(t *testing.T) {
	segments, err := buildSegmentPlan("https://archive.org/download/show/show.mp4", 601, 300)
	if err != nil {
		t.Fatal(err)
	}
	if len(segments) != 3 || segments[2].Start != 600 || segments[2].End != 601 {
		t.Fatalf("segments = %#v", segments)
	}
	if got := segments[2].URL; got != "https://archive.org/download/show/show.mp4?end=601&ignore=x.mp4&start=600" {
		t.Fatalf("last segment URL = %s", got)
	}
}

func TestOptionalDuration(t *testing.T) {
	tests := map[string]float64{"01:02:03": 3723, "02:03": 123, "3723": 3723}
	for value, expected := range tests {
		raw, _ := json.Marshal(value)
		parsed := optionalDuration(raw)
		if parsed == nil || math.Abs(*parsed-expected) > 0.001 {
			t.Fatalf("optionalDuration(%q) = %v", value, parsed)
		}
	}
}

func TestSafeFilename(t *testing.T) {
	if got := safeFilename("a/../unsafe name"); got != "a_.._unsafe_name" {
		t.Fatalf("safeFilename() = %q", got)
	}
}
