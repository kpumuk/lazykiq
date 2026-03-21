package views

import (
	"fmt"

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
	if len(errorStr) > 100 {
		errorStr = errorStr[:99] + "…"
	}
	return errorStr
}
