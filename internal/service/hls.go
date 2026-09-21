package service

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	hlsStreamInf = "#EXT-X-STREAM-INF:"
	hlsDuration  = "#EXTINF:"
	hlsByteRange = "#EXT-X-BYTERANGE:"
)

// hlsVariant is one rendition a master playlist offers.
type hlsVariant struct {
	uri string
	// pixels is 0 when the playlist omits RESOLUTION.
	pixels int
}

// hlsSegment is one independently decodable chunk of a media playlist. A
// segment that is a whole file leaves start at -1.
type hlsSegment struct {
	uri    string
	start  int64
	length int64
}

type hlsPlaylist struct {
	variants []hlsVariant
	segments []hlsSegment
}

// parseHLSPlaylist reads the subset of an HLS playlist this service needs. 115
// serves a media playlist directly for some files and a master playlist for
// others, so the same parser handles both.
func parseHLSPlaylist(body []byte) (hlsPlaylist, error) {
	var (
		result    hlsPlaylist
		expectURI string
		pixels    int
		segment   hlsSegment
		// A byte range without an offset continues the previous one, so the
		// first range in a playlist has to state one.
		previousEnd = int64(-1)
	)
	segment.start = -1
	for _, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "#") {
			switch expectURI {
			case "variant":
				result.variants = append(result.variants, hlsVariant{uri: trimmed, pixels: pixels})
			case "segment":
				segment.uri = trimmed
				result.segments = append(result.segments, segment)
				if segment.length > 0 {
					previousEnd = segment.start + segment.length
				}
			}
			expectURI, pixels, segment = "", 0, hlsSegment{start: -1}
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, hlsStreamInf):
			expectURI, pixels = "variant", parseResolution(trimmed)
		case strings.HasPrefix(trimmed, hlsDuration):
			expectURI = "segment"
		case strings.HasPrefix(trimmed, hlsByteRange):
			length, start, err := parseByteRange(strings.TrimPrefix(trimmed, hlsByteRange), previousEnd)
			if err != nil {
				return hlsPlaylist{}, err
			}
			segment.length, segment.start = length, start
		}
	}
	return result, nil
}

// bestVariant returns the highest quality rendition, so the still is not taken
// from a low resolution transcode. Playlists without RESOLUTION keep their order.
func (playlist hlsPlaylist) bestVariant() string {
	best := hlsVariant{}
	for _, variant := range playlist.variants {
		if best.uri == "" || variant.pixels > best.pixels {
			best = variant
		}
	}
	return best.uri
}

// stillSegment picks a segment that is unlikely to be a title card or a fade
// from black, while keeping the download to one chunk of the stream.
func (playlist hlsPlaylist) stillSegment() (hlsSegment, bool) {
	if len(playlist.segments) == 0 {
		return hlsSegment{}, false
	}
	return playlist.segments[len(playlist.segments)/4], true
}

func parseResolution(tag string) int {
	const key = "RESOLUTION="
	index := strings.Index(tag, key)
	if index < 0 {
		return 0
	}
	value := tag[index+len(key):]
	if end := strings.IndexByte(value, ','); end >= 0 {
		value = value[:end]
	}
	width, height, found := strings.Cut(value, "x")
	if !found {
		return 0
	}
	columns, widthErr := strconv.Atoi(strings.TrimSpace(width))
	rows, heightErr := strconv.Atoi(strings.TrimSpace(height))
	if widthErr != nil || heightErr != nil || columns <= 0 || rows <= 0 {
		return 0
	}
	return columns * rows
}

// parseByteRange reads "length[@offset]". A range without an offset continues
// where the previous one ended, which the playlist must have already stated.
func parseByteRange(value string, previousEnd int64) (int64, int64, error) {
	length, offset, hasOffset := strings.Cut(value, "@")
	size, err := strconv.ParseInt(strings.TrimSpace(length), 10, 64)
	if err != nil || size <= 0 {
		return 0, 0, fmt.Errorf("115 playlist has an invalid byte range %q", strings.TrimSpace(value))
	}
	start := previousEnd
	if hasOffset {
		start, err = strconv.ParseInt(strings.TrimSpace(offset), 10, 64)
		if err != nil || start < 0 {
			return 0, 0, fmt.Errorf("115 playlist has an invalid byte range %q", strings.TrimSpace(value))
		}
	}
	if start < 0 {
		return 0, 0, fmt.Errorf("115 playlist byte range %q has no offset to continue from", strings.TrimSpace(value))
	}
	return size, start, nil
}
