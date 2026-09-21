package tasks

import (
	"encoding/json"
	"fmt"
)

// Common JSON paths within task payloads for SQLite queries and indexing.
const (
	PathAccountID    = "account_id"
	PathDirectoryID  = "directory_id"
	PathHash         = "hash"
	PathJavDBID      = "javdb_id"
	PathMovieID      = "movie_id"
	PathScanTaskID   = "scan_task_id"
	PathScrapeTaskID = "scrape_task_id"
	PathTargetID     = "target_id"
	PathSource       = "source"
)

// Path returns a string slice for sqljson.Path.
func Path(parts ...string) []string {
	return parts
}

// JSONPath formats nested property names into SQLite JSON extraction expressions like "$.a.b".
func JSONPath(parts ...string) string {
	if len(parts) == 0 {
		return "$"
	}
	res := "$"
	for _, part := range parts {
		res += "." + part
	}
	return res
}

// Payload wraps a typed payload value.
type Payload[T any] struct {
	Data T
}

// Encode marshals the payload into raw JSON.
func (p Payload[T]) Encode() (json.RawMessage, error) {
	return EncodePayload(p.Data)
}

// ParsePayload unmarshals raw JSON into a typed Payload[T].
func ParsePayload[T any](raw json.RawMessage) (Payload[T], error) {
	data, err := DecodePayload[T](raw)
	if err != nil {
		return Payload[T]{}, err
	}
	return Payload[T]{Data: data}, nil
}

// DecodePayload unmarshals raw JSON into type T.
func DecodePayload[T any](payload json.RawMessage) (T, error) {
	var value T
	if err := json.Unmarshal(payload, &value); err != nil {
		return value, fmt.Errorf("decode stored task: %w", err)
	}
	return value, nil
}

// EncodePayload marshals any value into raw JSON.
func EncodePayload(value any) (json.RawMessage, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode task payload: %w", err)
	}
	return encoded, nil
}

// SetPayloadField modifies or adds a top-level field in an existing raw JSON object without
// deserializing the whole payload.
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
