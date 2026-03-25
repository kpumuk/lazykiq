package views

import (
	"context"

	"github.com/kpumuk/lazykiq/internal/devtools"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/lazytable"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
)

type sortedEntriesClient interface {
	GetSortedEntries(context.Context, sidekiq.SortedSetKind, int, int) ([]*sidekiq.SortedEntry, int64, error)
	ScanSortedEntries(context.Context, sidekiq.SortedSetKind, string) ([]*sidekiq.SortedEntry, error)
	GetSortedEntryBounds(context.Context, sidekiq.SortedSetKind) (*sidekiq.SortedEntry, *sidekiq.SortedEntry, error)
}

type sortedEntriesWindowScanner interface {
	ScanSortedEntriesWindow(context.Context, sidekiq.SortedSetKind, string, int, int) (sidekiq.SortedEntriesWindow, error)
}

type sortedWindowConfig struct {
	client           sortedEntriesClient
	kind             sidekiq.SortedSetKind
	filter           string
	windowStart      int
	windowSize       int
	fallbackPageSize int
	windowPages      int
}

type sortedWindowResult struct {
	jobs        []*sidekiq.SortedEntry
	total       int64
	windowStart int
	firstEntry  *sidekiq.SortedEntry
	lastEntry   *sidekiq.SortedEntry
}

type sortedEntriesPayload struct {
	jobs       []*sidekiq.SortedEntry
	firstEntry *sidekiq.SortedEntry
	lastEntry  *sidekiq.SortedEntry
}

type sortedEntriesFetchConfig struct {
	tracker          string
	client           sortedEntriesClient
	kind             sidekiq.SortedSetKind
	filter           string
	windowStart      int
	windowSize       int
	fallbackPageSize int
	windowPages      int
	buildRows        func([]*sidekiq.SortedEntry) []table.Row
}

func fetchSortedEntriesWindow(ctx context.Context, cfg sortedEntriesFetchConfig) (lazytable.FetchResult, error) {
	if cfg.tracker != "" {
		ctx = devtools.WithTracker(ctx, cfg.tracker)
	}
	result, err := fetchSortedWindow(ctx, sortedWindowConfig{
		client:           cfg.client,
		kind:             cfg.kind,
		filter:           cfg.filter,
		windowStart:      cfg.windowStart,
		windowSize:       cfg.windowSize,
		fallbackPageSize: cfg.fallbackPageSize,
		windowPages:      cfg.windowPages,
	})
	if err != nil {
		return lazytable.FetchResult{}, err
	}

	return lazytable.FetchResult{
		Rows:        cfg.buildRows(result.jobs),
		Total:       result.total,
		WindowStart: result.windowStart,
		Payload: sortedEntriesPayload{
			jobs:       result.jobs,
			firstEntry: result.firstEntry,
			lastEntry:  result.lastEntry,
		},
	}, nil
}

func fetchSortedWindow(ctx context.Context, cfg sortedWindowConfig) (sortedWindowResult, error) {
	windowSize := cfg.windowSize
	if windowSize <= 0 {
		windowSize = max(cfg.fallbackPageSize, 1) * max(cfg.windowPages, 1)
	}

	if cfg.filter != "" {
		return fetchFilteredSortedWindow(ctx, cfg, windowSize)
	}

	jobs, totalSize, err := cfg.client.GetSortedEntries(ctx, cfg.kind, cfg.windowStart, windowSize)
	if err != nil {
		return sortedWindowResult{}, err
	}

	if totalSize <= 0 {
		return sortedWindowResult{
			jobs:        jobs,
			total:       totalSize,
			windowStart: 0,
		}, nil
	}

	windowStart := cfg.windowStart
	maxStart := max(int(totalSize)-windowSize, 0)
	if windowStart > maxStart {
		windowStart = maxStart
		jobs, totalSize, err = cfg.client.GetSortedEntries(ctx, cfg.kind, windowStart, windowSize)
		if err != nil {
			return sortedWindowResult{}, err
		}
	}

	var firstEntry *sidekiq.SortedEntry
	var lastEntry *sidekiq.SortedEntry
	if totalSize > 0 {
		firstEntry, lastEntry, err = cfg.client.GetSortedEntryBounds(ctx, cfg.kind)
		if err != nil {
			return sortedWindowResult{}, err
		}
	}

	return sortedWindowResult{
		jobs:        jobs,
		total:       totalSize,
		windowStart: windowStart,
		firstEntry:  firstEntry,
		lastEntry:   lastEntry,
	}, nil
}

func fetchFilteredSortedWindow(
	ctx context.Context,
	cfg sortedWindowConfig,
	windowSize int,
) (sortedWindowResult, error) {
	if _, ok := cfg.client.(sortedEntriesWindowScanner); ok {
		return fetchFilteredSortedWindowPage(ctx, cfg, windowSize)
	}
	return fetchFilteredSortedWindowFallback(ctx, cfg, windowSize)
}

func fetchFilteredSortedWindowPage(
	ctx context.Context,
	cfg sortedWindowConfig,
	windowSize int,
) (sortedWindowResult, error) {
	scanner := cfg.client.(sortedEntriesWindowScanner)
	windowStart := max(cfg.windowStart, 0)
	window, err := scanner.ScanSortedEntriesWindow(ctx, cfg.kind, cfg.filter, windowStart, windowSize)
	if err != nil {
		return sortedWindowResult{}, err
	}
	if window.Total <= 0 {
		return sortedWindowResult{total: 0}, nil
	}

	maxStart := max(int(window.Total)-windowSize, 0)
	if windowStart > maxStart {
		windowStart = maxStart
		window, err = scanner.ScanSortedEntriesWindow(ctx, cfg.kind, cfg.filter, windowStart, windowSize)
		if err != nil {
			return sortedWindowResult{}, err
		}
	}

	return sortedWindowResult{
		jobs:        window.Entries,
		total:       window.Total,
		windowStart: windowStart,
		firstEntry:  window.FirstEntry,
		lastEntry:   window.LastEntry,
	}, nil
}

func fetchFilteredSortedWindowFallback(
	ctx context.Context,
	cfg sortedWindowConfig,
	windowSize int,
) (sortedWindowResult, error) {
	jobs, err := cfg.client.ScanSortedEntries(ctx, cfg.kind, cfg.filter)
	if err != nil {
		return sortedWindowResult{}, err
	}

	total := int64(len(jobs))
	if total <= 0 {
		return sortedWindowResult{total: 0}, nil
	}

	windowStart := max(cfg.windowStart, 0)
	maxStart := max(int(total)-windowSize, 0)
	if windowStart > maxStart {
		windowStart = maxStart
	}

	firstEntry, lastEntry := sortedEntryBounds(jobs)
	end := min(windowStart+windowSize, len(jobs))
	return sortedWindowResult{
		jobs:        jobs[windowStart:end],
		total:       total,
		windowStart: windowStart,
		firstEntry:  firstEntry,
		lastEntry:   lastEntry,
	}, nil
}

func sortedEntryBounds(entries []*sidekiq.SortedEntry) (*sidekiq.SortedEntry, *sidekiq.SortedEntry) {
	if len(entries) == 0 {
		return nil, nil
	}

	minEntry := entries[0]
	maxEntry := entries[0]
	for _, entry := range entries[1:] {
		if entry.Score < minEntry.Score {
			minEntry = entry
		}
		if entry.Score > maxEntry.Score {
			maxEntry = entry
		}
	}

	return minEntry, maxEntry
}
