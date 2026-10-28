package sidekiq

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// ErrorGroupKey identifies one aggregated error bucket.
type ErrorGroupKey struct {
	DisplayClass string
	ErrorClass   string
	Queue        string
}

// ErrorSummaryRow is one aggregated row in the Errors summary.
type ErrorSummaryRow struct {
	DisplayClass string
	ErrorClass   string
	Queue        string
	Count        int64
	ErrorMessage string
}

// ErrorSummaryMeta reports the exact matched totals behind an Errors summary snapshot.
type ErrorSummaryMeta struct {
	DeadCount  int64
	RetryCount int64
}

// ErrorGroupEntry is one job belonging to a selected error group.
type ErrorGroupEntry struct {
	Entry  *SortedEntry
	Source string
}

// ErrorGroupWindow is one exact paged window of jobs for an error group.
type ErrorGroupWindow struct {
	Entries     []ErrorGroupEntry
	Total       int64
	WindowStart int
}

type errorSummaryState struct {
	row    ErrorSummaryRow
	source string
	score  float64
}

// GetErrorSummary fetches exact error summary rows across dead and retry sets.
func (c *Client) GetErrorSummary(ctx context.Context, query string) ([]ErrorSummaryRow, ErrorSummaryMeta, error) {
	rowsByKey := make(map[ErrorGroupKey]*errorSummaryState)
	meta := ErrorSummaryMeta{}

	if err := c.scanSortedSetEntries(ctx, deadSetKey, query, func(entry *SortedEntry) error {
		meta.DeadCount++
		addErrorSummaryEntry(rowsByKey, entry, "dead")
		return nil
	}); err != nil {
		return nil, ErrorSummaryMeta{}, err
	}

	if err := c.scanSortedSetEntries(ctx, retrySetKey, query, func(entry *SortedEntry) error {
		meta.RetryCount++
		addErrorSummaryEntry(rowsByKey, entry, "retry")
		return nil
	}); err != nil {
		return nil, ErrorSummaryMeta{}, err
	}

	rows := make([]ErrorSummaryRow, 0, len(rowsByKey))
	for _, state := range rowsByKey {
		rows = append(rows, state.row)
	}
	sort.Slice(rows, func(i, j int) bool {
		return errorSummaryRowBefore(rows[i], rows[j])
	})

	return rows, meta, nil
}

// GetErrorGroupWindow fetches one exact paged error group window across dead and retry sets.
func (c *Client) GetErrorGroupWindow(
	ctx context.Context,
	key ErrorGroupKey,
	query string,
	start, count int,
) (ErrorGroupWindow, error) {
	start = max(start, 0)
	key = normalizedErrorGroupKey(key)

	for {
		window, err := c.getErrorGroupWindow(ctx, key, query, start, count)
		if err != nil {
			return ErrorGroupWindow{}, err
		}
		if count <= 0 {
			window.WindowStart = start
			return window, nil
		}

		maxStart := max(int(window.Total)-count, 0)
		if start > maxStart {
			start = maxStart
			continue
		}

		window.WindowStart = start
		return window, nil
	}
}

func (c *Client) getErrorGroupWindow(
	ctx context.Context,
	key ErrorGroupKey,
	query string,
	start, count int,
) (ErrorGroupWindow, error) {
	match := errorGroupScanMatch(key, query)

	deadEntries, deadTotal, err := c.collectErrorGroupEntries(ctx, deadSetKey, match, true, key, start, count)
	if err != nil {
		return ErrorGroupWindow{}, err
	}

	retryStart := max(start-int(deadTotal), 0)
	retryCount := -1
	if count > 0 {
		retryCount = max(count-len(deadEntries), 0)
	}

	retryEntries, retryTotal, err := c.collectErrorGroupEntries(ctx, retrySetKey, match, false, key, retryStart, retryCount)
	if err != nil {
		return ErrorGroupWindow{}, err
	}

	entries := make([]ErrorGroupEntry, 0, len(deadEntries)+len(retryEntries))
	for _, entry := range deadEntries {
		entries = append(entries, ErrorGroupEntry{Entry: entry, Source: "dead"})
	}
	for _, entry := range retryEntries {
		entries = append(entries, ErrorGroupEntry{Entry: entry, Source: "retry"})
	}

	return ErrorGroupWindow{
		Entries: entries,
		Total:   deadTotal + retryTotal,
	}, nil
}

func (c *Client) collectErrorGroupEntries(
	ctx context.Context,
	setKey, match string,
	reverse bool,
	groupKey ErrorGroupKey,
	start, count int,
) ([]*SortedEntry, int64, error) {
	start = max(start, 0)

	limit := -1
	switch {
	case count == 0:
		limit = 0
	case count > 0:
		limit = sortedEntryWindowLimit(start, count)
	}

	total := int64(0)
	var selected []*SortedEntry
	selection := newSortedEntrySelection(limit, reverse)
	err := c.scanSortedSetEntries(ctx, setKey, match, func(entry *SortedEntry) error {
		if normalizedErrorGroupKeyFromEntry(entry) != groupKey {
			return nil
		}

		total++
		if limit < 0 {
			selected = append(selected, entry)
			return nil
		}
		selection.add(entry)
		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	if limit >= 0 {
		return selection.window(start, count), total, nil
	}
	sortSortedEntries(selected, reverse)
	if start >= len(selected) {
		return nil, total, nil
	}
	return append([]*SortedEntry(nil), selected[start:]...), total, nil
}

func addErrorSummaryEntry(rowsByKey map[ErrorGroupKey]*errorSummaryState, entry *SortedEntry, source string) {
	metadata := errorMetadataFromEntry(entry)
	key := metadata.groupKey()
	if state, ok := rowsByKey[key]; ok {
		state.row.Count++
		if errorSummaryRepresentativeBefore(source, entry.Score, state.source, state.score) {
			state.row.ErrorMessage = normalizedErrorField(metadata.message)
			state.source = source
			state.score = entry.Score
		}
		return
	}

	rowsByKey[key] = &errorSummaryState{
		row: ErrorSummaryRow{
			DisplayClass: key.DisplayClass,
			ErrorClass:   key.ErrorClass,
			Queue:        key.Queue,
			Count:        1,
			ErrorMessage: normalizedErrorField(metadata.message),
		},
		source: source,
		score:  entry.Score,
	}
}

func errorSummaryRepresentativeBefore(source string, score float64, currentSource string, currentScore float64) bool {
	switch {
	case currentSource == "":
		return true
	case source == "dead" && currentSource == "dead":
		return score > currentScore
	case source == "retry" && currentSource == "retry":
		return score < currentScore
	case source == "dead":
		return true
	default:
		return false
	}
}

func errorSummaryRowBefore(a, b ErrorSummaryRow) bool {
	if a.DisplayClass != b.DisplayClass {
		return a.DisplayClass < b.DisplayClass
	}
	if a.ErrorClass != b.ErrorClass {
		return a.ErrorClass < b.ErrorClass
	}
	if a.Queue != b.Queue {
		return a.Queue < b.Queue
	}
	return a.ErrorMessage < b.ErrorMessage
}

func errorGroupScanMatch(key ErrorGroupKey, query string) string {
	if query != "" {
		return query
	}
	if key.ErrorClass != "" && key.ErrorClass != "unknown" {
		return key.ErrorClass
	}
	if key.Queue != "" && key.Queue != "unknown" {
		return key.Queue
	}
	return ""
}

func normalizedErrorGroupKeyFromEntry(entry *SortedEntry) ErrorGroupKey {
	return errorMetadataFromEntry(entry).groupKey()
}

func normalizedErrorGroupKey(key ErrorGroupKey) ErrorGroupKey {
	key.DisplayClass = normalizedErrorField(key.DisplayClass)
	key.ErrorClass = normalizedErrorField(key.ErrorClass)
	key.Queue = normalizedErrorField(key.Queue)
	return key
}

func normalizedErrorField(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return value
}

type errorJobMetadata struct {
	class      string
	queue      string
	errorClass string
	message    string
}

func (m errorJobMetadata) groupKey() ErrorGroupKey {
	return normalizedErrorGroupKey(ErrorGroupKey{
		DisplayClass: m.class,
		ErrorClass:   m.errorClass,
		Queue:        m.queue,
	})
}

func errorMetadataFromEntry(entry *SortedEntry) errorJobMetadata {
	if entry == nil || entry.JobRecord == nil {
		return errorJobMetadata{}
	}
	jr := entry.JobRecord
	if !jr.parsed {
		if metadata, ok := parseErrorJobMetadata(jr.value); ok {
			if jr.queue != "" {
				metadata.queue = jr.queue
			}
			return metadata
		}
	}
	// Preserve existing accessor behavior for malformed jobs and parsed records.
	return errorJobMetadata{
		class:      jr.DisplayClass(),
		queue:      jr.Queue(),
		errorClass: jr.ErrorClass(),
		message:    jr.ErrorMessage(),
	}
}

// parseErrorJobMetadata reads only grouping fields. Validate with encoding/json
// first so the span walker below never needs to implement JSON syntax rules.
// Keys stay case sensitive and duplicate/non-string fields match JobRecord's map.
func parseErrorJobMetadata(value string) (errorJobMetadata, bool) {
	raw := []byte(value)
	if !json.Valid(raw) {
		return errorJobMetadata{}, false
	}
	pos := skipErrorJSONSpace(raw, 0)
	if raw[pos] != '{' {
		return errorJobMetadata{}, false
	}
	var metadata errorJobMetadata
	var wrapped, args []byte
	for pos = skipErrorJSONSpace(raw, pos+1); raw[pos] != '}'; {
		key, value, next, ok := errorJSONFieldAt(raw, pos, true)
		if !ok {
			return errorJobMetadata{}, false
		}
		var field *string
		switch key {
		case "class":
			field = &metadata.class
		case "queue":
			field = &metadata.queue
		case "error_class":
			field = &metadata.errorClass
		case "error_message":
			field = &metadata.message
		case "wrapped":
			wrapped = value
		case "args":
			args = value
		}
		if field != nil {
			*field = ""
			if value[0] == '"' {
				if err := json.Unmarshal(value, field); err != nil {
					return errorJobMetadata{}, false
				}
			}
		}
		pos = next
	}
	if isActiveJobWrapper(metadata.class) {
		metadata.class = errorActiveJobDisplayClass(metadata.class, wrapped, args)
	}
	return metadata, true
}

// errorActiveJobDisplayClass reads the same class-bearing arguments as
// JobRecord.unwrapActiveJobDisplayClass, without decoding unrelated arguments.
func errorActiveJobDisplayClass(class string, wrapped, args []byte) string {
	displayClass, hasWrapped := errorJSONString(wrapped)
	var firstArg []byte
	if !hasWrapped || isActionMailerWrapper(displayClass) {
		if values := firstErrorJSONValues(args, 1); len(values) > 0 {
			firstArg = values[0]
		}
	}
	if !hasWrapped {
		displayClass = class
		if name, ok := errorJSONString(firstArg); ok {
			displayClass = name
		}
	}
	if !isActionMailerWrapper(displayClass) {
		return displayClass
	}

	arguments, _ := errorJSONObjectField(firstArg, "arguments")
	// deserializeArgument unwraps a sole GlobalID key without recursively
	// deserializing its value. Preserve that distinction even for unusual jobs.
	deserialize := true
	if globalID, otherFields := errorJSONObjectField(arguments, globalIDKey); globalID != nil && !otherFields {
		arguments = globalID
		deserialize = false
	}
	values := firstErrorJSONValues(arguments, 2)
	if len(values) < 2 {
		return displayClass
	}
	var names [2]string
	for i, value := range values {
		if deserialize {
			if globalID, otherFields := errorJSONObjectField(value, globalIDKey); globalID != nil && !otherFields {
				value = globalID
			}
		}
		name, ok := errorJSONString(value)
		if !ok {
			return displayClass
		}
		names[i] = name
	}
	return names[0] + "#" + names[1]
}

func errorJSONString(raw []byte) (string, bool) {
	if len(raw) == 0 || raw[0] != '"' {
		return "", false
	}
	var value string
	err := json.Unmarshal(raw, &value)
	return value, err == nil
}

// errorJSONObjectField returns the last matching field and whether other keys
// exist. Duplicate keys count once, matching JobRecord's decoded map semantics.
func errorJSONObjectField(raw []byte, name string) ([]byte, bool) {
	if len(raw) == 0 || raw[0] != '{' {
		return nil, false
	}
	var value []byte
	otherFields := false
	for pos := skipErrorJSONSpace(raw, 1); raw[pos] != '}'; {
		key, field, next, ok := errorJSONFieldAt(raw, pos, false)
		if !ok {
			return nil, false
		}
		if key == name {
			value = field
		} else {
			otherFields = true
		}
		pos = next
	}
	return value, otherFields
}

func firstErrorJSONValues(raw []byte, count int) [][]byte {
	if len(raw) == 0 || raw[0] != '[' {
		return nil
	}
	values := make([][]byte, 0, count)
	for pos := skipErrorJSONSpace(raw, 1); raw[pos] != ']' && len(values) < count; {
		end, ok := errorJSONValueEnd(raw, pos, false)
		if !ok {
			return nil
		}
		values = append(values, raw[pos:end])
		pos = skipErrorJSONSpace(raw, end)
		if raw[pos] == ',' {
			pos = skipErrorJSONSpace(raw, pos+1)
		}
	}
	return values
}

// errorJSONFieldAt reads a field from an already validated object. The top-level
// walk checks numeric ranges; nested lookups reuse that check. Value bytes borrow raw.
func errorJSONFieldAt(raw []byte, pos int, checkNumbers bool) (string, []byte, int, bool) {
	keyEnd := errorJSONStringEnd(raw, pos)
	key := string(raw[pos+1 : keyEnd-1])
	if bytes.ContainsRune(raw[pos:keyEnd], '\\') {
		if err := json.Unmarshal(raw[pos:keyEnd], &key); err != nil {
			return "", nil, 0, false
		}
	}
	colon := skipErrorJSONSpace(raw, keyEnd)
	start := skipErrorJSONSpace(raw, colon+1)
	end, ok := errorJSONValueEnd(raw, start, checkNumbers)
	if !ok {
		return "", nil, 0, false
	}
	next := skipErrorJSONSpace(raw, end)
	if raw[next] == ',' {
		next = skipErrorJSONSpace(raw, next+1)
	}
	return key, raw[start:end], next, true
}

func skipErrorJSONSpace(raw []byte, pos int) int {
	for pos < len(raw) && (raw[pos] == ' ' || raw[pos] == '\t' || raw[pos] == '\r' || raw[pos] == '\n') {
		pos++
	}
	return pos
}

// errorJSONStringEnd finds the closing quote in an already validated string.
// An odd number of preceding backslashes escapes a quote.
func errorJSONStringEnd(raw []byte, pos int) int {
	pos++
	for {
		quote := pos + bytes.IndexByte(raw[pos:], '"')
		backslashes := 0
		for i := quote - 1; i >= pos && raw[i] == '\\'; i-- {
			backslashes++
		}
		if backslashes%2 == 0 {
			return quote + 1
		}
		pos = quote + 1
	}
}

// errorJSONValueEnd walks one already validated value without decoding strings,
// containers or backtraces. The top-level pass must check even skipped numbers:
// JobRecord rejects float64 overflow and discards the entire job on error.
func errorJSONValueEnd(raw []byte, pos int, checkNumbers bool) (int, bool) {
	depth := 0
	for pos < len(raw) {
		switch raw[pos] {
		case '"':
			pos = errorJSONStringEnd(raw, pos)
		case '{', '[':
			depth++
			pos++
		case '}', ']':
			depth--
			pos++
		case ',', ':', ' ', '\t', '\n', '\r':
			pos++
		default:
			start := pos
			if end := bytes.IndexAny(raw[pos:], ",]} \t\r\n"); end >= 0 {
				pos += end
			} else {
				pos = len(raw)
			}
			if checkNumbers && (raw[start] == '-' || (raw[start] >= '0' && raw[start] <= '9')) {
				if _, err := strconv.ParseFloat(string(raw[start:pos]), 64); err != nil {
					return 0, false
				}
			}
		}
		if depth == 0 {
			return pos, true
		}
	}
	return pos, true
}
