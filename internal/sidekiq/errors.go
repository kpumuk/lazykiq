package sidekiq

import (
	"context"
	"sort"
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
		limit = start + count
	}

	total := int64(0)
	selected := make([]*SortedEntry, 0, max(min(limit, int(sortedSetScanCount)), 0))
	err := c.scanSortedSetEntries(ctx, setKey, match, func(entry *SortedEntry) error {
		if normalizedErrorGroupKeyFromEntry(entry) != groupKey {
			return nil
		}

		total++
		if limit < 0 {
			selected = append(selected, entry)
			return nil
		}
		selected = insertSortedEntry(selected, entry, reverse, limit)
		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	if limit < 0 {
		sortSortedEntries(selected, reverse)
	}
	if count == 0 {
		return nil, total, nil
	}
	if start >= len(selected) {
		return nil, total, nil
	}
	if count > 0 {
		end := min(start+count, len(selected))
		return append([]*SortedEntry(nil), selected[start:end]...), total, nil
	}

	return append([]*SortedEntry(nil), selected[start:]...), total, nil
}

func addErrorSummaryEntry(rowsByKey map[ErrorGroupKey]*errorSummaryState, entry *SortedEntry, source string) {
	key := normalizedErrorGroupKeyFromEntry(entry)
	if state, ok := rowsByKey[key]; ok {
		state.row.Count++
		if errorSummaryRepresentativeBefore(source, entry.Score, state.source, state.score) {
			state.row.ErrorMessage = errorMessageOnly(entry)
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
			ErrorMessage: errorMessageOnly(entry),
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
	if entry == nil || entry.JobRecord == nil {
		return normalizedErrorGroupKey(ErrorGroupKey{})
	}
	return normalizedErrorGroupKey(ErrorGroupKey{
		DisplayClass: entry.DisplayClass(),
		ErrorClass:   entry.ErrorClass(),
		Queue:        entry.Queue(),
	})
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

func errorMessageOnly(entry *SortedEntry) string {
	if entry == nil || entry.JobRecord == nil {
		return "unknown"
	}

	message := strings.TrimSpace(entry.ErrorMessage())
	if message == "" {
		return "unknown"
	}
	return message
}
