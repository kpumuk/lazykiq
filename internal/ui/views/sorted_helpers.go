package views

import "github.com/kpumuk/lazykiq/internal/sidekiq"

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
