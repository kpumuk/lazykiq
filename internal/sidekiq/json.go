package sidekiq

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// safeParseJSON decodes JSON while preserving numbers as json.Number.
// It mirrors json.Unmarshal by rejecting trailing non-whitespace data.
func safeParseJSON(data []byte, dest any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(dest); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("extra JSON input")
	}
	return nil
}

type timestampFormat int

const (
	timestampUnknown timestampFormat = iota
	timestampSecondsFloat
	timestampMilliseconds
)

func detectTimestampFormat(payload map[string]any, version Version) timestampFormat {
	fields := []string{"enqueued_at", "created_at", "failed_at", "retried_at"}
	for _, field := range fields {
		seconds, ok := parseTimestampSeconds(payload[field])
		if !ok {
			continue
		}
		if seconds > 1e12 {
			return timestampMilliseconds
		}
		return timestampSecondsFloat
	}

	switch version {
	case Version7:
		return timestampSecondsFloat
	case Version8, VersionUnknown:
		return timestampMilliseconds
	default:
		return timestampMilliseconds
	}
}
