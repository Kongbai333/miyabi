package tasks

import (
	"encoding/json"
	"fmt"
)

// JSON property names used inside task payloads. SQLite expression indexes
// and sqljson predicates reference these so the on-disk shape has one owner.
const (
	PathAccountID        = "account_id"
	PathDirectoryID      = "directory_id"
	PathHash             = "hash"
	PathJavDBID          = "javdb_id"
	PathMovieID          = "movie_id"
	PathScanTaskID       = "scan_task_id"
	PathScrapeTaskID     = "scrape_task_id"
	PathTargetID         = "target_id"
	PathSource           = "source"
	PathFileID           = "file_id"
	PathAwaitingLocation = "awaiting_location"
	PathSnapshot         = "snapshot"
	PathArtwork          = "artwork"
)

// JSONExtract renders a SQLite json_extract expression for a nested path.
func JSONExtract(column string, parts ...string) string {
	path := "$"
	for _, part := range parts {
		path += "." + part
	}
	return "json_extract(" + column + ", '" + path + "')"
}

func DecodePayload[T any](payload json.RawMessage) (T, error) {
	var value T
	if err := json.Unmarshal(payload, &value); err != nil {
		return value, fmt.Errorf("decode stored task: %w", err)
	}
	return value, nil
}

func EncodePayload(value any) (json.RawMessage, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode task payload: %w", err)
	}
	return encoded, nil
}

// SetPayloadField replaces one top-level field without decoding the rest, so
// unknown fields written by newer builds survive.
func SetPayloadField(payload json.RawMessage, key string, value any) (json.RawMessage, error) {
	fields, err := DecodePayload[map[string]json.RawMessage](payload)
	if err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, fmt.Errorf("stored task payload must be an object")
	}
	encoded, err := EncodePayload(value)
	if err != nil {
		return nil, err
	}
	fields[key] = encoded
	return EncodePayload(fields)
}
