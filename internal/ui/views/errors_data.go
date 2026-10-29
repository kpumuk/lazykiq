package views

import (
	"fmt"

	"github.com/charmbracelet/x/ansi"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
)

func errorGroupKeyForRow(row sidekiq.ErrorSummaryRow) sidekiq.ErrorGroupKey {
	return sidekiq.ErrorGroupKey{
		DisplayClass: row.DisplayClass,
		ErrorClass:   row.ErrorClass,
		Queue:        row.Queue,
	}
}

func errorGroupRowID(key sidekiq.ErrorGroupKey) string {
	return key.DisplayClass + "\x1f" + key.ErrorClass + "\x1f" + key.Queue
}

func errorDisplay(job *sidekiq.SortedEntry) string {
	if job == nil || job.JobRecord == nil {
		return ""
	}
	if !job.HasError() {
		return ""
	}

	errorStr := fmt.Sprintf("%s: %s", job.ErrorClass(), job.ErrorMessage())
	if ansi.StringWidth(errorStr) <= 100 {
		return errorStr
	}
	// Reserve one cell for the ellipsis and keep whole graphemes, including
	// keycap emoji that start with an ASCII digit.
	end, cells := 0, 0
	for end < len(errorStr) {
		cluster, width := ansi.FirstGraphemeCluster(errorStr[end:], ansi.GraphemeWidth)
		if cells+width > 99 {
			break
		}
		end += len(cluster)
		cells += width
	}
	return errorStr[:end] + "…"
}
