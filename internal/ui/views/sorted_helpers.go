package views

import (
	"context"

	"github.com/kpumuk/lazykiq/internal/devtools"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/lazytable"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
)

type sortedWindowConfig struct {
	filter           string
	windowStart      int
	windowSize       int
	fallbackPageSize int
	windowPages      int
	scan             func(context.Context, string) ([]*sidekiq.SortedEntry, error)
	fetch            func(context.Context, int, int) ([]*sidekiq.SortedEntry, int64, error)
	bounds           func(context.Context) (*sidekiq.SortedEntry, *sidekiq.SortedEntry, error)
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
	filter           string
	windowStart      int
	windowSize       int
	fallbackPageSize int
	windowPages      int
	scan             func(context.Context, string) ([]*sidekiq.SortedEntry, error)
	fetch            func(context.Context, int, int) ([]*sidekiq.SortedEntry, int64, error)
	bounds           func(context.Context) (*sidekiq.SortedEntry, *sidekiq.SortedEntry, error)
	buildRows        func([]*sidekiq.SortedEntry) []table.Row
}

func fetchSortedEntriesWindow(ctx context.Context, cfg sortedEntriesFetchConfig) (lazytable.FetchResult, error) {
	if cfg.tracker != "" {
		ctx = devtools.WithTracker(ctx, cfg.tracker)
	}
	result, err := fetchSortedWindow(ctx, sortedWindowConfig{
		filter:           cfg.filter,
		windowStart:      cfg.windowStart,
		windowSize:       cfg.windowSize,
		fallbackPageSize: cfg.fallbackPageSize,
		windowPages:      cfg.windowPages,
		scan:             cfg.scan,
		fetch:            cfg.fetch,
		bounds:           cfg.bounds,
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
	if cfg.filter != "" {
		jobs, err := cfg.scan(ctx, cfg.filter)
		if err != nil {
			return sortedWindowResult{}, err
		}
		firstEntry, lastEntry := sortedEntryBounds(jobs)
		return sortedWindowResult{
			jobs:        jobs,
			total:       int64(len(jobs)),
			windowStart: 0,
			firstEntry:  firstEntry,
			lastEntry:   lastEntry,
		}, nil
	}

	windowSize := cfg.windowSize
	if windowSize <= 0 {
		windowSize = max(cfg.fallbackPageSize, 1) * cfg.windowPages
	}

	jobs, totalSize, err := cfg.fetch(ctx, cfg.windowStart, windowSize)
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
		jobs, totalSize, err = cfg.fetch(ctx, windowStart, windowSize)
		if err != nil {
			return sortedWindowResult{}, err
		}
	}

	var firstEntry *sidekiq.SortedEntry
	var lastEntry *sidekiq.SortedEntry
	if totalSize > 0 {
		firstEntry, lastEntry, err = cfg.bounds(ctx)
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
