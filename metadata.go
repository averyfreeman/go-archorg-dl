package archorgdl

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	metadataURL       = "https://archive.org/metadata"
	downloadURL       = "https://archive.org/download"
	defaultSegmentSec = 300
)

var archiveIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
var safeFilenamePattern = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

type archiveURL struct {
	Identifier string
}

type archiveFile struct {
	Name          string
	Source        string
	Format        string
	Size          *int64
	MD5           string
	SHA1          string
	LengthSeconds *float64
}

func (f archiveFile) extension() string {
	return strings.ToLower(path.Ext(f.Name))
}

func (f archiveFile) isMP4() bool {
	return f.extension() == ".mp4"
}

type archiveItem struct {
	Identifier      string
	Title           string
	DurationSeconds *float64
	Files           []archiveFile
}

type segment struct {
	Index int
	Start int
	End   int
	URL   string
}

func parseArchiveURL(raw string) (archiveURL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return archiveURL{}, fmt.Errorf("invalid Archive.org URL: %w", err)
	}
	if parsed.Scheme != "https" || parsed.Hostname() != "archive.org" {
		return archiveURL{}, errors.New("URL must be an https://archive.org/details/{identifier} URL")
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 || parts[0] != "details" {
		return archiveURL{}, errors.New("URL must point to an Archive.org details item")
	}
	identifier, err := url.PathUnescape(parts[1])
	if err != nil || !archiveIdentifierPattern.MatchString(identifier) {
		return archiveURL{}, errors.New("Archive.org item identifiers may contain only letters, numbers, '_' and '-'")
	}
	return archiveURL{Identifier: identifier}, nil
}

func archiveDownloadURL(identifier, filename string) string {
	encodedIdentifier := url.PathEscape(identifier)
	encodedParts := make([]string, 0, len(strings.Split(filename, "/")))
	for _, part := range strings.Split(filename, "/") {
		encodedParts = append(encodedParts, url.PathEscape(part))
	}
	return downloadURL + "/" + encodedIdentifier + "/" + strings.Join(encodedParts, "/")
}

func parseMetadata(payload []byte, identifier string) (archiveItem, error) {
	var record map[string]json.RawMessage
	if err := json.Unmarshal(payload, &record); err != nil {
		return archiveItem{}, fmt.Errorf("Archive.org returned invalid JSON metadata: %w", err)
	}
	if message := optionalString(record["error"]); message != "" {
		return archiveItem{}, fmt.Errorf("Archive.org metadata error: %s", message)
	}

	var rawFiles []json.RawMessage
	if err := json.Unmarshal(record["files"], &rawFiles); err != nil || len(rawFiles) == 0 {
		return archiveItem{}, errors.New("Archive.org metadata did not include a usable file list")
	}
	files := make([]archiveFile, 0, len(rawFiles))
	for _, rawFile := range rawFiles {
		var value map[string]json.RawMessage
		if err := json.Unmarshal(rawFile, &value); err != nil {
			continue
		}
		name := optionalString(value["name"])
		if name == "" {
			continue
		}
		files = append(files, archiveFile{
			Name:          name,
			Source:        optionalString(value["source"]),
			Format:        optionalString(value["format"]),
			Size:          optionalInt64(value["size"]),
			MD5:           optionalString(value["md5"]),
			SHA1:          optionalString(value["sha1"]),
			LengthSeconds: optionalDuration(value["length"]),
		})
	}
	if len(files) == 0 {
		return archiveItem{}, errors.New("Archive.org metadata did not include a usable file list")
	}

	var nested map[string]json.RawMessage
	_ = json.Unmarshal(record["metadata"], &nested)
	title := optionalString(record["title"])
	if title == "" {
		title = optionalString(nested["title"])
	}
	duration := optionalDuration(record["duration"])
	if duration == nil {
		duration = optionalDuration(record["runtime"])
	}
	return archiveItem{Identifier: identifier, Title: title, DurationSeconds: duration, Files: files}, nil
}

func optionalString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return strings.TrimSpace(value)
	}
	return ""
}

func optionalInt64(raw json.RawMessage) *int64 {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	switch value := value.(type) {
	case float64:
		if value >= 0 && value == math.Trunc(value) {
			result := int64(value)
			return &result
		}
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err == nil && parsed >= 0 {
			return &parsed
		}
	}
	return nil
}

func optionalDuration(raw json.RawMessage) *float64 {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	var parsed float64
	switch value := value.(type) {
	case float64:
		parsed = value
	case string:
		text := strings.TrimSpace(value)
		if text == "" {
			return nil
		}
		if strings.Contains(text, ":") {
			parts := strings.Split(text, ":")
			if len(parts) != 2 && len(parts) != 3 {
				return nil
			}
			values := make([]float64, len(parts))
			for i, part := range parts {
				parsedPart, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
				if err != nil {
					return nil
				}
				values[i] = parsedPart
			}
			parsed = values[len(values)-1] + values[len(values)-2]*60
			if len(values) == 3 {
				parsed += values[0] * 3600
			}
		} else {
			var err error
			parsed, err = strconv.ParseFloat(text, 64)
			if err != nil {
				return nil
			}
		}
	default:
		return nil
	}
	if !math.IsNaN(parsed) && !math.IsInf(parsed, 0) && parsed > 0 {
		return &parsed
	}
	return nil
}

func isVideoFile(file archiveFile) bool {
	format := strings.ToLower(file.Format)
	return map[string]bool{
		".avi": true, ".m4v": true, ".mkv": true, ".mov": true, ".mp4": true, ".webm": true,
	}[file.extension()] || strings.Contains(format, "video") || strings.Contains(format, "mpeg")
}

func selectVideoFile(item archiveItem) (archiveFile, bool) {
	candidates := make([]archiveFile, 0, len(item.Files))
	for _, file := range item.Files {
		lowerName := strings.ToLower(file.Name)
		lowerFormat := strings.ToLower(file.Format)
		if isVideoFile(file) && !strings.HasSuffix(lowerName, "_thumb.mp4") && !strings.HasSuffix(lowerName, "_thumb.mkv") && !strings.Contains(lowerFormat, "thumbnail") {
			candidates = append(candidates, file)
		}
	}
	if len(candidates) == 0 {
		return archiveFile{}, false
	}
	type fileRank struct {
		nonMP4       int
		nonOriginal  int
		negativeSize int64
		name         string
	}
	rank := func(file archiveFile) fileRank {
		size := int64(0)
		if file.Size != nil {
			size = *file.Size
		}
		return fileRank{
			nonMP4:       boolRank(!file.isMP4()),
			nonOriginal:  boolRank(strings.ToLower(file.Source) != "original"),
			negativeSize: -size,
			name:         file.Name,
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left, right := rank(candidates[i]), rank(candidates[j])
		if left.nonMP4 != right.nonMP4 {
			return left.nonMP4 < right.nonMP4
		}
		if left.nonOriginal != right.nonOriginal {
			return left.nonOriginal < right.nonOriginal
		}
		if left.negativeSize != right.negativeSize {
			return left.negativeSize < right.negativeSize
		}
		return left.name < right.name
	})
	return candidates[0], true
}

func boolRank(value bool) int {
	if value {
		return 1
	}
	return 0
}

func buildSegmentPlan(sourceURL string, totalSeconds float64, segmentSeconds int) ([]segment, error) {
	if math.IsNaN(totalSeconds) || math.IsInf(totalSeconds, 0) || totalSeconds <= 0 {
		return nil, errors.New("a positive finite program duration is required for segmented fallback")
	}
	if segmentSeconds <= 0 {
		return nil, errors.New("segment duration must be greater than zero")
	}
	count := int(math.Ceil(totalSeconds / float64(segmentSeconds)))
	endTotal := int(math.Ceil(totalSeconds))
	segments := make([]segment, 0, count)
	for index := 0; index < count; index++ {
		start := index * segmentSeconds
		end := endTotal
		if candidate := start + segmentSeconds; candidate < end {
			end = candidate
		}
		parsed, err := url.Parse(sourceURL)
		if err != nil {
			return nil, fmt.Errorf("invalid segment source URL: %w", err)
		}
		query := parsed.Query()
		query.Add("start", strconv.Itoa(start))
		query.Add("end", strconv.Itoa(end))
		query.Add("ignore", "x.mp4")
		parsed.RawQuery = query.Encode()
		segments = append(segments, segment{Index: index, Start: start, End: end, URL: parsed.String()})
	}
	return segments, nil
}

func safeFilename(value string) string {
	cleaned := safeFilenamePattern.ReplaceAllString(value, "_")
	cleaned = strings.Trim(cleaned, "._")
	if cleaned == "" {
		return "archive_program"
	}
	return cleaned
}
